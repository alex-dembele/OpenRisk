// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useState } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { X, Server } from 'lucide-react';
import { Button, Field, Input } from '../../shared/ds';
import { useI18n } from '../../hooks/useI18n';
import { useToast } from '../../hooks/useToast';
import { useAssets } from './useAssets';
import { ASSET_CRITICALITIES, ASSET_TYPES } from '../../types/asset';
import { useEscapeToClose } from '../../shared/useBackTo';
import { apiErrorMessage } from '../../lib/apiError';
import { AttributeForm } from '../attackSurface/AttributeForm';
import { useAssetSchemas } from '../attackSurface/useAssetSchemas';
import {
  ASSET_CATEGORIES,
  CATEGORY_LABELS,
  type AssetCategory,
  type AttributeBag,
} from '../attackSurface/schemaTypes';

const schema = z.object({
  name: z.string().min(2),
  type: z.enum(ASSET_TYPES),
  criticality: z.enum(['LOW', 'MEDIUM', 'HIGH', 'CRITICAL']),
  owner: z.string().optional(),
});
type FormValues = z.infer<typeof schema>;

/**
 * Free-text type → typed category. Picking the category for the user (rather
 * than asking twice) is what stops the two vocabularies from disagreeing on the
 * very first screen; they can still override it below.
 */
const CATEGORY_FOR_TYPE: Partial<Record<(typeof ASSET_TYPES)[number], AssetCategory>> = {
  Server: 'server',
  Laptop: 'workstation',
  Application: 'application',
  SaaS: 'application',
  Database: 'database',
  Network: 'network',
  Cloud: 'cloud',
  Storage: 'cloud',
  Supplier: 'vendor',
  Data: 'data_processing',
  User: 'workstation',
};

interface CreateAssetModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const CreateAssetModal = ({ isOpen, onClose }: CreateAssetModalProps) => {
  // Esc closes this overlay (spec §2).
  useEscapeToClose(isOpen, onClose);
  const { t } = useI18n();
  const toast = useToast();
  const { createAsset } = useAssets();

  const { defsFor } = useAssetSchemas();

