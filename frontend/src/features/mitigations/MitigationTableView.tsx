// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useFormat } from '../../hooks/useI18n';
import { useState, useMemo, memo } from 'react';
import { motion } from 'framer-motion';
import { ChevronUp, ChevronDown, ChevronsUpDown } from 'lucide-react';
import type { Mitigation } from '../../types/mitigation';
import { UserAvatar, StatusDot } from '../../components/shared';
import { AutoDetectedBadge } from '../../components/shared/AutoDetectedBadge';
import { cn } from '../../utils/cn';

interface MitigationTableViewProps {
  mitigations: Mitigation[];
  isLoading?: boolean;
  onRowClick?: (mitigation: Mitigation) => void;
}

type SortField = 'title' | 'due_date' | 'progress' | 'priority' | 'status';
type SortDir = 'asc' | 'desc' | null;

export const MitigationTableView = memo(function MitigationTableView({
  mitigations,
  isLoading = false,
  onRowClick,
}: MitigationTableViewProps) {
  const fmt = useFormat();
  const [sortField, setSortField] = useState<SortField>('due_date');
  const [sortDir, setSortDir] = useState<SortDir>('asc');

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir(sortDir === 'asc' ? 'desc' : sortDir === 'desc' ? null : 'asc');
      if (sortDir === null) setSortField(field);
    } else {
      setSortField(field);
      setSortDir('asc');
    }
  };

  const sortedMitigations = useMemo(() => {
    if (!sortDir || !sortField) return mitigations;

    const sorted = [...mitigations].sort((a, b) => {
      let aVal: any = a[sortField as keyof Mitigation];
      let bVal: any = b[sortField as keyof Mitigation];

      if (sortField === 'due_date') {
        aVal = new Date(aVal).getTime();
        bVal = new Date(bVal).getTime();
      }

      if (aVal < bVal) return sortDir === 'asc' ? -1 : 1;
      if (aVal > bVal) return sortDir === 'asc' ? 1 : -1;
      return 0;
    });

    return sorted;
  }, [mitigations, sortField, sortDir]);

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) return <ChevronsUpDown size={14} className="text-fg-muted" />;
    return sortDir === 'asc' ? (
      <ChevronUp size={14} className="text-info-text" />
    ) : (
      <ChevronDown size={14} className="text-info-text" />
    );
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin">⏳</div>
      </div>
    );
  }

  if (mitigations.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-fg-secondary">
        Aucun plan d'atténuation
      </div>
    );
  }

  return (
    <div className="w-full overflow-x-auto">
      <table className="w-full text-sm">
        <thead className="border-b border-border-default bg-surface-1/50">
          <tr>
            <th className="text-left px-4 py-3 font-semibold text-fg-secondary">
              <button
                onClick={() => handleSort('title')}
                className="flex items-center gap-2 hover:text-fg-primary transition-colors"
              >
                Titre
                <SortIcon field="title" />
              </button>
            </th>
            <th className="text-left px-4 py-3 font-semibold text-fg-secondary">
              <button
                onClick={() => handleSort('status')}
                className="flex items-center gap-2 hover:text-fg-primary transition-colors"
              >
                Statut
                <SortIcon field="status" />
              </button>
            </th>
            <th className="text-left px-4 py-3 font-semibold text-fg-secondary">
              <button
                onClick={() => handleSort('priority')}
                className="flex items-center gap-2 hover:text-fg-primary transition-colors"
              >
                Priorité
                <SortIcon field="priority" />
              </button>
            </th>
            <th className="text-left px-4 py-3 font-semibold text-fg-secondary">
              <button
                onClick={() => handleSort('progress')}
                className="flex items-center gap-2 hover:text-fg-primary transition-colors"
              >
                Progression
                <SortIcon field="progress" />
              </button>
            </th>
            <th className="text-left px-4 py-3 font-semibold text-fg-secondary">
              <button
                onClick={() => handleSort('due_date')}
                className="flex items-center gap-2 hover:text-fg-primary transition-colors"
              >
                Deadline
                <SortIcon field="due_date" />
              </button>
            </th>
            <th className="text-left px-4 py-3 font-semibold text-fg-secondary">Assigné</th>
            <th className="text-left px-4 py-3 font-semibold text-fg-secondary">Actions</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border-subtle">
          {sortedMitigations.map((mitigation) => (
            <motion.tr
              key={mitigation.id}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              onClick={() => onRowClick?.(mitigation)}
              className="hover:bg-surface-2/30 transition-colors cursor-pointer"
            >
              <td className="px-4 py-3 text-fg-primary">{mitigation.title}</td>
              <td className="px-4 py-3">
                <StatusDot
                  status={mitigation.status.toLowerCase() as any}
                  size="sm"
                  withLabel={true}
                />
              </td>
              <td className="px-4 py-3">
                <span
                  className={cn(
                    'px-2 py-1 rounded text-xs font-medium',
                    mitigation.priority === 'critical'
                      ? 'bg-danger/20 text-danger-text'
                      : mitigation.priority === 'high'
                        ? 'bg-warning/20 text-warning-text'
                        : mitigation.priority === 'medium'
                          ? 'bg-warning/20 text-warning-text'
                          : 'bg-success/20 text-success-text',
                  )}
                >
                  {mitigation.priority}
                </span>
              </td>
              <td className="px-4 py-3 text-fg-secondary">{mitigation.progress_percentage}%</td>
              <td className="px-4 py-3 text-fg-secondary">
                {fmt.date(mitigation.due_date)}
              </td>
              <td className="px-4 py-3">
                {mitigation.assigned_to_user ? (
                  <UserAvatar
                    name={mitigation.assigned_to_user.name}
                    avatar={mitigation.assigned_to_user.avatar}
                    size="xs"
                    tooltip={true}
                  />
                ) : (
                  <span className="text-xs text-fg-muted">−</span>
                )}
              </td>
              <td className="px-4 py-3">
                {mitigation.auto_detected_count > 0 && <AutoDetectedBadge size="sm" />}
              </td>
            </motion.tr>
          ))}
        </tbody>
      </table>
    </div>
  );
});
