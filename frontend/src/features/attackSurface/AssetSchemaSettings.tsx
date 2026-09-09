// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useMemo, useState } from 'react';
import { ArrowDown, ArrowUp, Fingerprint, Plus, RotateCcw, Save, Trash2 } from 'lucide-react';
import { useToast } from '../../hooks/useToast';
import { useAuthStore } from '../../hooks/useAuthStore';
import { apiErrorMessage } from '../../lib/apiError';
import { useAssetSchemas } from './useAssetSchemas';
import { useI18n } from '../../hooks/useI18n';
import {
  ASSET_CATEGORIES,
  ATTRIBUTE_TYPES,
  ATTRIBUTE_TYPE_LABELS,
  CATEGORY_LABELS,
  isListType,
  type AssetCategory,
  type AttributeDef,
} from './schemaTypes';

const FINGERPRINT_ROLES = ['', 'hostname', 'ip', 'cloud_id', 'cpe'] as const;

const inputCls =
  'w-full rounded-lg border px-2.5 py-1.5 text-sm outline-none focus:ring-2 focus:ring-primary/40';
const inputSty = {
  background: 'var(--surface-2)',
  borderColor: 'var(--border)',
  color: 'var(--fg-primary)',
} as const;

/**
 * The tenant's schema editor: what a Server, a Vendor or a Processing activity
 * means HERE. Editing a schema changes the contract every asset of that category
 * is validated against, so it is admin-gated and saved as a whole.
 */