  const {
    register,
    handleSubmit,
    reset,
    watch,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', type: ASSET_TYPES[0], criticality: 'MEDIUM', owner: '' },
  });

  const selectedType = watch('type');
  // The category defaults from the chosen type but stays overridable: the two
  // vocabularies serve different purposes (display vs. schema selection).
  const [categoryOverride, setCategoryOverride] = useState<AssetCategory | ''>('');
  const category: AssetCategory | '' = categoryOverride || CATEGORY_FOR_TYPE[selectedType] || '';
  const defs = defsFor(category);

  const [attributes, setAttributes] = useState<AttributeBag>({});

  const handleClose = () => {
    reset();
    setAttributes({});
    setCategoryOverride('');
    onClose();
  };

  const onSubmit = async (values: FormValues) => {
    try {
      await createAsset.mutateAsync({
        ...values,
        ...(category ? { category, attributes } : {}),
      });
      toast.success(t('assets.createSuccess'));
      handleClose();
    } catch (err) {
      // The server owns attribute validation, so its message is the specific
      // one ("Nom d'hôte is required") — showing a generic failure instead
      // would hide the only useful part.
      toast.error(apiErrorMessage(err) || t('errors.failedToCreateAsset'));
    }
  };

  return (
    <AnimatePresence>
      {isOpen && (
        <>
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={handleClose}
            className="fixed inset-0 z-40 bg-surface-overlay backdrop-blur-sm"
          />
          <motion.div
            initial={{ opacity: 0, scale: 0.96, y: 40 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.96, y: 40 }}
            transition={{ duration: 0.22, type: 'spring', stiffness: 240 }}
            className="fixed inset-0 z-50 flex items-center justify-center p-4"
          >
            {/* Announced as a dialog and named by its own heading. Without these
                a screen reader walks straight past the form into the inventory
                behind it, which is still in the tree. */}
            <div
              role="dialog"
              aria-modal="true"
              aria-labelledby="create-asset-title"
              className="flex max-h-[90vh] w-full max-w-lg flex-col overflow-hidden rounded-3xl border border-border bg-elevated shadow-card-lg"
            >
              <div className="flex shrink-0 items-center justify-between gap-4 border-b border-border px-6 py-5">
                <div className="flex items-center gap-3">
                  <div className="rounded-2xl bg-primary/10 p-2 text-primary">
                    <Server size={20} />
                  </div>
                  <h2 id="create-asset-title" className="text-xl font-semibold text-ink">
                    {t('assets.createAsset')}
                  </h2>
                </div>
                <button
                  type="button"
                  onClick={handleClose}
                  aria-label={t('common.close')}
                  className="rounded-full p-2 text-ink-soft hover:bg-hover hover:text-ink transition-colors"
                >
                  <X size={20} aria-hidden="true" />
                </button>
              </div>

              <form onSubmit={handleSubmit(onSubmit)} className="flex min-h-0 flex-1 flex-col">
                <div className="flex-1 space-y-5 overflow-y-auto px-6 py-6 scrollbar-thin">
                  <Field
                    label={t('assets.form.name')}
                    message={errors.name?.message}
                    status={errors.name?.message ? 'invalid' : 'default'}
                  >
                    <Input
                      {...register('name')}
                      disabled={isSubmitting}
                      placeholder="Production-DB-01"
                      autoFocus
                    />
                  </Field>

                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-1.5">
                      <label
                        htmlFor="create-asset-type"
                        className="text-xs font-medium text-ink-muted uppercase tracking-wider"
                      >
                        {t('assets.form.type')}
                      </label>
                      <select
                        id="create-asset-type"
                        {...register('type')}
                        disabled={isSubmitting}
                        className="w-full h-10 rounded-lg border border-border bg-elevated px-3 text-sm text-ink outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary transition-all"
                      >
                        {ASSET_TYPES.map((type) => (
                          <option key={type} value={type}>
                            {type}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="space-y-1.5">
                      <label
                        htmlFor="create-asset-criticality"
                        className="text-xs font-medium text-ink-muted uppercase tracking-wider"
                      >
                        {t('assets.form.criticality')}
                      </label>
                      <select
                        id="create-asset-criticality"
                        {...register('criticality')}
                        disabled={isSubmitting}
                        className="w-full h-10 rounded-lg border border-border bg-elevated px-3 text-sm text-ink outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary transition-all"
                      >
                        {ASSET_CRITICALITIES.map((level) => (
                          <option key={level} value={level}>
                            {t(`assets.criticality.${level}`)}
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>

                  <Field label={t('assets.form.owner')}>
                    <Input {...register('owner')} disabled={isSubmitting} placeholder="IT Dept" />
                  </Field>

                  <div className="space-y-1.5">
                    <label
                      htmlFor="create-asset-category"
                      className="text-xs font-medium text-ink-muted uppercase tracking-wider"
                    >
                      {t('assets.form.category', 'Catégorie typée')}
                    </label>
                    <select
                      id="create-asset-category"
                      value={category}
                      disabled={isSubmitting}
                      onChange={(e) => {
                        setCategoryOverride(e.target.value as AssetCategory | '');
                        // The old bag belongs to the old schema; carrying it over
                        // would submit attributes the new category never declared.
                        setAttributes({});
                      }}
                      className="w-full h-10 rounded-lg border border-border bg-elevated px-3 text-sm text-ink outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary transition-all"
                    >
                      <option value="">
                        {t('assets.form.noCategory', 'Aucune (sans attributs typés)')}
                      </option>
                      {ASSET_CATEGORIES.map((cat) => (
                        <option key={cat} value={cat}>
                          {CATEGORY_LABELS[cat]}
                        </option>
                      ))}
                    </select>
                  </div>

                  {category && defs.length > 0 ? (
                    <div className="rounded-2xl border border-border p-4">
                      <AttributeForm
                        defs={defs}
                        values={attributes}
                        onChange={setAttributes}
                        disabled={isSubmitting}
                      />
                    </div>
                  ) : null}
                </div>

                <div className="flex shrink-0 justify-end gap-3 border-t border-border bg-elevated px-6 py-4">
                  <Button type="button" variant="ghost" onClick={handleClose}>
                    {t('common.cancel', 'Cancel')}
                  </Button>
                  <Button type="submit" variant="primary" loading={isSubmitting}>
                    {t('assets.createAsset')}
                  </Button>
                </div>
              </form>
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
};
