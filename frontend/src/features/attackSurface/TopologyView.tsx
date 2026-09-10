// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  AlertTriangle,
  Crosshair,
  Download,
  Image as ImageIcon,
  Maximize2,
  Minus,
  Network,
  Plus,
  X,
} from 'lucide-react';
import { PageFrame, PageHeader, Btn, Chip, CritBadge, Skeleton } from '../../shared/ui';
import { critColor, type Criticality } from '../../shared/riskColors';
import { useToast } from '../../hooks/useToast';
import { topologyService } from './topologyService';
import {
  bounds,
  createLayout,
  hitTest,
  reheat,
  tick,
  type LaidOutNode,
  type LayoutMode,
  type LayoutState,
} from './forceLayout';
import { downloadPng, downloadSvg } from './topologyExport';
import {
  EDGE_DASH,
  EDGE_LABELS,
  TOPOLOGY_EDGE_TYPES,
  type ColorMode,
  type CompromiseChain,
  type TopologyEdgeType,
  type TopologyNode,
} from './topologyTypes';
import { CATEGORY_LABELS, type AssetCategory } from './schemaTypes';
import { useAssetDependencies } from '../assets/useAssetDependencies';
import { useAuthStore } from '../../hooks/useAuthStore';
import { DEPENDENCY_TYPES, type AssetDependency, type DependencyType } from '../../types/asset';
import { apiErrorMessage } from '../../lib/apiError';

const CRIT_LABEL: Record<string, string> = {
  CRITICAL: 'Critique',
  HIGH: 'Élevée',
  MEDIUM: 'Moyenne',
  LOW: 'Faible',
};

const EXPOSED_COLOR = 'var(--critical)';
const INTERNAL_COLOR = 'var(--low)';

/** Resolves a CSS variable to a concrete colour — the SVG export cannot carry
 *  `var(--x)` out of the document. */
function resolveColor(cssVar: string, root: HTMLElement): string {
  const name = cssVar.match(/var\((--[^)]+)\)/)?.[1];
  if (!name) return cssVar;
  // Falling back to --fg-muted rather than a literal grey: an unresolvable
  // token should still land on something that follows the theme.
  const resolved = getComputedStyle(root).getPropertyValue(name).trim();
  // An unresolvable token falls back to another TOKEN, never a literal: the
  // fallback has to follow the theme too.
  return resolved || getComputedStyle(root).getPropertyValue('--fg-muted').trim();
}

