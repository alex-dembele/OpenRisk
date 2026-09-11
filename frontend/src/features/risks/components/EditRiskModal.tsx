// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useEffect } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { X, ShieldAlert } from 'lucide-react';
import { toast } from 'sonner';
import { apiErrorMessage } from '../../../lib/apiError';

import { useRiskStore } from '../../../hooks/useRiskStore';
import { useAssetStore } from '../../../hooks/useAssetStore';
import { Button, Field, Input } from '../../../shared/ds';
import { useI18n } from '../../../hooks/useI18n';

const riskSchema = z.object({
  title: z.string().min(5).max(100),
  description: z.string().min(10),
  // THE SCORE ENGINE SCALES, not 1-5. Score = Probability (0.0-1.0) x Impact
  // (0.0-10.0) x AssetCriticality. This form used to declare 1-5 for both, which
  // broke saving in two independent ways: loading any real risk (probability
  // 0.7) failed `min(1)` so zodResolver refused to submit and the Save button
  // did nothing at all, and any value the 1-5 picker could produce failed the
  // server's `max=1` on probability. CreateRiskModal always used the right
  // scale — the two forms simply disagreed.
  impact: z.number().min(0).max(10),
  probability: z.number().min(0).max(1),
  // Bound to a comma-separated text input; split into an array on submit. The
  // schema previously declared z.array here while the input produced a string,
  // so zodResolver rejected EVERY save (the Edit form's Save was broken).
  tags: z.string(),
  asset_ids: z.array(z.string()).optional(),
  frameworks: z.array(z.string()).optional(),
});

type RiskFormData = z.infer<typeof riskSchema>;

interface EditRiskModalProps {
  isOpen: boolean;
  onClose: () => void;
  risk: any | null;
  onSuccess?: () => void;
}