export default function AssetSchemaSettings() {
  const { t } = useI18n();
  const toast = useToast();
  const isAdmin = useAuthStore((s) => s.hasPermission('*'));
  const { schemas, isLoading, isError, refetch, updateSchema, resetSchema } = useAssetSchemas();

  const [selected, setSelected] = useState<AssetCategory>('server');
  const [draft, setDraft] = useState<AttributeDef[]>([]);
  const [label, setLabel] = useState('');

  const current = useMemo(() => schemas.find((s) => s.category === selected), [schemas, selected]);

  // Re-sync the editable copy when the selected category's schema changes, by
  // adjusting state during render rather than in an effect.
  const [syncedFrom, setSyncedFrom] = useState<typeof current>(undefined);
  if (current !== syncedFrom) {
    setSyncedFrom(current);
    setDraft(current?.attributes ? current.attributes.map((a) => ({ ...a })) : []);
    setLabel(current?.label ?? '');
  }

  const dirty = useMemo(
    () =>
      JSON.stringify(draft) !== JSON.stringify(current?.attributes ?? []) ||
      label !== (current?.label ?? ''),
    [draft, label, current],
  );

  const patch = (i: number, next: Partial<AttributeDef>) =>
    setDraft((d) => d.map((a, idx) => (idx === i ? { ...a, ...next } : a)));

  const move = (i: number, delta: number) =>
    setDraft((d) => {
      const j = i + delta;
      if (j < 0 || j >= d.length) return d;
      const copy = [...d];
      [copy[i], copy[j]] = [copy[j], copy[i]];
      return copy;
    });

  const save = async () => {
    try {
      await updateSchema.mutateAsync({ category: selected, payload: { label, attributes: draft } });
      toast.success('Schéma enregistré.');
    } catch (err) {
      // The server validates the schema as a whole and names what is wrong
      // ("attribute #3: key must be snake_case"). That message is the fix.
      toast.error(apiErrorMessage(err) || "Le schéma n'a pas pu être enregistré.");
    }
  };

  const reset = async () => {
    try {
      await resetSchema.mutateAsync(selected);
      toast.success('Schéma par défaut restauré.');
    } catch (err) {
      toast.error(apiErrorMessage(err) || 'La réinitialisation a échoué.');
    }
  };

  if (isLoading) {
    return <div className="p-6 space-y-3">{skeletons()}</div>;
  }
  if (isError) {
    return (
      <div className="p-6">
        <p className="text-sm" style={{ color: 'var(--fg-secondary)' }}>
          Les schémas n'ont pas pu être chargés.
        </p>
        <button
          onClick={() => refetch()}
          className="mt-3 rounded-lg border px-3 py-1.5 text-sm"
          style={{ borderColor: 'var(--border)', color: 'var(--fg-primary)' }}
        >
          Réessayer
        </button>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-5">
      <header>
        <h1 className="text-xl font-semibold" style={{ color: 'var(--fg-primary)' }}>
          Attributs par catégorie d'actif
        </h1>
        <p className="mt-1 text-sm" style={{ color: 'var(--fg-muted)' }}>
          Ce que vous décrivez ici devient le formulaire de saisie et la validation serveur pour
          chaque actif de cette catégorie.
        </p>
      </header>

      <div className="flex flex-wrap gap-1.5">
        {ASSET_CATEGORIES.map((cat) => {
          const on = cat === selected;
          const customised = schemas.find((s) => s.category === cat)?.customized;
          return (
            <button
              key={cat}
              onClick={() => setSelected(cat)}
              className="rounded-full border px-3 py-1.5 text-sm transition"
              style={{
                borderColor: on ? 'var(--accent)' : 'var(--border)',
                background: on ? 'var(--accent-soft)' : 'transparent',
                color: on ? 'var(--accent)' : 'var(--fg-secondary)',
              }}
            >
              {CATEGORY_LABELS[cat]}
              {customised ? ' •' : ''}
            </button>
          );
        })}
      </div>

      <div
        className="rounded-2xl border p-4 space-y-4"
        style={{ borderColor: 'var(--border)', background: 'var(--surface-1)' }}
      >
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium" style={{ color: 'var(--fg-muted)' }}>
              Nom affiché de la catégorie
            </label>
            <input
              className={inputCls}
              style={{ ...inputSty, minWidth: 260 }}
              value={label}
              disabled={!isAdmin}
              onChange={(e) => setLabel(e.target.value)}
            />
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs" style={{ color: 'var(--fg-muted)' }}>
              {t('common.attributes', { count: draft.length })}
              {current?.customized ? ' · personnalisé' : ' · par défaut'}
              {current?.version ? ` · v${current.version}` : ''}
            </span>
            {isAdmin && (
              <>
                <button
                  onClick={reset}
                  disabled={resetSchema.isPending}
                  className="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm"
                  style={{ borderColor: 'var(--border)', color: 'var(--fg-secondary)' }}
                >
                  <RotateCcw size={14} /> Valeurs par défaut
                </button>
                <button
                  onClick={save}
                  disabled={!dirty || updateSchema.isPending}
                  className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium disabled:opacity-50"
                  style={{ background: 'var(--accent)', color: 'var(--fg-inverse)' }}
                >
                  <Save size={14} />
                  {updateSchema.isPending ? 'Enregistrement…' : 'Enregistrer'}
                </button>
              </>
            )}
          </div>
        </div>

        {!isAdmin && (
          <p className="text-xs" style={{ color: 'var(--fg-muted)' }}>
            Lecture seule — modifier un schéma est une action d'administrateur.
          </p>
        )}

        <div className="space-y-2">
          {draft.map((def, i) => (
            <div
              key={`${def.key}-${i}`}
              className="rounded-xl border p-3"
              style={{ borderColor: 'var(--border)', background: 'var(--surface-2)' }}
            >
              <div className="grid gap-2 sm:grid-cols-12">
                <input
                  className={`${inputCls} sm:col-span-3`}
                  style={inputSty}
                  value={def.key ?? ''}
                  disabled={!isAdmin}
                  placeholder="cle_technique"
                  onChange={(e) => patch(i, { key: e.target.value })}
                />
                <input
                  className={`${inputCls} sm:col-span-3`}
                  style={inputSty}
                  value={def.label ?? ''}
                  disabled={!isAdmin}
                  placeholder="Libellé affiché"
                  onChange={(e) => patch(i, { label: e.target.value })}
                />
                <select
                  className={`${inputCls} sm:col-span-2`}
                  style={inputSty}
                  value={def.type}
                  disabled={!isAdmin}
                  onChange={(e) => patch(i, { type: e.target.value as AttributeDef['type'] })}
                >
                  {ATTRIBUTE_TYPES.map((t) => (
                    <option key={t} value={t}>
                      {ATTRIBUTE_TYPE_LABELS[t]}
                    </option>
                  ))}
                </select>
                <input
                  className={`${inputCls} sm:col-span-2`}
                  style={inputSty}
                  value={def.group ?? ''}
                  disabled={!isAdmin}
                  placeholder="Groupe"
                  onChange={(e) => patch(i, { group: e.target.value })}
                />
                <div className="flex items-center gap-1 sm:col-span-2">
                  <label
                    className="flex items-center gap-1 text-xs"
                    style={{ color: 'var(--fg-secondary)' }}
                  >
                    <input
                      type="checkbox"
                      checked={!!def.required}
                      disabled={!isAdmin}
                      onChange={(e) => patch(i, { required: e.target.checked })}
                    />
                    requis
                  </label>
                  <div className="ml-auto flex">
                    <IconBtn onClick={() => move(i, -1)} disabled={!isAdmin || i === 0}>
                      <ArrowUp size={14} />
                    </IconBtn>
                    <IconBtn
                      onClick={() => move(i, 1)}
                      disabled={!isAdmin || i === draft.length - 1}
                    >
                      <ArrowDown size={14} />
                    </IconBtn>
                    <IconBtn
                      onClick={() => setDraft((d) => d.filter((_, idx) => idx !== i))}
                      disabled={!isAdmin}
                      danger
                    >
                      <Trash2 size={14} />
                    </IconBtn>
                  </div>
                </div>
              </div>

              <div className="mt-2 grid gap-2 sm:grid-cols-12">
                {(def.type === 'enum' || isListType(def.type)) &&
                def.type !== 'ip_list' &&
                def.type !== 'string_list' ? (
                  <input
                    className={`${inputCls} sm:col-span-7`}
                    style={inputSty}
                    value={(def.enum ?? []).join(', ')}
                    disabled={!isAdmin}
                    placeholder="valeurs autorisées, séparées par des virgules"
                    onChange={(e) =>
                      patch(i, {
                        enum: e.target.value
                          .split(',')
                          .map((v) => v.trim())
                          .filter(Boolean),
                      })
                    }
                  />
                ) : (
                  <input
                    className={`${inputCls} sm:col-span-7`}
                    style={inputSty}
                    value={def.help ?? ''}
                    disabled={!isAdmin}
                    placeholder="Aide affichée sous le champ"
                    onChange={(e) => patch(i, { help: e.target.value })}
                  />
                )}
                <label
                  className="flex items-center gap-1.5 text-xs sm:col-span-5"
                  style={{ color: 'var(--fg-muted)' }}
                  title="Marque cet attribut comme signal d'identité pour corréler les vulnérabilités"
                >
                  <Fingerprint size={13} />
                  Empreinte
                  <select
                    className={inputCls}
                    style={inputSty}
                    value={def.fingerprint ?? ''}
                    disabled={!isAdmin}
                    onChange={(e) =>
                      patch(i, { fingerprint: e.target.value as AttributeDef['fingerprint'] })
                    }
                  >
                    {FINGERPRINT_ROLES.map((r) => (
                      <option key={r} value={r}>
                        {r === '' ? 'aucune' : r}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
            </div>
          ))}
        </div>

        {isAdmin && (
          <button
            onClick={() =>
              setDraft((d) => [
                ...d,
                { key: '', label: '', type: 'string', group: 'Général' } as AttributeDef,
              ])
            }
            className="inline-flex items-center gap-1.5 rounded-lg border border-dashed px-3 py-2 text-sm"
            style={{ borderColor: 'var(--border)', color: 'var(--fg-secondary)' }}
          >
            <Plus size={14} /> Ajouter un attribut
          </button>
        )}
      </div>
    </div>
  );
}

function IconBtn({
  children,
  onClick,
  disabled,
  danger,
}: {
  children: React.ReactNode;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="rounded p-1.5 transition disabled:opacity-30"
      style={{ color: danger ? 'var(--critical)' : 'var(--fg-muted)' }}
    >
      {children}
    </button>
  );
}

function skeletons() {
  return [0, 1, 2].map((i) => (
    <div
      key={i}
      className="h-16 animate-pulse rounded-xl"
      style={{ background: 'var(--surface-2)' }}
    />
  ));
}