export default function TopologyView() {
  const toast = useToast();
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const layoutRef = useRef<LayoutState | null>(null);
  const rafRef = useRef<number | null>(null);
  // The camera lives in a ref, not state: it changes on every wheel/drag event
  // and a re-render per event would be the whole frame budget.
  const camRef = useRef({ x: 0, y: 0, k: 1 });

  const [colorMode, setColorMode] = useState<ColorMode>('criticality');
  const [layoutMode, setLayoutMode] = useState<LayoutMode>('zones');
  const [zoneFilter, setZoneFilter] = useState<string>('');
  // Criticality filter, carried over from the Asset Universe. Unticking a level
  // drops those assets AND every edge that ends on one — a dangling edge to a
  // hidden node would draw a dependency on nothing.
  const [critFilter, setCritFilter] = useState<Set<string>>(
    new Set(['CRITICAL', 'HIGH', 'MEDIUM', 'LOW']),
  );
  const [selected, setSelected] = useState<LaidOutNode | null>(null);
  const [chainOrigin, setChainOrigin] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);

  // Editing the graph lives here rather than on a separate screen: the place you
  // notice a missing dependency is while looking at the topology.
  const canEdit = useAuthStore((st) => st.hasPermission('assets:update'));
  const { dependencies, createDependency, deleteDependency } = useAssetDependencies();

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['attack-surface', 'topology'],
    queryFn: () => topologyService.get(),
  });

  const { data: chain } = useQuery<CompromiseChain>({
    queryKey: ['attack-surface', 'chain', chainOrigin],
    queryFn: () => topologyService.compromiseChain(chainOrigin as string),
    enabled: !!chainOrigin,
  });

  const nodes = useMemo(() => data?.nodes ?? [], [data]);
  const edges = useMemo(() => data?.edges ?? [], [data]);

  // How many assets survive the zone and criticality filters.
  //
  // The header used to report data.nodes.length unconditionally, so unticking
  // CRITICAL removed nodes from the canvas while the count kept claiming the
  // whole estate. A count that ignores the filter beside a graph that obeys it
  // is a screen telling the user two different things at once.
  //
  // Mirrors the predicate the layout effect uses; if one changes the other must.
  const visibleCount = useMemo(
    () =>
      nodes.filter(
        (n) =>
          (!zoneFilter || n.zone === zoneFilter) &&
          critFilter.has((n.criticality ?? 'LOW') as string),
      ).length,
    [nodes, zoneFilter, critFilter],
  );
  const filtered = visibleCount !== nodes.length;

  // The highlighted chain: origin + everything impacted + everything reachable.
  const highlighted = useMemo(() => {
    if (!chain) return null;
    const s = new Set<string>([chain.origin_id as string]);
    for (const h of chain.impacted ?? []) s.add(h.asset_id as string);
    for (const h of chain.reachable ?? []) s.add(h.asset_id as string);
    return s;
  }, [chain]);
  const highlightedEdges = useMemo(
    () => (chain ? new Set((chain.edge_ids ?? []) as string[]) : null),
    [chain],
  );

  const colorOfNode = useCallback(
    (n: LaidOutNode): string => {
      if (colorMode === 'exposure') {
        return n.node.internet_exposed ? EXPOSED_COLOR : INTERNAL_COLOR;
      }
      const crit = ((n.node.criticality ?? 'LOW') as string).toLowerCase() as Criticality;
      return critColor[crit] ?? critColor.low;
    },
    [colorMode],
  );

  // --- build / rebuild the layout when the data or the zone filter changes ---
  useEffect(() => {
    const wrap = wrapRef.current;
    if (!wrap || nodes.length === 0) {
      layoutRef.current = null;
      return;
    }
    const visibleNodes = nodes.filter(
      (n) =>
        (!zoneFilter || n.zone === zoneFilter) &&
        critFilter.has((n.criticality ?? 'LOW') as string),
    );
    const visibleIds = new Set(visibleNodes.map((n) => n.id as string));
    const visibleEdges = edges.filter(
      (e) => visibleIds.has(e.source as string) && visibleIds.has(e.target as string),
    );

    const w = wrap.clientWidth || 900;
    const h = wrap.clientHeight || 600;
    layoutRef.current = createLayout(visibleNodes, visibleEdges, w, h, layoutMode);
    // Frame the whole graph rather than dropping the user at an arbitrary
    // corner of it.
    fitToView();
  }, [nodes, edges, zoneFilter, critFilter, layoutMode]); // eslint-disable-line react-hooks/exhaustive-deps

  const fitToView = useCallback(() => {
    const st = layoutRef.current;
    const wrap = wrapRef.current;
    if (!st || !wrap) return;
    const b = bounds(st);
    const w = wrap.clientWidth || 900;
    const h = wrap.clientHeight || 600;
    const k = Math.min(w / (b.maxX - b.minX), h / (b.maxY - b.minY), 2.5);
    camRef.current = {
      k,
      x: w / 2 - ((b.minX + b.maxX) / 2) * k,
      y: h / 2 - ((b.minY + b.maxY) / 2) * k,
    };
  }, []);

  // --- render loop ----------------------------------------------------------
  useEffect(() => {
    const draw = () => {
      const canvas = canvasRef.current;
      const wrap = wrapRef.current;
      const st = layoutRef.current;
      if (canvas && wrap && st) {
        const dpr = window.devicePixelRatio || 1;
        const w = wrap.clientWidth;
        const h = wrap.clientHeight;
        if (canvas.width !== w * dpr || canvas.height !== h * dpr) {
          canvas.width = w * dpr;
          canvas.height = h * dpr;
          canvas.style.width = `${w}px`;
          canvas.style.height = `${h}px`;
        }
        const ctx = canvas.getContext('2d');
        if (ctx) {
          tick(st);
          paint(ctx, st, dpr, w, h);
        }
      }
      rafRef.current = requestAnimationFrame(draw);
    };
    rafRef.current = requestAnimationFrame(draw);
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
    };
  });

  const paint = (
    ctx: CanvasRenderingContext2D,
    st: LayoutState,
    dpr: number,
    w: number,
    h: number,
  ) => {
    const root = document.documentElement;
    const ink = resolveColor('var(--fg-primary)', root);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);

    const cam = camRef.current;
    ctx.save();
    ctx.translate(cam.x, cam.y);
    ctx.scale(cam.k, cam.k);

    const dim = !!highlighted;

    // Edges. Dash pattern carries the relation type; colour is reserved for the
    // criticality/exposure scale so the two never fight.
    ctx.lineCap = 'round';
    for (const e of st.edges) {
      const on = !dim || highlightedEdges?.has(e.id);
      ctx.globalAlpha = on ? 0.45 : 0.06;
      ctx.strokeStyle = ink;
      ctx.lineWidth = (on ? 1.4 : 1) / cam.k;
      const dash = EDGE_DASH[(e.edge.type ?? 'depends_on') as TopologyEdgeType] ?? [];
      ctx.setLineDash(dash.map((d) => d / cam.k));
      ctx.beginPath();
      ctx.moveTo(e.source.x, e.source.y);
      ctx.lineTo(e.target.x, e.target.y);
      ctx.stroke();
    }
    ctx.setLineDash([]);

    // Nodes.
    for (const n of st.nodes) {
      const on = !dim || highlighted?.has(n.id);
      ctx.globalAlpha = on ? 1 : 0.14;
      ctx.fillStyle = resolveColor(colorOfNode(n), root);
      ctx.beginPath();
      ctx.arc(n.x, n.y, n.r, 0, Math.PI * 2);
      ctx.fill();
      if (n.id === chainOrigin) {
        ctx.globalAlpha = 1;
        ctx.strokeStyle = ink;
        ctx.lineWidth = 2 / cam.k;
        ctx.beginPath();
        ctx.arc(n.x, n.y, n.r + 4, 0, Math.PI * 2);
        ctx.stroke();
      }
      if (selected && n.id === selected.id) {
        ctx.globalAlpha = 1;
        ctx.strokeStyle = resolveColor('var(--accent)', root);
        ctx.lineWidth = 2 / cam.k;
        ctx.beginPath();
        ctx.arc(n.x, n.y, n.r + 6, 0, Math.PI * 2);
        ctx.stroke();
      }
    }

    // Labels. Universe mode uses large orbs and is meant to be read at a glance,
    // so it labels sooner; topology mode waits until the labels would not smear.
    const labelZoom = st.mode === 'universe' ? 0.3 : 0.55;
    if (cam.k > labelZoom) {
      ctx.globalAlpha = 0.85;
      ctx.fillStyle = ink;
      ctx.font = `${11 / cam.k}px ui-sans-serif, system-ui, sans-serif`;
      for (const n of st.nodes) {
        if (dim && !highlighted?.has(n.id)) continue;
        ctx.fillText(n.node.name ?? '', n.x + n.r + 3 / cam.k, n.y + 3 / cam.k);
      }
    }

    ctx.globalAlpha = 1;
    ctx.restore();
  };

  // --- pointer interaction: pan, zoom, drag, select -------------------------
  const dragRef = useRef<{ node: LaidOutNode | null; lastX: number; lastY: number } | null>(null);

  const toWorld = (clientX: number, clientY: number) => {
    const rect = canvasRef.current?.getBoundingClientRect();
    const cam = camRef.current;
    const px = clientX - (rect?.left ?? 0);
    const py = clientY - (rect?.top ?? 0);
    return { x: (px - cam.x) / cam.k, y: (py - cam.y) / cam.k };
  };

  const onPointerDown = (e: React.PointerEvent) => {
    const st = layoutRef.current;
    if (!st) return;
    (e.target as Element).setPointerCapture?.(e.pointerId);
    const { x, y } = toWorld(e.clientX, e.clientY);
    const hit = hitTest(st, x, y);
    dragRef.current = { node: hit, lastX: e.clientX, lastY: e.clientY };
    if (hit) {
      setSelected(hit);
      hit.fixed = true;
    }
  };

  const onPointerMove = (e: React.PointerEvent) => {
    const drag = dragRef.current;
    const st = layoutRef.current;
    if (!drag || !st) return;
    const dx = e.clientX - drag.lastX;
    const dy = e.clientY - drag.lastY;
    drag.lastX = e.clientX;
    drag.lastY = e.clientY;
    if (drag.node) {
      drag.node.x += dx / camRef.current.k;
      drag.node.y += dy / camRef.current.k;
      reheat(st, 0.35);
    } else {
      camRef.current.x += dx;
      camRef.current.y += dy;
    }
  };

  const onPointerUp = () => {
    const drag = dragRef.current;
    if (drag?.node) drag.node.fixed = false;
    dragRef.current = null;
  };

  const zoomBy = (factor: number, centerX?: number, centerY?: number) => {
    const wrap = wrapRef.current;
    const cam = camRef.current;
    const cx = centerX ?? (wrap?.clientWidth ?? 0) / 2;
    const cy = centerY ?? (wrap?.clientHeight ?? 0) / 2;
    const next = Math.min(4, Math.max(0.08, cam.k * factor));
    // Keep the point under the cursor fixed while zooming — otherwise the graph
    // slides away from whatever the user was aiming at.
    cam.x = cx - ((cx - cam.x) / cam.k) * next;
    cam.y = cy - ((cy - cam.y) / cam.k) * next;
    cam.k = next;
  };

  const onWheel = (e: React.WheelEvent) => {
    const rect = canvasRef.current?.getBoundingClientRect();
    zoomBy(
      e.deltaY < 0 ? 1.12 : 1 / 1.12,
      e.clientX - (rect?.left ?? 0),
      e.clientY - (rect?.top ?? 0),
    );
  };

  // --- export ---------------------------------------------------------------
  const exportOptions = () => {
    const root = document.documentElement;
    const st = layoutRef.current;
    const colors = new Map<string, string>();
    for (const n of st?.nodes ?? []) colors.set(n.id, resolveColor(colorOfNode(n), root));
    return {
      // The export leaves the document, so every colour must already be a
      // literal here — `var(--x)` means nothing in a downloaded file. They are
      // resolved FROM tokens, which is what makes the export follow whichever
      // theme it was taken in.
      colorOf: (id: string) => colors.get(id) ?? resolveColor('var(--fg-muted)', root),
      highlighted: highlighted ?? undefined,
      highlightedEdges: highlightedEdges ?? undefined,
      background: resolveColor('var(--surface-1)', root),
      ink: resolveColor('var(--fg-primary)', root),
      title: `Topologie — ${st?.nodes.length ?? 0} actifs`,
    };
  };

  const doExport = async (format: 'svg' | 'png') => {
    const st = layoutRef.current;
    if (!st) return;
    setExporting(true);
    try {
      if (format === 'svg') {
        downloadSvg(st, exportOptions());
      } else {
        await downloadPng(st, exportOptions());
      }
      toast.success(`Export ${format.toUpperCase()} téléchargé.`);
    } catch (err) {
      // A failed export must say so; a button that silently does nothing is the
      // exact failure this module exists to remove.
      toast.error(err instanceof Error ? err.message : "L'export a échoué.");
    } finally {
      setExporting(false);
    }
  };

  const zones = data?.zones ?? [];
  const chainCounts = chain
    ? { impacted: chain.impacted?.length ?? 0, reachable: chain.reachable?.length ?? 0 }
    : null;

  return (
    <PageFrame wide>
      <PageHeader
        title="Topologie de la surface d'attaque"
        count={
          data
            ? filtered
              ? `${visibleCount} / ${nodes.length} actifs`
              : `${nodes.length} actifs · ${edges.length} liens`
            : null
        }
        actions={
          <>
            <Btn label="SVG" icon={Download} onClick={() => void doExport('svg')} />
            <Btn
              label={exporting ? 'Export…' : 'PNG'}
              icon={ImageIcon}
              onClick={() => void doExport('png')}
            />
          </>
        }
      />

      {data?.truncated ? (
        <div
          className="mb-3 flex items-start gap-2 rounded-xl border px-3 py-2 text-[13px]"
          style={{
            borderColor: 'var(--high)',
            background: 'var(--surface-2)',
            color: 'var(--fg-secondary)',
          }}
        >
          <AlertTriangle size={16} style={{ color: 'var(--high)' }} className="mt-0.5 shrink-0" />
          <span>
            Le graphe est limité à {data.node_limit} nœuds : les actifs les plus critiques et les
            plus connectés sont affichés. Filtrez par zone pour voir le reste.
          </span>
        </div>
      ) : null}

      {/* Controls */}
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <span className="text-[12px]" style={{ color: 'var(--fg-muted)' }}>
          Vue
        </span>
        <Chip
          label="Topologie"
          active={layoutMode === 'zones'}
          onClick={() => setLayoutMode('zones')}
        />
        <Chip
          label="Univers"
          active={layoutMode === 'universe'}
          onClick={() => setLayoutMode('universe')}
        />

        <span className="ml-3 text-[12px]" style={{ color: 'var(--fg-muted)' }}>
          Couleur
        </span>
        <Chip
          label="Criticité"
          active={colorMode === 'criticality'}
          onClick={() => setColorMode('criticality')}
        />
        <Chip
          label="Exposition Internet"
          active={colorMode === 'exposure'}
          onClick={() => setColorMode('exposure')}
        />

        <span className="ml-3 text-[12px]" style={{ color: 'var(--fg-muted)' }}>
          Zone
        </span>
        <Chip label="Toutes" active={!zoneFilter} onClick={() => setZoneFilter('')} />
        {zones.slice(0, 8).map((z) => (
          <Chip
            key={z.key}
            label={`${z.label} (${z.count})`}
            active={zoneFilter === z.key}
            onClick={() => setZoneFilter(zoneFilter === z.key ? '' : (z.key as string))}
          />
        ))}

        <span className="ml-3 text-[12px]" style={{ color: 'var(--fg-muted)' }}>
          Criticité
        </span>
        {(['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'] as const).map((c) => (
          <Chip
            key={c}
            testId={`universe-filter-${c.toLowerCase()}`}
            label={CRIT_LABEL[c]}
            color={critColor[c.toLowerCase() as Criticality]}
            active={critFilter.has(c)}
            onClick={() =>
              setCritFilter((prev) => {
                const next = new Set(prev);
                if (next.has(c)) next.delete(c);
                else next.add(c);
                return next;
              })
            }
          />
        ))}
      </div>

      <div className="relative" style={{ height: '68vh' }}>
        <div
          ref={wrapRef}
          className="h-full w-full overflow-hidden rounded-2xl border"
          style={{ borderColor: 'var(--border)', background: 'var(--surface-1)' }}
        >
          {isLoading ? (
            <div className="p-6">
              <Skeleton style={{ height: '100%', minHeight: 320 }} />
            </div>
          ) : isError ? (
            <div className="flex h-full flex-col items-center justify-center gap-3">
              <p className="text-sm" style={{ color: 'var(--fg-secondary)' }}>
                La topologie n'a pas pu être chargée.
              </p>
              <Btn label="Réessayer" onClick={() => void refetch()} />
            </div>
          ) : nodes.length === 0 ? (
            <div className="flex h-full flex-col items-center justify-center gap-2">
              <Network size={32} style={{ color: 'var(--fg-muted)' }} />
              <p className="text-sm" style={{ color: 'var(--fg-secondary)' }}>
                Aucun actif à cartographier.
              </p>
              <p className="text-[12px]" style={{ color: 'var(--fg-muted)' }}>
                Ajoutez des actifs et des dépendances pour voir la topologie.
              </p>
            </div>
          ) : (
            <canvas
              ref={canvasRef}
              className="h-full w-full touch-none"
              style={{ cursor: dragRef.current?.node ? 'grabbing' : 'grab' }}
              onPointerDown={onPointerDown}
              onPointerMove={onPointerMove}
              onPointerUp={onPointerUp}
              onPointerCancel={onPointerUp}
              onWheel={onWheel}
            />
          )}
        </div>

        {/* Zoom controls */}
        {nodes.length > 0 && (
          <div className="absolute bottom-3 left-3 flex flex-col gap-1">
            <MapBtn title="Zoom avant" onClick={() => zoomBy(1.25)}>
              <Plus size={15} />
            </MapBtn>
            <MapBtn title="Zoom arrière" onClick={() => zoomBy(1 / 1.25)}>
              <Minus size={15} />
            </MapBtn>
            <MapBtn title="Ajuster à l'écran" onClick={fitToView}>
              <Maximize2 size={15} />
            </MapBtn>
          </div>
        )}

        {/* Legend */}
        {nodes.length > 0 && (
          <div
            className="absolute bottom-3 right-3 rounded-xl border px-3 py-2 text-[11px]"
            style={{
              borderColor: 'var(--border)',
              background: 'var(--surface-2)',
              color: 'var(--fg-muted)',
            }}
          >
            <div className="mb-1.5 font-medium" style={{ color: 'var(--fg-secondary)' }}>
              Relations
            </div>
            {TOPOLOGY_EDGE_TYPES.map((t) => (
              <div key={t} className="flex items-center gap-2">
                <svg width="26" height="6">
                  <line
                    x1="0"
                    y1="3"
                    x2="26"
                    y2="3"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeDasharray={EDGE_DASH[t].join(' ') || undefined}
                  />
                </svg>
                {EDGE_LABELS[t]}
              </div>
            ))}
          </div>
        )}

        {/* Detail panel */}
        {selected && (
          <aside
            className="absolute right-3 top-3 w-72 rounded-2xl border p-4 shadow-lg"
            style={{ borderColor: 'var(--border)', background: 'var(--surface-2)' }}
          >
            <div className="flex items-start justify-between gap-2">
              <div>
                <h3 className="text-[15px] font-semibold" style={{ color: 'var(--fg-primary)' }}>
                  {selected.node.name}
                </h3>
                <p className="text-[12px]" style={{ color: 'var(--fg-muted)' }}>
                  {selected.node.category
                    ? CATEGORY_LABELS[selected.node.category as AssetCategory]
                    : (selected.node.type ?? 'Non typé')}
                </p>
              </div>
              <button onClick={() => setSelected(null)} aria-label="Fermer">
                <X size={16} style={{ color: 'var(--fg-muted)' }} />
              </button>
            </div>

            <div className="mt-3 space-y-2 text-[12.5px]" style={{ color: 'var(--fg-secondary)' }}>
              <Row label="Criticité">
                <CritBadge
                  crit={
                    ((selected.node.criticality ?? 'LOW') as string).toLowerCase() as Criticality
                  }
                />
              </Row>
              <Row label="Zone">{selected.zone}</Row>
              <Row label="Exposé Internet">{selected.node.internet_exposed ? 'Oui' : 'Non'}</Row>
              <Row label="Liens">{selected.node.degree ?? 0}</Row>
              <Row label="Risques">{selected.node.risk_count ?? 0}</Row>
              <Row label="Vulnérabilités ouvertes">{selected.node.vuln_count ?? 0}</Row>
            </div>

            <button
              onClick={() => setChainOrigin(chainOrigin === selected.id ? null : selected.id)}
              className="mt-3 flex w-full items-center justify-center gap-1.5 rounded-lg px-3 py-2 text-[13px] font-medium"
              style={{
                background: chainOrigin === selected.id ? 'var(--surface-1)' : 'var(--accent-soft)',
                color: chainOrigin === selected.id ? 'var(--fg-secondary)' : 'var(--accent)',
                border: '1px solid var(--border)',
              }}
            >
              <Crosshair size={14} />
              {chainOrigin === selected.id ? 'Masquer la chaîne' : 'Si cet actif est compromis…'}
            </button>

            {chainOrigin === selected.id && chainCounts ? (
              <p className="mt-2 text-[12px]" style={{ color: 'var(--fg-muted)' }}>
                <strong style={{ color: 'var(--critical)' }}>{chainCounts.impacted}</strong> actif
                {chainCounts.impacted > 1 ? 's' : ''} impacté
                {chainCounts.impacted > 1 ? 's' : ''} ·{' '}
                <strong style={{ color: 'var(--high)' }}>{chainCounts.reachable}</strong>{' '}
                atteignable{chainCounts.reachable > 1 ? 's' : ''} depuis lui.
              </p>
            ) : null}

            <DependencyEditor
              assetId={selected.id}
              assetName={selected.node.name ?? ''}
              nodes={nodes}
              dependencies={dependencies}
              canEdit={canEdit}
              busy={createDependency.isPending || deleteDependency.isPending}
              onCreate={async (targetId, type) => {
                try {
                  await createDependency.mutateAsync({
                    source_asset_id: selected.id,
                    target_asset_id: targetId,
                    type,
                  });
                  await refetch();
                  toast.success('Dépendance ajoutée.');
                } catch (err) {
                  toast.error(apiErrorMessage(err) || "La dépendance n'a pas pu être ajoutée.");
                }
              }}
              onDelete={async (id) => {
                try {
                  await deleteDependency.mutateAsync(id);
                  await refetch();
                } catch (err) {
                  toast.error(apiErrorMessage(err) || "La dépendance n'a pas pu être retirée.");
                }
              }}
            />

            <a
              href={`/assets?focus=${selected.id}`}
              className="mt-2 block text-center text-[12px] underline"
              style={{ color: 'var(--fg-muted)' }}
            >
              Ouvrir la fiche de l'actif
            </a>
          </aside>
        )}
      </div>
    </PageFrame>
  );
}

