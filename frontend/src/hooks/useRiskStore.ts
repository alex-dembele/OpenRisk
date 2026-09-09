// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { create } from 'zustand';
import { registerTenantStore } from '../lib/sessionScope';
import { api } from '../lib/api';
import { riskService } from '../services/riskService';
import type { Asset } from './useAssetStore';
import type { RiskControlMapping } from '../services/taxonomyService';
import type { RiskState } from '../features/risks/useLifecycle';

export interface Mitigation {
  id: string;
  title: string;
  status: 'PLANNED' | 'IN_PROGRESS' | 'DONE';
  progress: number;
  assignee?: string;
}

// The six ISO 31000 lifecycle phases. DERIVED server-side from lifecycle_state
// and kept only so existing filters and pills keep working — nothing writes them.
export type RiskPhase =
  'identified' | 'analyzed' | 'evaluated' | 'treated' | 'monitored' | 'closed';

export interface Risk {
  id: string;
  title: string;
  description: string;
  score: number;
  impact: number;
  probability: number;
  status: string;
  // --- Classification: three separate concepts (see services/taxonomyService).
  /** Free text, user-authored → colonne « Étiquettes ». */
  tags: string[];
  /** The tenant's CONTROLLED vocabulary → colonne « Catégorie ». */
  category_id?: string | null;
  category?: { id: string; name: string; slug: string; color: string } | null;
  /** Real compliance references → colonne « Référentiel ». */
  control_mappings?: RiskControlMapping[];
  assets?: Asset[]; // Important pour l'association Risk-Asset
  /** @deprecated free-text framework names, frozen by migration 0046. */
  frameworks?: string[];
  // --- Ownership (migration 0044): responsable / exécutant / validateur.
  owner_id?: string | null;
  assignee_id?: string | null;
  reviewer_id?: string | null;
  owner_email?: string;
  assignee_email?: string;
  reviewer_email?: string;
  /** @deprecated superseded by assignee_id. */
  assigned_to?: string;
  source: string; // Important pour l'étape d'intégration (THEHIVE, etc.)
  /**
   * The CVE this risk came from, when the CTI engine or the vuln→risk rule
   * raised it (domain.Risk.SourceCVEID). The drawer's CTI tab uses it to fetch
   * the real advisory; null on a manually-entered risk.
   */
  source_cve_id?: string | null;
  mitigations?: Mitigation[]; // Important pour le drawer de détails
  /**
   * Server-computed count of this risk's mitigation plans. The register and the
   * drawer derive "Créer une mitigation" vs "Voir les mitigations (n)" from it,
   * so the button reflects what EXISTS rather than what happened in this
   * session — the previous version reverted to "Créer" on every reload.
   */
  mitigations_count?: number;
  created_at?: string;
  updated_at?: string;
  // Server-computed severity band (ScoreWorker), derived from score via the
  // Score Engine bands. This is the source of truth for severity. `level` is a
  // legacy field that is not reliably populated — prefer `criticality`.
  criticality?: string;
  /** @deprecated use `criticality` — `level` is not reliably populated. */
  level?: 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW';
  // The single canonical lifecycle. status and lifecycle_phase are derived from
  // it server-side; this is the one that means anything.
  lifecycle_state?: RiskState;
  /** @deprecated derived from lifecycle_state. */
  lifecycle_phase?: RiskPhase;
  // Cyber Risk Quantification (CRQ). Inputs in XAF; ALE returned in XAF + USD.
  sle_xaf?: number | null;
  aro?: number | null;
  ale_xaf?: number;
  ale_usd?: number;
  ale_basis?: 'explicit' | 'reference';
  // Full financial-quantification drivers (spec §9), all XAF, all optional.
  downtime_hours?: number | null;
  hourly_downtime_cost_xaf?: number | null;
  data_loss_cost_xaf?: number | null;
  fines_xaf?: number | null;
  other_direct_cost_xaf?: number | null;
  remediation_cost_xaf?: number | null;
  mitigation_effectiveness?: number | null;
}

interface RiskFetchParams {
  q?: string;
  status?: string;
  min_score?: number;
  max_score?: number;
  tag?: string;
  sort_by?: string;
  sort_dir?: 'asc' | 'desc';
}

interface RiskFilters {
  q?: string;
  status?: string;
  framework?: string;
  assignedTo?: string;
  createdBy?: string;
  source?: string;
  tag?: string;
  minScore?: number;
  maxScore?: number;
  dateFrom?: string;
  dateTo?: string;
}

interface RiskStore {
  risks: Risk[];
  isLoading: boolean;
  /** True when the last fetch failed — the register renders a retry state
   *  instead of an empty table pretending the tenant has no risks. */
  error: boolean;
  // pagination
  total: number;
  page: number;
  pageSize: number;
  setPage: (p: number) => Promise<void>;
  // selected risk for global drawer
  selectedRisk?: Risk | null;
  setSelectedRisk: (r: Risk | null) => void;

  // filters & selection for bulk actions
  filters: RiskFilters;
  setFilters: (patch: Partial<RiskFilters>) => void;
  clearFilters: () => void;

