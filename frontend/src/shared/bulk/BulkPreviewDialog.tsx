// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Governed bulk operations (#582) — the impact preview.
//
// The point of this dialog is that the user stops committing blind. It states
// how many rows were named, how many exist, how many would actually change, and
// shows exactly what the change looks like on up to ten of them, before there
// is anything to confirm.
//
// Everything it renders is the server's answer about the tenant's own rows —
// there is no illustrative content in here.
//
// It renders the phases of useGovernedBulk and nothing else. Focus trapping,
// Escape and the dialog role belong to <Modal>; three states — loading, error,
// empty — are handled here (ABSOLUTE RULES 8 and 9) with skeletons, never a
// spinner over the page.

import { useState } from 'react';
import { AlertTriangle, TriangleAlert } from 'lucide-react';

import { Button, ErrorState, Field, Modal, Skeleton, Textarea } from '../ds';
import { useI18n } from '../../hooks/useI18n';
import type { BulkSampleChange } from '../../services/bulkService';
import type { BulkPhase, UseGovernedBulk } from './useGovernedBulk';

/** Every phase that renders something. `idle` renders nothing at all. */
type ActivePhase = Exclude<BulkPhase, { kind: 'idle' }>;

interface BulkPreviewDialogProps {
  bulk: UseGovernedBulk;
  /** Human name of the register, already translated — "vulnerabilities". */
  entityLabel: string;
}

export function BulkPreviewDialog({ bulk, entityLabel }: BulkPreviewDialogProps) {
  // Unmounting on idle is what resets the justification between interactions:
  // carrying one over would attribute this batch's reason to the next batch's
  // audit rows. A reset effect would do the same thing less reliably.
  if (bulk.phase.kind === 'idle') return null;
  return <Interaction bulk={bulk} phase={bulk.phase} entityLabel={entityLabel} />;
}

function Interaction({
  bulk,
  phase,
  entityLabel,
}: {
  bulk: UseGovernedBulk;
  phase: ActivePhase;
  entityLabel: string;
}) {
  const { t } = useI18n();
  const { confirm, retryPreview, close } = bulk;
  const [justification, setJustification] = useState('');

  const applying = phase.kind === 'applying';
  const destructive = phase.change.action === 'delete';
  const preview = 'preview' in phase ? phase.preview : undefined;
  const nothingToDo = preview !== undefined && (preview.affected ?? 0) === 0;
  const subjectCount = preview?.found ?? (phase.kind === 'previewing' ? phase.count : 0);

  const title = (
    destructive
      ? t('bulk.title.delete', 'Delete {n} {entity}?')
      : t('bulk.title.changeStatus', 'Change the status of {n} {entity}?')
  )
    .replace('{n}', String(subjectCount))
    .replace('{entity}', entityLabel);

  // After a refusal or a failed preview there is nothing to confirm — the only
  // way forward is to ask the server again.
  const mustRepreview = phase.kind === 'preview-failed' || phase.kind === 'refused';

  return (
    <Modal
      open
      onClose={close}
      // Two answers, one of them often destructive: announce the whole dialog
      // rather than waiting for the user to explore it.
      role="alertdialog"
      size="md"
      title={title}
      subtitle={t('bulk.subtitle', 'Nothing has changed yet. This is what would.')}
      // An apply is a server-side transaction; offering an X that cannot stop it
      // would be a lie about what dismissing does.
      dismissable={!applying}
      closeLabel={t('common.cancel', 'Cancel')}
      leading={
        destructive ? (
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-danger-surface text-danger-text">
            <AlertTriangle size={18} aria-hidden="true" />
          </span>
        ) : undefined
      }
      footer={
        <>
          <Button variant="secondary" onClick={close} disabled={applying}>
            {t('common.cancel', 'Cancel')}
          </Button>
          {mustRepreview ? (
            <Button variant="primary" onClick={retryPreview}>
              {t('bulk.previewAgain', 'Preview again')}
            </Button>
          ) : (
            <Button
              variant={destructive ? 'destructive' : 'primary'}
              onClick={() => confirm(justification.trim() || undefined)}
              loading={applying}
              disabled={phase.kind !== 'ready' || nothingToDo}
            >
              {destructive ? t('bulk.confirmDelete', 'Delete') : t('bulk.confirmApply', 'Apply')}
            </Button>
          )}
        </>
      }
    >
      <PhaseBody
        phase={phase}
        entityLabel={entityLabel}
        justification={justification}
        onJustificationChange={setJustification}
      />
    </Modal>
  );
}

