// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Danger zone (spec §6): the hardened organization erasure. Unlike a bare delete
// button, this flow forces a full export first, exact-name confirmation, an MFA
// code (for enrolled admins), and shows the 30-day cancelable grace window with a
// running countdown. Nothing is destroyed synchronously.

import { useFormat } from '../../hooks/useI18n';
import { useState } from 'react';
import { toast } from 'sonner';
import { AlertTriangle, Download } from 'lucide-react';
import { useAuthStore } from '../../hooks/useAuthStore';
import { orgDeletionService } from '../../services/entitlementService';
import { useOrgDeletion, useRequestOrgDeletion, useCancelOrgDeletion } from './useEntitlements';

export function DangerZonePanel() {
  const fmt = useFormat();
  const user = useAuthStore((s) => s.user);
  const orgName = user?.org_name ?? '';
  const { data: state } = useOrgDeletion();
  const request = useRequestOrgDeletion();
  const cancelDeletion = useCancelOrgDeletion();

  const [confirmName, setConfirmName] = useState('');
  const [mfaCode, setMfaCode] = useState('');
  const [reason, setReason] = useState('');

  const nameOk = confirmName.trim() === orgName.trim() && orgName !== '';

  const submit = async () => {
    try {
      await request.mutateAsync({ confirmName, mfaCode, reason });
      toast.success('Suppression programmée — délai de grâce de 30 jours, annulable.');
      setConfirmName('');
      setMfaCode('');
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string; code?: string } } };
      const code = err.response?.data?.code;
      toast.error(
        code === 'mfa_required'
          ? 'Code MFA invalide ou manquant.'
          : code === 'name_mismatch'
            ? "Le nom saisi ne correspond pas à l'organisation."
            : (err.response?.data?.error ?? 'Suppression impossible.'),
      );
    }
  };

  const cancel = async () => {
    try {
      await cancelDeletion.mutateAsync();
      toast.success('Suppression annulée.');
    } catch {
      toast.error('Annulation impossible.');
    }
  };

  // Pending state — show the countdown + cancel.
  if (state?.pending) {
    return (
      <div
        className="rounded-[16px] p-[22px]"
        style={{
          border: '1px solid rgba(255,69,58,.35)',
          background: 'color-mix(in srgb,var(--critical) 6%,transparent)',
        }}
      >
        <div
          className="flex items-center gap-2 text-[15px] font-semibold mb-2"
          style={{ color: 'var(--critical)' }}
        >
          <AlertTriangle size={18} /> Suppression programmée
        </div>
        <div className="text-[13px] text-ink-soft mb-1">
          L'organisation <strong>{orgName}</strong> sera définitivement supprimée le{' '}
          <strong>
            {state.scheduled_purge_at
              ? fmt.date(state.scheduled_purge_at)
              : ''}
          </strong>
          .
        </div>
        <div className="text-[13px] text-ink-soft mb-4">
          Il reste <strong>{state.days_remaining ?? 0} jour(s)</strong> pour annuler. Un export
          complet a été généré avant la programmation.
        </div>
        <button
          onClick={cancel}
          disabled={cancelDeletion.isPending}
          className="h-[38px] px-4 rounded-[10px] text-[13px] font-semibold text-fg-primary disabled:opacity-50"
          style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
        >
          Annuler la suppression
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Export */}
      <div
        className="rounded-[16px] p-5"
        style={{ background: 'var(--bg-elev)', border: '1px solid var(--border)' }}
      >
        <div className="text-[13px] font-semibold text-ink mb-1">Exporter vos données</div>
        <div className="text-[12.5px] text-ink-soft mb-3 max-w-[60ch]">
          Téléchargez une archive JSON complète de votre organisation (risques, actifs, conformité,
          incidents…). Un export est aussi généré automatiquement avant toute suppression.
        </div>
        <a
          href={orgDeletionService.exportUrl}
          className="inline-flex items-center gap-2 h-9 px-4 rounded-[10px] text-[13px] font-semibold"
          style={{ border: '1px solid var(--border)', color: 'var(--ink)' }}
        >
          <Download size={15} /> Exporter mon organisation
        </a>
      </div>

      {/* Delete */}
      <div
        className="rounded-[16px] p-[22px]"
        style={{
          border: '1px solid rgba(255,69,58,.35)',
          background: 'color-mix(in srgb,var(--critical) 5%,transparent)',
        }}
      >
        <div className="text-[15px] font-semibold mb-1.5" style={{ color: 'var(--critical)' }}>
          Supprimer l'organisation
        </div>
        <div className="text-[13px] text-ink-soft mb-4 max-w-[60ch]">
          Action irréversible après le délai de grâce. Un export est effectué automatiquement, tous
          les administrateurs sont notifiés, et vous disposez de 30 jours pour annuler. Conforme au
          RGPD et à la loi camerounaise 2024/017.
        </div>

        <label className="block text-[12px] text-ink-soft mb-1">
          Tapez le nom exact de l'organisation pour confirmer :{' '}
          <strong className="text-ink">{orgName}</strong>
        </label>
        <input
          value={confirmName}
          onChange={(e) => setConfirmName(e.target.value)}
          placeholder={orgName}
          className="w-full h-10 px-3 rounded-[10px] text-[13px] mb-3 bg-transparent"
          style={{ border: '1px solid var(--border)', color: 'var(--ink)' }}
        />
        <label className="block text-[12px] text-ink-soft mb-1">
          Code MFA (si activé sur votre compte)
        </label>
        <input
          value={mfaCode}
          onChange={(e) => setMfaCode(e.target.value)}
          placeholder="123456"
          inputMode="numeric"
          className="w-full h-10 px-3 rounded-[10px] text-[13px] mb-3 bg-transparent"
          style={{ border: '1px solid var(--border)', color: 'var(--ink)' }}
        />
        <label className="block text-[12px] text-ink-soft mb-1">Motif (optionnel)</label>
        <input
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          className="w-full h-10 px-3 rounded-[10px] text-[13px] mb-4 bg-transparent"
          style={{ border: '1px solid var(--border)', color: 'var(--ink)' }}
        />

        <button
          onClick={submit}
          disabled={!nameOk || request.isPending}
          className="h-[38px] px-4 rounded-[10px] text-[13px] font-semibold disabled:opacity-50 disabled:pointer-events-none"
          style={{
            border: '1px solid rgba(255,69,58,.5)',
            color: 'var(--critical)',
            background: 'color-mix(in srgb,var(--critical) 8%,transparent)',
          }}
        >
          Programmer la suppression (30 j de grâce)
        </button>
      </div>
    </div>
  );
}