  selectedIds: string[];
  setSelectedIds: (ids: string[]) => void;
  toggleSelection: (id: string) => void;
  clearSelection: () => void;

  // bulk operations
  /**
   * Delete a selection in one governed request. Rejects — leaving the store
   * untouched — when the server refuses the batch; there is no partial outcome
   * to report, by design (D-036).
   */
  bulkDelete: (ids: string[]) => Promise<void>;

  // import / export helpers
  importRisks: (file: File) => Promise<void>;
  exportRisks: (params?: Record<string, any>) => Promise<Blob>;

  // realtime
  startSSE: (url?: string) => void;
  stopSSE: () => void;

  fetchRisks: (params?: RiskFetchParams & { page?: number; limit?: number }) => Promise<void>;
  createRisk: (payload: any) => Promise<void>;
  updateRisk: (id: string, payload: any) => Promise<void>;
  transitionPhase: (id: string, phase: RiskPhase, note?: string) => Promise<Risk>;
  deleteRisk: (id: string) => Promise<void>;
}

// --- STORE ZUSTAND ---

/** The state a fresh session must start from — see the registration below. */
const RISK_STORE_INITIAL = {
  risks: [] as Risk[],
  isLoading: false,
  error: false,
  total: 0,
  page: 1,
  pageSize: 20,
  selectedRisk: null as Risk | null,
  filters: {} as RiskFilters,
  selectedIds: [] as string[],
};