export const EditRiskModal = ({ isOpen, onClose, risk, onSuccess }: EditRiskModalProps) => {
  const { updateRisk, isLoading } = useRiskStore();
  const { t } = useI18n();
  const { assets, fetchAssets } = useAssetStore();

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    formState: { errors, isSubmitting },
    reset,
  } = useForm<RiskFormData>({
    resolver: zodResolver(riskSchema),
    defaultValues: {
      impact: 5,
      probability: 0.5,
      asset_ids: [],
      tags: '',
      frameworks: [],
    },
  });

  useEffect(() => {
    if (isOpen) fetchAssets();
    if (risk) {
      setValue('title', risk.title || '');
      setValue('description', risk.description || '');
      setValue('impact', typeof risk.impact === 'number' ? risk.impact : 5);
      setValue('probability', typeof risk.probability === 'number' ? risk.probability : 0.5);
      setValue('tags', (risk.tags || []).join(','));
      setValue(
        'asset_ids',
        (risk.assets || []).map((a: any) => a.id),
      );
      setValue('frameworks', risk.frameworks || []);
    } else {
      reset();
    }
  }, [isOpen, risk, setValue, reset, fetchAssets]);

  // Accessibility: focus title input when modal opens
  useEffect(() => {
    if (!isOpen) return;
    const t = window.setTimeout(() => {
      const el = document.querySelector('input[name="title"]') as HTMLInputElement | null;
      el?.focus();
    }, 40);
    return () => window.clearTimeout(t);
  }, [isOpen]);

  // Close on Escape
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [isOpen, onClose]);

  const selectedAssetIds = watch('asset_ids') || [];
  const selectedFrameworks = watch('frameworks') || [];
  const toggleAsset = (assetId: string) => {
    const current = selectedAssetIds;
    if (current.includes(assetId))
      setValue(
        'asset_ids',
        current.filter((id: string) => id !== assetId),
        { shouldValidate: true },
      );
    else setValue('asset_ids', [...current, assetId], { shouldValidate: true });
  };

  const frameworksList = ['ISO27001', 'CIS', 'NIST', 'OWASP'];
  const toggleFramework = (f: string) => {
    const current = selectedFrameworks;
    if (current.includes(f))
      setValue(
        'frameworks',
        current.filter((v: string) => v !== f),
        { shouldValidate: true },
      );
    else setValue('frameworks', [...current, f], { shouldValidate: true });
  };

  const onSubmit = async (data: RiskFormData) => {
    if (!risk) return;
    try {
      // Split the comma-separated tags text into the array the API expects.
      const payload = {
        ...data,
        tags: data.tags
          .split(',')
          .map((t) => t.trim())
          .filter(Boolean),
      };
      await updateRisk(risk.id, payload);
      toast.success(t('risks.updated'));
      onClose();
      onSuccess?.();
    } catch (err) {
      // Show what the server actually said. A generic message hides the one
      // sentence that names the offending field.
      toast.error(apiErrorMessage(err) || t('risks.updateFailed'));
    }
  };

  const handleClose = () => {
    reset();
    onClose();
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
            className="fixed inset-0 bg-surface-overlay z-80"
          />

          <motion.div
            initial={{ opacity: 0, y: 50 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 50 }}
            transition={{ type: 'spring', stiffness: 300, damping: 30 }}
            role="dialog"
            aria-modal="true"
            aria-labelledby="edit-risk-title"
            className="fixed inset-0 m-auto w-full max-w-lg h-fit max-h-[90vh] bg-surface border border-border rounded-xl shadow-2xl p-6 z-90 overflow-hidden"
          >
            <div className="flex justify-between items-center mb-6 border-b border-border-strong/5 pb-4">
              <h2
                id="edit-risk-title"
                className="text-xl font-bold text-fg-primary flex items-center gap-2"
              >
                <ShieldAlert className="text-primary" size={20} aria-hidden="true" />{' '}
                {t('risks.editRisk')}
              </h2>
              <button
                onClick={handleClose}
                aria-label={t('common.close')}
                className="text-fg-muted hover:text-fg-primary transition-colors"
              >
                <X size={24} aria-hidden="true" />
              </button>
            </div>

            <form
              onSubmit={handleSubmit((data: any) => onSubmit(data))}
              className="space-y-4 overflow-y-auto pr-2 max-h-[calc(90vh-140px)]"
            >
              <Field
                label={t('risks.fieldTitle')}
                message={errors.title?.message}
                status={errors.title?.message ? 'invalid' : 'default'}
              >
                <Input {...register('title')} disabled={isLoading} />
              </Field>

              <div className="space-y-1.5">
                <label
                  htmlFor="edit-risk-description"
                  className="text-xs font-medium text-fg-secondary uppercase tracking-wider"
                >
                  {t('risks.fieldDescription')}
                </label>
                <textarea
                  id="edit-risk-description"
                  {...register('description')}
                  rows={4}
                  disabled={isLoading}
                  className={`w-full bg-surface-1 border ${errors.description ? 'border-danger' : 'border-border'} rounded-lg p-3 text-sm text-fg-primary focus:ring-2 focus:ring-primary/50 outline-none resize-none transition-colors ${isLoading ? 'opacity-70' : ''}`}
                />
                {errors.description && (
                  <p className="text-xs text-danger-text mt-1">{errors.description?.message}</p>
                )}
              </div>

              <div className="grid grid-cols-2 gap-4 pt-2">
                <div className="space-y-1.5">
                  <label
                    htmlFor="edit-risk-impact"
                    className="text-xs font-medium text-fg-secondary uppercase tracking-wider"
                  >
                    {t('risks.impact')} (0 – 10)
                  </label>
                  {/* The visible value sits in a separate row below, which is read
                      as loose text rather than as the slider's value. */}
                  <input
                    id="edit-risk-impact"
                    type="range"
                    min={0}
                    max={10}
                    step={0.5}
                    disabled={isLoading}
                    aria-valuetext={(watch('impact') ?? 0).toFixed(1)}
                    {...register('impact', { valueAsNumber: true })}
                    className="w-full"
                  />
                  <div className="flex items-center justify-between text-xs text-fg-secondary">
                    <span>0</span>
                    <span className="font-semibold text-fg-primary">
                      {(watch('impact') ?? 0).toFixed(1)}
                    </span>
                    <span>10</span>
                  </div>
                  {errors.impact && (
                    <p className="text-xs text-danger-text mt-1">{errors.impact?.message}</p>
                  )}
                </div>

                <div className="space-y-1.5">
                  <label
                    htmlFor="edit-risk-probability"
                    className="text-xs font-medium text-fg-secondary uppercase tracking-wider"
                  >
                    {t('risks.probability')} (0 – 1)
                  </label>
                  <input
                    id="edit-risk-probability"
                    type="range"
                    min={0}
                    max={1}
                    step={0.05}
                    disabled={isLoading}
                    aria-valuetext={(watch('probability') ?? 0).toFixed(2)}
                    {...register('probability', { valueAsNumber: true })}
                    className="w-full"
                  />
                  <div className="flex items-center justify-between text-xs text-fg-secondary">
                    <span>0</span>
                    <span className="font-semibold text-fg-primary">
                      {(watch('probability') ?? 0).toFixed(2)}
                    </span>
                    <span>1</span>
                  </div>
                  {errors.probability && (
                    <p className="text-xs text-danger-text mt-1">{errors.probability?.message}</p>
                  )}
                </div>
              </div>

              {/* Assets selector */}
              <div className="space-y-2 pt-2">
                {/* A <label> with no control names nothing. These are toggle
                    buttons, so the set is a group named by its own text. */}
                <div
                  id="edit-risk-assets-label"
                  className="text-xs font-medium text-fg-secondary uppercase tracking-wider flex justify-between"
                >
                  {t('risks.affectedAssets')}
                  <span className="text-[10px] bg-surface-2 px-2 py-0.5 rounded-full">
                    {t('risks.selectedCount', { count: selectedAssetIds.length })}
                  </span>
                </div>
                <div className="flex flex-wrap gap-2 max-h-32 overflow-y-auto p-2 border border-border rounded-lg bg-surface-1/30">
                  {assets.length === 0 ? (
                    <div className="text-fg-muted text-xs w-full text-center py-2">
                      {t('risks.noAssets')}
                    </div>
                  ) : (
                    assets.map((a) => (
                      <button
                        key={a.id}
                        type="button"
                        onClick={() => toggleAsset(a.id)}
                        disabled={isLoading}
                        aria-pressed={selectedAssetIds.includes(a.id)}
                        className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-xs font-medium border transition-all ${selectedAssetIds.includes(a.id) ? 'bg-accent-soft border-accent text-info-text' : 'bg-surface-2 border-border-default text-fg-secondary'} ${isLoading ? 'opacity-70' : ''}`}
                      >
                        {a.name}
                      </button>
                    ))
                  )}
                </div>
              </div>

              <Field label={t('risks.tags')}>
                <Input
                  {...register('tags')}
                  placeholder="ex: critical, web-app, legacy"
                  disabled={isLoading}
                />
              </Field>

              {/* Frameworks selector */}
              <div className="space-y-2 pt-2">
                <div
                  id="edit-risk-frameworks-label"
                  className="text-xs font-medium text-fg-secondary uppercase tracking-wider flex justify-between"
                >
                  {t('risks.frameworks')}
                  <span className="text-[10px] bg-surface-2 px-2 py-0.5 rounded-full">
                    {t('risks.selectedCount', { count: selectedFrameworks.length })}
                  </span>
                </div>
                <div className="flex flex-wrap gap-2 p-2">
                  {frameworksList.map((f) => (
                    <button
                      key={f}
                      type="button"
                      onClick={() => toggleFramework(f)}
                      disabled={isLoading}
                      aria-pressed={selectedFrameworks.includes(f)}
                      className={`px-3 py-1.5 rounded-md text-xs font-medium border transition-all ${selectedFrameworks.includes(f) ? 'bg-success/20 border-success text-success-text' : 'bg-surface-2 border-border-default text-fg-secondary hover:border-border-strong'}`}
                    >
                      {f}
                    </button>
                  ))}
                </div>
              </div>

              <div className="flex justify-end gap-3 mt-6 pt-4 border-t border-border-strong/5 sticky bottom-0 bg-surface">
                <Button type="button" variant="ghost" onClick={handleClose} disabled={isLoading}>
                  {t('common.cancel')}
                </Button>
                <Button variant="primary" type="submit" loading={isLoading || isSubmitting}>
                  {t('common.save')}
                </Button>
              </div>
            </form>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
};
