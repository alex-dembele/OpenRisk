// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useFormat } from '../../hooks/useI18n';
import { useMemo, memo, useRef, useEffect } from 'react';
import { motion } from 'framer-motion';
import type { Mitigation } from '../../types/mitigation';
import { cn } from '../../utils/cn';

interface MitigationGanttViewProps {
  mitigations: Mitigation[];
  isLoading?: boolean;
  onRowClick?: (mitigation: Mitigation) => void;
}

export const MitigationGanttView = memo(function MitigationGanttView({
  mitigations,
  isLoading = false,
  onRowClick,
}: MitigationGanttViewProps) {
  const fmt = useFormat();
  const containerRef = useRef<HTMLDivElement>(null);
  const today = new Date();
  today.setHours(0, 0, 0, 0);

  const dateRange = useMemo(() => {
    if (mitigations.length === 0) {
      const end = new Date(today);
      end.setDate(end.getDate() + 30);
      return { start: new Date(today), end };
    }

    const dates = mitigations.map((m) => new Date(m.due_date));
    const start = new Date(Math.min(today.getTime(), Math.min(...dates.map((d) => d.getTime()))));
    const end = new Date(Math.max(today.getTime(), Math.max(...dates.map((d) => d.getTime()))));

    start.setDate(start.getDate() - 5);
    end.setDate(end.getDate() + 5);

    return { start, end };
  }, [mitigations]);

  const dayCount = Math.ceil(
    (dateRange.end.getTime() - dateRange.start.getTime()) / (1000 * 60 * 60 * 24),
  );

  const getPosition = (date: string) => {
    const d = new Date(date);
    d.setHours(0, 0, 0, 0);
    const diff = Math.ceil((d.getTime() - dateRange.start.getTime()) / (1000 * 60 * 60 * 24));
    return (diff / dayCount) * 100;
  };

  const getTodayPosition = () => {
    const diff = Math.ceil((today.getTime() - dateRange.start.getTime()) / (1000 * 60 * 60 * 24));
    return (diff / dayCount) * 100;
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
    <div ref={containerRef} className="w-full overflow-x-auto">
      <div className="min-w-max p-4">
        {/* Header with dates */}
        <div className="flex mb-8">
          <div className="w-48 shrink-0" />
          <div className="flex-1 relative h-8 bg-surface-2/50 rounded-lg overflow-hidden border border-border-default">
            {/* Today indicator */}
            <motion.div
              className="absolute top-0 bottom-0 w-1 bg-danger z-10"
              style={{ left: `${getTodayPosition()}%` }}
            />

            {/* Month markers */}
            <div className="absolute inset-0 flex">
              {Array.from({ length: dayCount }).map((_, i) => {
                const date = new Date(dateRange.start);
                date.setDate(date.getDate() + i);
                return (
                  <div
                    key={i}
                    className={cn(
                      'flex-1 border-r border-border-default/50 text-xs text-fg-muted px-1 py-1',
                      i % 7 === 0 ? 'border-r border-border-default font-semibold' : '',
                    )}
                  >
                    {i % 7 === 0 &&
                      fmt.date(date, { day: 'numeric', month: 'short' })}
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* Gantt bars */}
        <div className="space-y-3">
          {mitigations.map((mitigation) => {
            const startPos = getPosition(mitigation.created_at || mitigation.due_date);
            const endPos = getPosition(mitigation.due_date);
            const width = Math.max(endPos - startPos, 1);
            const isOverdue = new Date(mitigation.due_date) < today;

            return (
              <div
                key={mitigation.id}
                className="flex items-center gap-3"
                onClick={() => onRowClick?.(mitigation)}
              >
                <div className="w-48 shrink-0 truncate text-sm text-fg-primary hover:text-info-text transition-colors cursor-pointer">
                  {mitigation.title}
                </div>
                <div className="flex-1 relative h-8 bg-surface-2/30 rounded border border-border-default overflow-hidden">
                  <motion.div
                    className={cn(
                      'absolute top-0 bottom-0 rounded transition-colors',
                      isOverdue
                        ? 'bg-danger/40 hover:bg-danger/50'
                        : 'bg-accent-line hover:bg-accent-line',
                    )}
                    style={{
                      left: `${startPos}%`,
                      width: `${width}%`,
                    }}
                    whileHover={{ scale: 1.1 }}
                  >
                    <div className="h-full px-2 py-1 flex items-center text-xs text-fg-primary font-medium truncate">
                      {mitigation.progress_percentage}%
                    </div>
                  </motion.div>
                </div>
              </div>
            );
          })}
        </div>

        {/* Legend */}
        <div className="mt-6 flex items-center gap-4 text-xs text-fg-secondary">
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 bg-danger" />
            <span>En retard</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 bg-accent" />
            <span>En cours</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-1 h-4 bg-danger" />
            <span>Aujourd'hui</span>
          </div>
        </div>
      </div>
    </div>
  );
});