export const useRiskStore = create<RiskStore>((set, get) => ({
  ...RISK_STORE_INITIAL,
  setSelectedRisk: (r: Risk | null) => set({ selectedRisk: r }),

  setPage: async (p: number) => {
    set({ page: p });
    // Re-fetch with current filters (if any)
    await get().fetchRisks({ page: p, limit: get().pageSize });
  },

  fetchRisks: async (params) => {
    set({ isLoading: true, error: false });
    try {
      const response = await api.get('/risks', { params });

      // Support new paginated response: { items: [...], total: number }
      if (response.data && response.data.items) {
        set({ risks: response.data.items, total: response.data.total || 0 });
      } else if (Array.isArray(response.data)) {
        set({ risks: response.data, total: response.data.length });
      } else {
        // Fallback
        set({ risks: [], total: 0 });
      }
    } catch (err) {
      console.error('Failed to fetch risks', err);
      // A failed fetch is NOT an empty register: surface it so the table can
      // offer a retry instead of showing a reassuring "no risks yet".
      set({ risks: [], total: 0, error: true });
    } finally {
      set({ isLoading: false });
    }
  },
  createRisk: async (payload) => {
    set({ isLoading: true });
    // optimistic create: add a temporary item to the list
    const tempId = `tmp-${Date.now()}`;
    const optimistic: Risk = {
      id: tempId,
      title: payload.title || 'Nouvel élément',
      description: payload.description || '',
      score: payload.score ?? 0,
      impact: payload.impact ?? 0,
      probability: payload.probability ?? 0,
      status: payload.status || 'DRAFT',
      tags: payload.tags || [],
      assets: undefined,
      source: payload.source || '',
      mitigations: [],
    };

    set((state) => ({ risks: [optimistic, ...state.risks], total: state.total + 1 }));
    try {
      const response = await api.post('/risks', payload);
      const created = response.data;
      // replace temp item with created item from server
      set((state) => ({ risks: state.risks.map((r) => (r.id === tempId ? created : r)) }));
    } catch (err) {
      // rollback optimistic add
      set((state) => ({
        risks: state.risks.filter((r) => r.id !== tempId),
        total: Math.max(0, state.total - 1),
      }));
      console.error('Failed to create risk', err);
      throw err;
    } finally {
      set({ isLoading: false });
    }
  },
  updateRisk: async (id, payload) => {
    set({ isLoading: true });
    const prev = get().risks.find((r) => r.id === id);
    // apply optimistic patch
    set((state) => ({ risks: state.risks.map((r) => (r.id === id ? { ...r, ...payload } : r)) }));
    try {
      const response = await api.patch(`/risks/${id}`, payload);
      const updated = response.data;
      set((state) => ({ risks: state.risks.map((r) => (r.id === id ? updated : r)) }));
    } catch (err) {
      // rollback
      if (prev) set((state) => ({ risks: state.risks.map((r) => (r.id === id ? prev : r)) }));
      console.error('Failed to update risk', err);
      throw err;
    } finally {
      set({ isLoading: false });
    }
  },
  transitionPhase: async (id, phase, note) => {
    const prev = get().risks.find((r) => r.id === id);
    // Optimistic: reflect the new phase immediately in the register.
    set((state) => ({
      risks: state.risks.map((r) => (r.id === id ? { ...r, lifecycle_phase: phase } : r)),
    }));
    try {
      const response = await api.post(`/risks/${id}/transition`, { phase, note });
      const updated = response.data as Risk;
      set((state) => ({ risks: state.risks.map((r) => (r.id === id ? updated : r)) }));
      return updated;
    } catch (err) {
      if (prev) set((state) => ({ risks: state.risks.map((r) => (r.id === id ? prev : r)) }));
      console.error('Failed to transition risk phase', err);
      throw err;
    }
  },
  deleteRisk: async (id) => {
    set({ isLoading: true });
    const prevList = get().risks;
    const prevTotal = get().total;
    // optimistic remove
    set((state) => ({
      risks: state.risks.filter((r) => r.id !== id),
      total: Math.max(0, state.total - 1),
    }));
    try {
      await api.delete(`/risks/${id}`);
    } catch (err) {
      // rollback
      set({ risks: prevList, total: prevTotal });
      console.error('Failed to delete risk', err);
      throw err;
    } finally {
      set({ isLoading: false });
    }
  },

  // filters & selection implementation
  setFilters: (patch) => set((state) => ({ filters: { ...state.filters, ...patch } })),
  clearFilters: () => set({ filters: {} }),

  setSelectedIds: (ids) => set({ selectedIds: ids }),
  toggleSelection: (id) =>
    set((state) => ({
      selectedIds: state.selectedIds.includes(id)
        ? state.selectedIds.filter((x) => x !== id)
        : [...state.selectedIds, id],
    })),
  clearSelection: () => set({ selectedIds: [] }),

  // bulk operations
  //
  // ONE request, not one per row (#598). `POST /risks/bulk` is transactional and
  // audited (#581): the batch either applies to every id or to none, and each
  // deleted risk gets an audit entry naming the actor. The previous
  // implementation fanned out `DELETE /risks/:id` through `Promise.all` under a
  // comment claiming "backend may not provide bulk delete endpoint" — it does,
  // and the fan-out produced N unattributable mutations that a rejection partway
  // through left half-applied.
  bulkDelete: async (ids) => {
    set({ isLoading: true });
    const prev = get().risks;
    const prevTotal = get().total;
    const prevSelection = get().selectedIds;
    // Optimistic (ABSOLUTE RULE 10); restored byte-for-byte below if the server
    // refuses, because under all-or-nothing a refusal means nothing was written.
    set((state) => ({
      risks: state.risks.filter((r) => !ids.includes(r.id)),
      total: Math.max(0, state.total - ids.length),
      selectedIds: [],
    }));
    try {
      await riskService.bulkAction({ type: 'delete', risk_ids: ids });
    } catch (err) {
      set({ risks: prev, total: prevTotal, selectedIds: prevSelection });
      throw err;
    } finally {
      set({ isLoading: false });
    }
  },

  // import/export
  importRisks: async (file) => {
    set({ isLoading: true });
    try {
      const fd = new FormData();
      fd.append('file', file);
      await api.post('/risks/import', fd, { headers: { 'Content-Type': 'multipart/form-data' } });
      // refresh
      await get().fetchRisks({ page: get().page, limit: get().pageSize });
    } catch (err) {
      console.error('importRisks failed', err);
      throw err;
    } finally {
      set({ isLoading: false });
    }
  },

  exportRisks: async (params) => {
    try {
      const res = await api.get('/risks/export', { params, responseType: 'blob' });
      return res.data as Blob;
    } catch (err) {
      console.error('exportRisks failed', err);
      throw err;
    }
  },

  // SSE realtime support
  startSSE: (url = '/api/v1/risks/events') => {
    // avoid multiple event sources
    // store eventSource on closure
    // @ts-ignore - keep internal ref
    if ((get() as any)._es) return;
    try {
      const es = new EventSource(url);
      (set as any)((state: unknown) => state); // noop to satisfy linter
      // save reference
      (get() as any)._es = es;
      es.onmessage = (evt) => {
        try {
          const payload = JSON.parse(evt.data);
          const type = payload.type || payload.event || '';
          const data = payload.data || payload;
          if (type === 'risk.created') {
            set((state) => ({ risks: [data, ...state.risks], total: state.total + 1 }));
          } else if (type === 'risk.updated' || type === 'risk.score_updated') {
            set((state) => ({
              risks: state.risks.map((r) => (r.id === data.id ? { ...r, ...data } : r)),
            }));
          } else if (type === 'risk.deleted') {
            set((state) => ({
              risks: state.risks.filter((r) => r.id !== data.id),
              total: Math.max(0, state.total - 1),
            }));
          }
        } catch (e) {
          // ignore parse errors
        }
      };
      es.onerror = () => {
        // close on error to avoid retry storms
        try {
          es.close();
        } catch {}
        (get() as any)._es = null;
      };
    } catch (err) {
      console.error('startSSE failed', err);
    }
  },

  stopSSE: () => {
    try {
      const ref = (get() as any)._es as EventSource | undefined | null;
      if (ref) {
        ref.close();
        (get() as any)._es = null;
      }
    } catch (err) {
      console.error('stopSSE failed', err);
    }
  },
}));

// The register holds one tenant's risks. Signing out — or in as somebody else —
// must not leave them on screen for the next person (W0-05 / D9). The SSE stream
// is torn down first: a live connection opened under the previous session would
// otherwise keep pushing into the cleared store.
registerTenantStore(useRiskStore, { ...RISK_STORE_INITIAL }, () =>
  useRiskStore.getState().stopSSE(),
);