function PhaseBody({
  phase,
  entityLabel,
  justification,
  onJustificationChange,
}: {
  phase: ActivePhase;
  entityLabel: string;
  justification: string;
  onJustificationChange: (value: string) => void;
}) {
  const { t } = useI18n();

  /* ------------------------------------------------------------- loading -- */
  if (phase.kind === 'previewing') {
    return (
      <div data-testid="bulk-preview-loading" className="flex flex-col gap-2.5">
        <span className="sr-only" role="status">
          {t('bulk.previewing', 'Working out what would change…')}
        </span>
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-4 w-1/2" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }

  /* --------------------------------------------------------------- error -- */
  if (phase.kind === 'preview-failed') {
    return (
      <ErrorState
        title={t('bulk.previewFailed', 'The preview could not be computed')}
        description={t(
          'bulk.previewFailedHelp',
          'Nothing was changed. Try again, or narrow the selection.',
        )}
        detail={phase.detail}
      />
    );
  }

  if (phase.kind === 'apply-failed') {
    return (
      <ErrorState
        title={t('bulk.applyFailed', 'The action did not run')}
        description={t(
          'bulk.applyFailedHelp',
          'The batch is applied in full or not at all, so nothing was changed. The list has been put back as it was.',
        )}
        detail={phase.detail}
      />
    );
  }

  /* ------------------------------------------------------------- refused -- */
  if (phase.kind === 'refused') {
    return (
      <div
        data-testid="bulk-preview-refused"
        role="alert"
        className="flex gap-2.5 rounded-lg p-3"
        style={{
          background: 'color-mix(in srgb,var(--high) 12%,transparent)',
          color: 'var(--fg-primary)',
        }}
      >
        <TriangleAlert size={18} aria-hidden="true" style={{ color: 'var(--high)' }} />
        <div className="text-[13px] leading-relaxed">
          <p className="font-semibold">
            {t('bulk.refused', 'The selection changed since the preview')}
          </p>
          <p className="text-ink-muted">
            {phase.refusal === 'missing'
              ? t(
                  'bulk.refusedMissing',
                  'Some of the selected rows no longer exist. Nothing was changed.',
                )
              : t(
                  'bulk.refusedMoved',
                  'Someone else edited these rows while you were reading the preview, so the action was refused rather than applied to rows you did not see. Nothing was changed.',
                )}
          </p>
        </div>
      </div>
    );
  }

  /* ------------------------------------------- ready / applying: the facts -- */
  const { preview } = phase;
  const requested = preview.requested ?? 0;
  const found = preview.found ?? 0;
  const affected = preview.affected ?? 0;
  const unchanged = preview.unchanged ?? 0;
  const missing = preview.missing ?? [];
  const changes = preview.sample ?? [];

  return (
    <div className="flex flex-col gap-3.5">
      <dl className="grid grid-cols-3 gap-2" data-testid="bulk-preview-counts">
        <Figure label={t('bulk.selected', 'Selected')} value={requested} />
        <Figure
          label={t('bulk.willChange', 'Will change')}
          value={affected}
          emphasis
          testId="bulk-affected"
        />
        <Figure label={t('bulk.alreadyThere', 'Already as asked')} value={unchanged} />
      </dl>

      {missing.length > 0 && (
        <p
          role="status"
          className="text-[12.5px] text-ink-muted"
          data-testid="bulk-preview-missing"
        >
          {t(
            'bulk.missing',
            '{n} of the {total} selected rows no longer exist and will be refused.',
          )
            .replace('{n}', String(missing.length))
            .replace('{total}', String(requested))}
        </p>
      )}

      {/* Empty state: a preview that would change nothing is a real answer, not
          an error, and the confirm button is disabled behind it. */}
      {found === 0 ? (
        <p className="text-[13px] text-ink-muted" data-testid="bulk-preview-empty">
          {t('bulk.noneFound', 'None of the selected rows exist any more. Nothing would change.')}
        </p>
      ) : affected === 0 ? (
        <p className="text-[13px] text-ink-muted" data-testid="bulk-preview-empty">
          {t(
            'bulk.nothingToDo',
            'None of the selected {entity} would change — they already hold that value.',
          ).replace('{entity}', entityLabel)}
        </p>
      ) : (
        <ChangeList changes={changes} affected={affected} />
      )}

      {found > 0 && affected > 0 && (
        <Field
          label={t('bulk.justification', 'Reason (optional)')}
          description={t(
            'bulk.justificationHelp',
            'Recorded on every audit entry this action writes.',
          )}
        >
          <Textarea
            rows={2}
            maxLength={1000}
            value={justification}
            onChange={(e) => onJustificationChange(e.target.value)}
            disabled={phase.kind === 'applying'}
          />
        </Field>
      )}
    </div>
  );
}

/** The server's bounded list of real rows that would change. */
function ChangeList({ changes, affected }: { changes: BulkSampleChange[]; affected: number }) {
  const { t } = useI18n();
  if (changes.length === 0) return null;

  return (
    <div className="flex flex-col gap-1.5">
      <p className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted">
        {t('bulk.changesTitle', 'What changes')}
      </p>
      <ul
        data-testid="bulk-preview-changes"
        className="flex flex-col gap-1 max-h-56 overflow-y-auto rounded-lg p-1"
        style={{ background: 'var(--bg-subtle)' }}
      >
        {changes.map((change) => (
          <li
            key={change.id}
            className="flex items-baseline justify-between gap-3 px-2 py-1.5 text-[12.5px]"
          >
            <span className="truncate text-ink">{change.label ?? change.id}</span>
            <ChangeSummary change={change} />
          </li>
        ))}
      </ul>
      {affected > changes.length && (
        <p className="text-[12px] text-ink-muted">
          {t('bulk.changesMore', '…and {n} more.').replace(
            '{n}',
            String(affected - changes.length),
          )}
        </p>
      )}
    </div>
  );
}

/**
 * before → after for the fields that move.
 *
 * A delete has no `after`, so it reads as a single word rather than an arrow
 * pointing at nothing.
 */
function ChangeSummary({ change }: { change: BulkSampleChange }) {
  const { t } = useI18n();
  const fields = change.changed_fields ?? [];

  if (fields.length === 0) {
    return (
      <span className="shrink-0 font-semibold" style={{ color: 'var(--critical)' }}>
        {t('bulk.willBeDeleted', 'deleted')}
      </span>
    );
  }

  return (
    <span className="shrink-0 mono text-[11.5px] text-ink-muted">
      {fields.map((field) => (
        <span key={field}>
          {display(change.before?.[field])} →{' '}
          <b className="text-ink">{display(change.after?.[field])}</b>
        </span>
      ))}
    </span>
  );
}

function display(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  return JSON.stringify(value);
}

function Figure({
  label,
  value,
  emphasis,
  testId,
}: {
  label: string;
  value: number;
  emphasis?: boolean;
  testId?: string;
}) {
  return (
    <div className="rounded-lg px-2.5 py-2" style={{ background: 'var(--bg-subtle)' }}>
      <dt className="text-[10.5px] font-semibold uppercase tracking-[.04em] text-ink-muted">
        {label}
      </dt>
      <dd
        data-testid={testId}
        className="mono text-[20px] font-bold"
        style={{ color: emphasis ? 'var(--accent)' : 'var(--fg-primary)' }}
      >
        {value}
      </dd>
    </div>
  );
}
