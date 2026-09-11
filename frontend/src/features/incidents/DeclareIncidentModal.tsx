// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// "Déclarer un incident".
//
// Declaring is the one moment where the person knows things the platform never
// will: which assets are involved, which risk this finally realises, and who
// needs to hear about it in the next five minutes. The form asks for exactly
// those, and nothing that could be derived later.

import { useMemo, useState } from 'react';
import { toast } from 'sonner';
import { Siren, X, Plus, Trash2, AlertTriangle } from 'lucide-react';
import { Btn } from '../../shared/ui';
import { useUIStore } from '../../store/uiStore';
import { useIncidents } from './useIncidents';
import { apiErrorMessage } from '../../lib/apiError';
import { SEVERITIES, SEV, TYPES } from './incidentMeta';
import { useRiskStore } from '../../hooks/useRiskStore';
import { useAssetStore } from '../../hooks/useAssetStore';
import { useQuery } from '@tanstack/react-query';
import { ownershipService, type Assignee } from '../../services/ownershipService';
import type { IncidentSeverity, IncidentStakeholder } from './incidentService';

const CHANNELS = ['in_app', 'email', 'slack', 'teams', 'sms'] as const;

export function DeclareIncidentModal({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated?: (id: number) => void;
}) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { createIncident } = useIncidents();

  const risks = useRiskStore((s) => s.risks);
  const assets = useAssetStore((s) => s.assets);
  // The same list the API validates against — the picker cannot offer somebody
  // the server would then refuse.
  const { data: assignable } = useQuery({
    queryKey: ['ownership', 'assignable', 'incidents'],
    queryFn: () => ownershipService.listAssignable({}),
  });
  const users: Assignee[] = assignable?.users ?? [];

  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [severity, setSeverity] = useState<IncidentSeverity>('high');
  const [type, setType] = useState(TYPES[0]);
  const [riskIDs, setRiskIDs] = useState<string[]>([]);
  const [assetIDs, setAssetIDs] = useState<string[]>([]);
  const [stakeholders, setStakeholders] = useState<IncidentStakeholder[]>([
    { role: 'admin', channels: ['in_app', 'email'] },
  ]);
  const [saving, setSaving] = useState(false);

  const canSubmit = title.trim().length > 2 && description.trim().length > 2;

  // A critical incident carries a commitment; say so before the click, not after.
  const criticalWarning = severity === 'critical';

  const toggleIn = (list: string[], set: (v: string[]) => void, id: string) =>
    set(list.includes(id) ? list.filter((x) => x !== id) : [...list, id]);

  const submit = async () => {
    if (!canSubmit) return;
    setSaving(true);
    try {
      const inc = await createIncident.mutateAsync({
        title: title.trim(),
        description: description.trim(),
        incident_type: type,
        severity,
        source: 'internal',
        reported_by: '', // stamped server-side from the token
        risk_ids: riskIDs,
        asset_ids: assetIDs,
        stakeholders: stakeholders.filter((s) => s.role || s.user_id),
      });
      toast.success(
        tr(
          'Incident déclaré — les parties prenantes sont notifiées.',
          'Incident declared — stakeholders notified.',
        ),
      );
      onCreated?.(inc.id);
      onClose();
    } catch (e) {
      // Show the server's reason. "La déclaration a échoué" on its own tells
      // the user nothing and tells us nothing either.
      toast.error(apiErrorMessage(e) || tr('La déclaration a échoué', 'The declaration failed'));
    } finally {
      setSaving(false);
    }
  };

  const field = 'w-full px-3 py-2 rounded-[9px] text-[13px] outline-none';
  const fieldStyle = {
    border: '1px solid var(--border-strong)',
    background: 'var(--bg)',
    color: 'var(--fg-primary)',
  } as const;

  const sortedRisks = useMemo(() => risks.slice(0, 200), [risks]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ background: 'rgba(0,0,0,.4)' }}
    >
      {/* Announced as a dialog and named by its heading; the incident list
          behind it stays in the accessibility tree otherwise. */}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="declare-incident-title"
        className="w-full max-w-[640px] max-h-[90vh] flex flex-col rounded-[14px] or-scalein"
        style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)' }}
      >
        <div
          className="px-5 py-4 flex items-center justify-between shrink-0"
          style={{ borderBottom: '1px solid var(--border)' }}
        >
          <h2
            id="declare-incident-title"
            className="text-[15px] font-bold inline-flex items-center gap-2"
            style={{ color: 'var(--fg-primary)' }}
          >
            <Siren size={16} style={{ color: 'var(--critical)' }} />
            {tr('Déclarer un incident', 'Declare an incident')}
          </h2>
          <button onClick={onClose} className="p-1.5 rounded-lg" aria-label={tr('Fermer', 'Close')}>
            <X size={18} />
          </button>
        </div>

        <div className="p-5 space-y-4 overflow-y-auto">
          <label className="block">
            <span className="text-[12px] font-semibold" style={{ color: 'var(--fg-secondary)' }}>
              {tr('Que se passe-t-il ?', 'What is happening?')}
            </span>
            <input
              autoFocus
              className={field}
              style={fieldStyle}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder={tr(
                'Ex : exfiltration suspectée depuis le serveur de facturation',
                'e.g. suspected exfiltration from the billing server',
              )}
            />
          </label>

          <label className="block">
            <span className="text-[12px] font-semibold" style={{ color: 'var(--fg-secondary)' }}>
              {tr('Ce que l’on sait pour l’instant', 'What we know so far')}
            </span>
            <textarea
              rows={3}
              className={field}
              style={fieldStyle}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={tr(
                'Les faits, pas les hypothèses. Elles iront dans le post-mortem.',
                'Facts, not theories. Those belong in the post-mortem.',
              )}
            />
          </label>

          <div className="grid grid-cols-2 gap-3">
            <label className="block">
              <span className="text-[12px] font-semibold" style={{ color: 'var(--fg-secondary)' }}>
                {tr('Gravité', 'Severity')}
              </span>
              <select
                className={field}
                style={fieldStyle}
                value={severity}
                onChange={(e) => setSeverity(e.target.value as IncidentSeverity)}
              >
                {SEVERITIES.map((s) => (
                  <option key={s} value={s}>
                    {lang === 'fr' ? SEV[s].fr : SEV[s].en}
                  </option>
                ))}
              </select>
            </label>
            <label className="block">
              <span className="text-[12px] font-semibold" style={{ color: 'var(--fg-secondary)' }}>
                {tr('Type', 'Type')}
              </span>
              <select
                className={field}
                style={fieldStyle}
                value={type}
                onChange={(e) => setType(e.target.value)}
              >
                {TYPES.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
            </label>
          </div>

          {criticalWarning && (
            <div
              className="rounded-[10px] p-3 flex items-start gap-2.5 text-[12.5px]"
              style={{
                background: 'color-mix(in srgb, var(--critical) 8%, transparent)',
                border: '1px solid color-mix(in srgb, var(--critical) 35%, transparent)',
                color: 'var(--fg-primary)',
              }}
            >
              <AlertTriangle
                size={15}
                style={{ color: 'var(--critical)' }}
                className="shrink-0 mt-0.5"
              />
              <span>
                {tr(
                  'Un incident critique ne pourra pas être clôturé tant que son post-mortem n’est pas publié.',
                  'A critical incident cannot be closed until its post-mortem is published.',
                )}
              </span>
            </div>
          )}

          {/* Links. Real ids, so the incident actually joins the register. */}
          <div>
            <span className="text-[12px] font-semibold" style={{ color: 'var(--fg-secondary)' }}>
              {tr('Actifs concernés', 'Assets affected')}
            </span>
            <div
              className="mt-1 max-h-[120px] overflow-y-auto rounded-[9px] p-2 space-y-1"
              style={{ border: '1px solid var(--border)' }}
            >
              {assets.length === 0 && (
                <p className="text-[12px]" style={{ color: 'var(--fg-secondary)' }}>
                  {tr('Aucun actif dans l’inventaire.', 'No assets in the inventory.')}
                </p>
              )}
              {assets.map((a) => (
                <label key={a.id} className="flex items-center gap-2 text-[12.5px]">
                  <input
                    type="checkbox"
                    checked={assetIDs.includes(a.id)}
                    onChange={() => toggleIn(assetIDs, setAssetIDs, a.id)}
                  />
                  <span style={{ color: 'var(--fg-primary)' }}>{a.name}</span>
                  <span style={{ color: 'var(--fg-secondary)' }}>· {a.type}</span>
                </label>
              ))}
            </div>
          </div>

          <div>
            <span className="text-[12px] font-semibold" style={{ color: 'var(--fg-secondary)' }}>
              {tr('Risques réalisés', 'Risks this realises')}
            </span>
            <p className="text-[11.5px] mb-1" style={{ color: 'var(--fg-secondary)' }}>
              {tr(
                'Les actions correctives du post-mortem viendront s’y rattacher.',
                'The post-mortem’s corrective actions will attach to it.',
              )}
            </p>
            <div
              className="max-h-[120px] overflow-y-auto rounded-[9px] p-2 space-y-1"
              style={{ border: '1px solid var(--border)' }}
            >
              {sortedRisks.length === 0 && (
                <p className="text-[12px]" style={{ color: 'var(--fg-secondary)' }}>
                  {tr('Aucun risque au registre.', 'No risks in the register.')}
                </p>
              )}
              {sortedRisks.map((r) => (
                <label key={r.id} className="flex items-center gap-2 text-[12.5px]">
                  <input
                    type="checkbox"
                    checked={riskIDs.includes(r.id)}
                    onChange={() => toggleIn(riskIDs, setRiskIDs, r.id)}
                  />
                  <span style={{ color: 'var(--fg-primary)' }}>{r.title}</span>
                </label>
              ))}
            </div>
          </div>

          {/* Stakeholders. Recording the channel per person is what makes
              "notify the stakeholders" a promise rather than a hope. */}
          <div>
            <div className="flex items-center justify-between">
              <span className="text-[12px] font-semibold" style={{ color: 'var(--fg-secondary)' }}>
                {tr('Parties prenantes à notifier', 'Stakeholders to notify')}
              </span>
              <button
                onClick={() => setStakeholders((s) => [...s, { channels: ['in_app', 'email'] }])}
                className="text-[12px] font-semibold inline-flex items-center gap-1"
                style={{ color: 'var(--accent-500)' }}
              >
                <Plus size={13} /> {tr('Ajouter', 'Add')}
              </button>
            </div>
            <div className="mt-1.5 space-y-2">
              {stakeholders.map((sh, i) => (
                <div
                  key={i}
                  className="rounded-[9px] p-2.5 space-y-2"
                  style={{ border: '1px solid var(--border)' }}
                >
                  <div className="flex gap-2">
                    <select
                      className={field}
                      style={fieldStyle}
                      value={sh.user_id ?? ''}
                      onChange={(e) =>
                        setStakeholders((list) =>
                          list.map((x, j) =>
                            j === i ? { ...x, user_id: e.target.value || undefined } : x,
                          ),
                        )
                      }
                    >
                      <option value="">{tr('— par rôle —', '— by role —')}</option>
                      {users.map((u) => (
                        <option key={u.user_id} value={u.user_id}>
                          {u.email}
                        </option>
                      ))}
                    </select>
                    <select
                      className={field}
                      style={fieldStyle}
                      value={sh.role ?? ''}
                      onChange={(e) =>
                        setStakeholders((list) =>
                          list.map((x, j) =>
                            j === i ? { ...x, role: e.target.value || undefined } : x,
                          ),
                        )
                      }
                    >
                      <option value="">{tr('— aucun rôle —', '— no role —')}</option>
                      <option value="admin">admin</option>
                      <option value="rssi">rssi</option>
                      <option value="dsi">dsi</option>
                      <option value="security_analyst">security_analyst</option>
                    </select>
                    <button
                      onClick={() => setStakeholders((list) => list.filter((_, j) => j !== i))}
                      className="w-9 h-9 rounded-[9px] flex items-center justify-center shrink-0"
                      style={{ background: 'var(--bg-hover)', color: 'var(--critical)' }}
                      aria-label={tr('Retirer', 'Remove')}
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                  <div className="flex gap-2 flex-wrap">
                    {CHANNELS.map((ch) => {
                      const on = sh.channels?.includes(ch) ?? false;
                      return (
                        <button
                          key={ch}
                          onClick={() =>
                            setStakeholders((list) =>
                              list.map((x, j) => {
                                if (j !== i) return x;
                                const cur = x.channels ?? [];
                                return {
                                  ...x,
                                  channels: cur.includes(ch)
                                    ? cur.filter((c) => c !== ch)
                                    : [...cur, ch],
                                };
                              }),
                            )
                          }
                          className="h-7 px-2 rounded-[7px] text-[11.5px] font-semibold"
                          style={{
                            background: on
                              ? 'color-mix(in srgb, var(--accent) 14%, transparent)'
                              : 'var(--bg-hover)',
                            color: on ? 'var(--accent)' : 'var(--fg-secondary)',
                          }}
                        >
                          {ch}
                        </button>
                      );
                    })}
                  </div>
                </div>
              ))}
              {stakeholders.length === 0 && (
                <p className="text-[12px]" style={{ color: 'var(--medium)' }}>
                  {tr(
                    'Sans partie prenante, l’alerte partira aux administrateurs.',
                    'With no stakeholder, the alert goes to the administrators.',
                  )}
                </p>
              )}
            </div>
          </div>
        </div>

        <div
          className="px-5 py-4 flex justify-end gap-2 shrink-0"
          style={{ borderTop: '1px solid var(--border)' }}
        >
          <Btn label={tr('Annuler', 'Cancel')} onClick={onClose} />
          <Btn
            label={saving ? tr('Déclaration…', 'Declaring…') : tr('Déclarer', 'Declare')}
            icon={Siren}
            primary
            onClick={submit}
            disabled={!canSubmit || saving}
          />
        </div>
      </div>
    </div>
  );
}
