// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useEffect } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { X, Zap, ShieldAlert } from 'lucide-react';
import { toast } from 'sonner';
import { useNavigate } from 'react-router';
import { mitigationService } from '../../services/mitigationService';
import type { Mitigation } from '../../types/mitigation';
import { Button, Field, Input } from '../../shared/ds';
import { useI18n } from '../../hooks/useI18n';
import { useEscapeToClose } from '../../shared/useBackTo';

const createMitigationSchema = z.object({
  title: z.string().min(3, 'Le titre doit comporter au moins 3 caractères'),
  description: z.string().optional(),
  due_date: z.string().optional(),
  priority: z.enum(['critical', 'high', 'medium', 'low']).optional(),
});

type CreateMitigationForm = z.infer<typeof createMitigationSchema>;

interface CreateMitigationModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated?: (m: Mitigation) => void;
  /** When set, the created mitigation is linked to this risk (used from the Risk drawer). */
  riskId?: string;
}

export const CreateMitigationModal = ({
  isOpen,
  onClose,
  onCreated,
  riskId,
}: CreateMitigationModalProps) => {
  // Esc closes this overlay (spec §2).
  useEscapeToClose(isOpen, onClose);
  const { t } = useI18n();

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<CreateMitigationForm>({
    resolver: zodResolver(createMitigationSchema),
    defaultValues: { title: '', description: '', due_date: '', priority: 'medium' },
  });

  useEffect(() => {
    if (isOpen) {
      setTimeout(() => {
        const input = document.querySelector('input[name="title"]') as HTMLInputElement | null;
        input?.focus();
      }, 50);
    }
  }, [isOpen]);

  const navigate = useNavigate();

  const handleClose = () => {
    reset();
    onClose();
  };

  const onSubmit = async (values: CreateMitigationForm) => {
    try {
      // Backend requires risk_id + due_date; default due_date to +30 days when the
      // user leaves it blank so a plan can always be created from the risk drawer.
      const payload = {
        title: values.title,
        description: values.description,
        due_date: values.due_date || new Date(Date.now() + 30 * 864e5).toISOString().slice(0, 10),
        priority: values.priority || 'medium',
        source: 'manual',
        ...(riskId ? { risk_id: riskId } : {}),
      } as any;

      const created = await mitigationService.createMitigation(payload);

      // §6: land the user ON the thing they just made, and give them the way
      // back. Creating a plan and being dropped back on the risk with no sign
      // of it is what made "j'ai créé une mitigation" feel like it failed.
      toast.success("Plan d'atténuation créé", {
        description: created.title,
        action: riskId
          ? {
              label: 'Revenir au risque',
              onClick: () => navigate(`/risks?focus=${riskId}`),
            }
          : undefined,
      });
      onCreated?.(created);
      handleClose();
      if (created.id) navigate(`/risks/mitigations/${created.id}`);
    } catch (err) {
      console.error(err);
      toast.error('Impossible de créer le plan', { description: 'Erreur serveur' });
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
            className="fixed inset-0 z-80 bg-surface-overlay backdrop-blur-sm"
          />

          <motion.div
            initial={{ opacity: 0, scale: 0.96, y: 40 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.96, y: 40 }}
            transition={{ duration: 0.22, type: 'spring', stiffness: 240 }}
            className="fixed inset-0 z-90 flex items-center justify-center p-4"
          >
            {/* Announced as a dialog and named by its heading; the board behind
                it stays in the accessibility tree otherwise. */}
            <div
              role="dialog"
              aria-modal="true"
              aria-labelledby="create-mitigation-title"
              className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-3xl border border-border bg-elevated shadow-card-lg"
            >
              <div className="flex shrink-0 items-center justify-between gap-4 border-b border-border px-6 py-5">
                <div>
                  <h2 id="create-mitigation-title" className="text-2xl font-semibold text-ink">
                    {t('mitigations.createTitle')}
                  </h2>
                  <p className="text-sm text-ink-muted">{t('mitigations.createSubtitle')}</p>
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
                <div className="flex-1 space-y-6 overflow-y-auto px-6 py-6 scrollbar-thin">
                  <Field
                    label={t('mitigations.fieldTitle')}
                    message={errors.title?.message}
                    status={errors.title?.message ? 'invalid' : 'default'}
                  >
                    <Input {...register('title')} disabled={isSubmitting} />
                  </Field>

                  <div className="space-y-1.5">
                    <label
                      htmlFor="create-mitigation-description"
                      className="text-xs font-semibold uppercase tracking-[0.18em] text-ink-muted"
                    >
                      {t('mitigations.fieldDescription')}
                    </label>
                    <textarea
                      id="create-mitigation-description"
                      {...register('description')}
                      rows={4}
                      className="w-full rounded-3xl border border-border bg-elevated px-4 py-3 text-sm text-ink outline-none focus:ring-2 focus:ring-primary/40"
                      disabled={isSubmitting}
                    />
                    {errors.description && (
                      <p className="text-xs text-danger-text">{errors.description.message}</p>
                    )}
                  </div>

                  <div className="grid gap-4 sm:grid-cols-2">
                    <div>
                      <label
                        htmlFor="create-mitigation-due-date"
                        className="text-xs font-semibold uppercase tracking-[0.18em] text-ink-muted"
                      >
                        {t('mitigations.fieldDeadline')}
                      </label>
                      <Input
                        id="create-mitigation-due-date"
                        type="date"
                        {...register('due_date')}
                        disabled={isSubmitting}
                      />
                    </div>
                    <div>
                      <label
                        htmlFor="create-mitigation-priority"
                        className="text-xs font-semibold uppercase tracking-[0.18em] text-ink-muted"
                      >
                        {t('mitigations.fieldPriority')}
                      </label>
                      <select
                        id="create-mitigation-priority"
                        {...register('priority')}
                        className="w-full rounded-3xl border border-border bg-elevated px-4 py-3 text-sm text-ink"
                      >
                        {(['critical', 'high', 'medium', 'low'] as const).map((level) => (
                          <option key={level} value={level}>
                            {t(`mitigations.priority.${level}`)}
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>
                </div>

                <div className="flex shrink-0 justify-end gap-3 border-t border-border bg-elevated px-6 py-4">
                  <Button type="button" variant="ghost" onClick={handleClose}>
                    {t('common.cancel')}
                  </Button>
                  <Button
                    type="submit"
                    variant="secondary"
                    loading={isSubmitting}
                    className="gap-2"
                  >
                    <Zap size={16} aria-hidden="true" />
                    {t('common.create')}
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

export default CreateMitigationModal;
