// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useFormat } from '../../hooks/useI18n';
import { motion } from 'framer-motion';
import { Zap } from 'lucide-react';
import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

interface AutoDetectedBadgeProps {
  detectedAt?: string;
  scanId?: string;
  size?: 'sm' | 'md';
  className?: string;
}

export const AutoDetectedBadge = ({
  detectedAt,
  scanId,
  size = 'md',
  className,
}: AutoDetectedBadgeProps) => {
  const fmt = useFormat();
  const sizeClasses = {
    sm: 'px-2 py-1 text-xs gap-1',
    md: 'px-3 py-1.5 text-sm gap-1.5',
  };

  const iconSizes = {
    sm: 12,
    md: 14,
  };

  const formatTime = (isoString?: string) => {
    if (!isoString) return '';
    const date = new Date(isoString);
    return fmt.dateTime(date, {
      day: '2-digit',
      month: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const tooltipText = detectedAt
    ? `Détecté automatiquement par le scanner le ${formatTime(detectedAt)}${scanId ? ` (scan #${scanId})` : ''}`
    : 'Auto-détecté par le scanner';

  return (
    <motion.div
      initial={{ scale: 0.9, opacity: 0 }}
      animate={{ scale: 1, opacity: 1 }}
      transition={{ duration: 0.2 }}
      className={cn(
        'group relative inline-flex items-center rounded-full border font-medium',
        'bg-success/20 border-success/50 text-success-text',
        sizeClasses[size],
        className,
      )}
    >
      <Zap size={iconSizes[size]} className="shrink-0" />
      <span>Auto</span>

      {/* Tooltip */}
      <div className="absolute bottom-full left-1/2 transform -translate-x-1/2 mb-2 px-3 py-2 bg-surface-1 border border-border-default rounded-lg text-xs text-fg-secondary whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-50">
        {tooltipText}
        <div className="absolute top-full left-1/2 transform -translate-x-1/2 w-2 h-2 bg-surface-1 border-r border-b border-border-default rotate-45 -translate-y-1" />
      </div>
    </motion.div>
  );
};