/**
 * Add and remove dependency edges from inside the detail panel. This is where
 * the old Asset Universe's side editor lived; it moved here with the graph,
 * because the moment you notice a missing dependency is while looking at one.
 */
function DependencyEditor({
  assetId,
  assetName,
  nodes,
  dependencies,
  canEdit,
  busy,
  onCreate,
  onDelete,
}: {
  assetId: string;
  assetName: string;
  nodes: TopologyNode[];
  dependencies: AssetDependency[];
  canEdit: boolean;
  busy: boolean;
  onCreate: (targetId: string, type: DependencyType) => void | Promise<void>;
  onDelete: (id: string) => void | Promise<void>;
}) {
  const [targetId, setTargetId] = useState('');
  const [type, setType] = useState<DependencyType>('depends_on');

  const nameOf = (id: string) => nodes.find((n) => n.id === id)?.name ?? id.slice(0, 8);
  const outgoing = dependencies.filter((d) => d.source_asset_id === assetId);
  const incoming = dependencies.filter((d) => d.target_asset_id === assetId);

  const selCls = 'w-full rounded-lg border px-2 py-1.5 text-[12px]';
  const selSty = {
    background: 'var(--surface-1)',
    borderColor: 'var(--border)',
    color: 'var(--fg-primary)',
  } as const;

  return (
    <div className="mt-3 border-t pt-3" style={{ borderColor: 'var(--border)' }}>
      <div
        className="mb-1.5 text-[11px] font-semibold uppercase"
        style={{ color: 'var(--fg-muted)' }}
      >
        Dépendances
      </div>

      <div className="max-h-32 space-y-1 overflow-y-auto">
        {outgoing.map((d) => (
          <DepRow
            key={d.id}
            text={`→ ${nameOf(d.target_asset_id as string)}`}
            sub={d.type as string}
            canEdit={canEdit}
            onDelete={() => void onDelete(d.id as string)}
          />
        ))}
        {incoming.map((d) => (
          <DepRow
            key={d.id}
            text={`← ${nameOf(d.source_asset_id as string)}`}
            sub={d.type as string}
            canEdit={canEdit}
            onDelete={() => void onDelete(d.id as string)}
          />
        ))}
        {outgoing.length === 0 && incoming.length === 0 ? (
          <p className="text-[12px]" style={{ color: 'var(--fg-muted)' }}>
            Aucune dépendance enregistrée.
          </p>
        ) : null}
      </div>

      {canEdit ? (
        <div className="mt-2 space-y-1.5">
          <select
            className={selCls}
            style={selSty}
            value={targetId}
            onChange={(e) => setTargetId(e.target.value)}
          >
            <option value="">Cible…</option>
            {nodes
              .filter((n) => n.id !== assetId)
              .map((n) => (
                <option key={n.id} value={n.id as string}>
                  {n.name}
                </option>
              ))}
          </select>
          <select
            className={selCls}
            style={selSty}
            value={type}
            onChange={(e) => setType(e.target.value as DependencyType)}
          >
            {DEPENDENCY_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
          <button
            type="button"
            disabled={!targetId || busy}
            onClick={() => {
              void onCreate(targetId, type);
              setTargetId('');
            }}
            className="w-full rounded-lg px-2 py-1.5 text-[12px] font-medium disabled:opacity-40"
            style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
          >
            {busy ? 'En cours…' : `${assetName} ${type} …`}
          </button>
        </div>
      ) : null}
    </div>
  );
}

function DepRow({
  text,
  sub,
  canEdit,
  onDelete,
}: {
  text: string;
  sub: string;
  canEdit: boolean;
  onDelete: () => void;
}) {
  return (
    <div className="flex items-center justify-between gap-2 text-[12px]">
      <span style={{ color: 'var(--fg-secondary)' }}>
        {text} <span style={{ color: 'var(--fg-muted)' }}>({sub})</span>
      </span>
      {canEdit ? (
        <button onClick={onDelete} aria-label="Retirer" style={{ color: 'var(--fg-muted)' }}>
          <X size={12} />
        </button>
      ) : null}
    </div>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span style={{ color: 'var(--fg-muted)' }}>{label}</span>
      <span className="font-medium">{children}</span>
    </div>
  );
}

function MapBtn({
  children,
  onClick,
  title,
}: {
  children: React.ReactNode;
  onClick: () => void;
  title: string;
}) {
  return (
    <button
      type="button"
      title={title}
      onClick={onClick}
      className="flex h-8 w-8 items-center justify-center rounded-lg border"
      style={{
        borderColor: 'var(--border)',
        background: 'var(--surface-2)',
        color: 'var(--fg-secondary)',
      }}
    >
      {children}
    </button>
  );
}
