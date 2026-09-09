// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useFormat } from '../hooks/useI18n';
import React, { useState, useEffect } from 'react';
import { CartesianChart, PieChart } from '../shared/ds/charts';
import { Download, RefreshCw, TrendingUp, TrendingDown } from 'lucide-react';

interface RiskMetrics {
  total_risks: number;
  active_risks: number;
  mitigated_risks: number;
  average_score: number;
  high_risks: number;
  medium_risks: number;
  low_risks: number;
  risks_by_framework: Record<string, number>;
  risks_by_status: Record<string, number>;
  created_this_month: number;
  updated_this_month: number;
}

interface MitigationMetrics {
  total_mitigations: number;
  completed_mitigations: number;
  pending_mitigations: number;
  overdue_mitigations: number;
  completion_rate: number;
  avg_completion_days: number;
  risks_with_mitigation: number;
}

interface TrendPoint {
  date: string;
  count: number;
  avg_score: number;
  new_risks: number;
  mitigated: number;
}

interface FrameworkAnalytic {
  framework: string;
  associated_risks: number;
  average_risk_score: number;
  compliance_percentage: number;
}

interface DashboardSnapshot {
  timestamp: string;
  risk_metrics: RiskMetrics;
  mitigation_metrics: MitigationMetrics;
  framework_analytics: FrameworkAnalytic[];
  trends: TrendPoint[];
}

export default function Analytics() {
  const fmt = useFormat();
  const [dashboard, setDashboard] = useState<DashboardSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchDashboard = async () => {
    try {
      setRefreshing(true);
      const response = await fetch('/api/v1/analytics/dashboard');
      if (!response.ok)
        throw new Error("We couldn't load your dashboard. Please refresh the page.");
      const data = await response.json();
      setDashboard(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    fetchDashboard();
    // Refresh every 5 minutes
    const interval = setInterval(fetchDashboard, 5 * 60 * 1000);
    return () => clearInterval(interval);
  }, []);

  const handleExport = async (format: 'json' | 'csv') => {
    try {
      const response = await fetch(`/api/v1/analytics/export?format=${format}`);
      if (!response.ok) throw new Error("We couldn't export the data. Please try again.");

      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `analytics-${format}.${format === 'json' ? 'json' : 'csv'}`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "We couldn't export the data. Please try again.",
      );
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="inline-block animate-spin">
            <div className="h-8 w-8 border-4 border-accent border-t-blue-600 rounded-full"></div>
          </div>
          <p className="mt-4 text-fg-secondary">Loading analytics...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-danger-surface border border-danger rounded-lg p-4">
          <p className="text-danger-text">Error: {error}</p>
        </div>
      </div>
    );
  }

  if (!dashboard) {
    return (
      <div className="p-6">
        <div className="text-fg-secondary">No data available</div>
      </div>
    );
  }

  const riskMetrics = dashboard.risk_metrics;
  const mitMetrics = dashboard.mitigation_metrics;

  // Prepare chart data
  /* These are SEVERITIES, not categories, so they carry a band and take the
     risk scale — `chart.ts`'s second rule. The hex literals they used to carry
     were a third copy of that scale, free to drift from the badges beside them. */
  const riskLevelData = [
    { key: 'high', label: 'High', value: riskMetrics.high_risks, band: 'high' },
    { key: 'medium', label: 'Medium', value: riskMetrics.medium_risks, band: 'medium' },
    { key: 'low', label: 'Low', value: riskMetrics.low_risks, band: 'low' },
  ] as const;

  const statusData = Object.entries(riskMetrics.risks_by_status).map(([status, count]) => ({
    name: status.charAt(0).toUpperCase() + status.slice(1),
    value: count,
  }));

  const frameworkData = Object.entries(riskMetrics.risks_by_framework).map(
    ([framework, count]) => ({
      name: framework,
      risks: count,
    }),
  );

  return (
    <div className="space-y-6 pb-6">
      {/* Header */}
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold text-fg-primary">Analytics Dashboard</h1>
          <p className="text-fg-secondary mt-2">
            Last updated: {new Date(dashboard.timestamp).toLocaleString()}
          </p>
        </div>
        <div className="flex gap-3">
          <button
            onClick={fetchDashboard}
            disabled={refreshing}
            className="flex items-center gap-2 px-4 py-2 bg-accent-solid hover:brightness-110 text-fg-on-solid rounded-lg transition disabled:opacity-50"
          >
            <RefreshCw size={18} className={refreshing ? 'animate-spin' : ''} />
            Refresh
          </button>
          <div className="flex gap-2">
            <button
              onClick={() => handleExport('json')}
              className="flex items-center gap-2 px-4 py-2 bg-success hover:bg-success text-fg-primary rounded-lg transition"
            >
              <Download size={18} />
              JSON
            </button>
            <button
              onClick={() => handleExport('csv')}
              className="flex items-center gap-2 px-4 py-2 bg-success hover:bg-success text-fg-primary rounded-lg transition"
            >
              <Download size={18} />
              CSV
            </button>
          </div>
        </div>
      </div>

      {/* Key Metrics */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <MetricCard
          title="Total Risks"
          value={riskMetrics.total_risks}
          change={riskMetrics.created_this_month}
          changeLabel="this month"
          icon={TrendingUp}
        />
        <MetricCard
          title="Active Risks"
          value={riskMetrics.active_risks}
          percentage={
            riskMetrics.total_risks > 0
              ? ((riskMetrics.active_risks / riskMetrics.total_risks) * 100).toFixed(1)
              : '0'
          }
          icon={TrendingUp}
        />
        <MetricCard
          title="Avg Risk Score"
          value={riskMetrics.average_score.toFixed(2)}
          maxValue="10"
          icon={TrendingDown}
        />
        <MetricCard
          title="Mitigation Rate"
          value={mitMetrics.completion_rate.toFixed(1)}
          suffix="%"
          icon={TrendingUp}
        />
      </div>

      {/* Risk Distribution */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Risk Levels Pie Chart */}
        <div className="bg-surface-1 rounded-lg p-6">
          <h2 className="text-xl font-bold text-fg-primary mb-4">Risk Distribution by Level</h2>
          <PieChart slices={riskLevelData} ariaLabel="Risk distribution by level" />
        </div>

        {/* Risk Status Distribution */}
        <div className="bg-surface-1 rounded-lg p-6">
          <h2 className="text-xl font-bold text-fg-primary mb-4">Risk Status Distribution</h2>
          <CartesianChart
            data={statusData}
            x="name"
            series={[{ type: 'bar', key: 'value', label: 'Risks' }]}
            ariaLabel="Risk count by status"
          />
        </div>
      </div>

      {/* Trends */}
      <div className="bg-surface-1 rounded-lg p-6">
        <h2 className="text-xl font-bold text-fg-primary mb-4">Risk Trends (30 days)</h2>
        <CartesianChart
          data={dashboard.trends}
          x="date"
          series={[
            { type: 'line', key: 'count', label: 'Total Risks' },
            { type: 'line', key: 'avg_score', label: 'Avg Score' },
            { type: 'line', key: 'new_risks', label: 'New Risks' },
          ]}
          formatCategory={(d) =>
            fmt.date(d, { month: 'short', day: 'numeric' })
          }
          ariaLabel="Total risks, average score and new risks over the last 30 days"
        />
      </div>

      {/* Framework Analysis */}
      <div className="bg-surface-1 rounded-lg p-6">
        <h2 className="text-xl font-bold text-fg-primary mb-4">Risks by Framework</h2>
        <CartesianChart
          data={frameworkData}
          x="name"
          series={[{ type: 'bar', key: 'risks', label: 'Risks' }]}
          ariaLabel="Risk count by compliance framework"
        />
      </div>

      {/* Mitigation Metrics */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <MetricCard
          title="Total Mitigations"
          value={mitMetrics.total_mitigations}
          icon={TrendingUp}
        />
        <MetricCard
          title="Completed"
          value={mitMetrics.completed_mitigations}
          percentage={
            mitMetrics.total_mitigations > 0
              ? ((mitMetrics.completed_mitigations / mitMetrics.total_mitigations) * 100).toFixed(1)
              : '0'
          }
          icon={TrendingUp}
        />
        <MetricCard
          title="Overdue"
          value={mitMetrics.overdue_mitigations}
          alert={mitMetrics.overdue_mitigations > 0}
          icon={TrendingDown}
        />
      </div>
    </div>
  );
}

interface MetricCardProps {
  title: string;
  value: string | number;
  change?: number;
  changeLabel?: string;
  percentage?: string;
  suffix?: string;
  maxValue?: string;
  alert?: boolean;
  icon?: React.ComponentType<{ size: number }>;
}

function MetricCard({
  title,
  value,
  change,
  changeLabel,
  percentage,
  suffix = '',
  maxValue,
  alert = false,
  icon: Icon,
}: MetricCardProps) {
  return (
    <div
      className={`bg-surface-1 rounded-lg p-6 border ${alert ? 'border-danger' : 'border-border-default'}`}
    >
      <div className="flex justify-between items-start">
        <div>
          <p className="text-fg-secondary text-sm font-medium">{title}</p>
          <p
            className={`text-3xl font-bold mt-2 ${alert ? 'text-danger-text' : 'text-fg-primary'}`}
          >
            {value}
            {suffix}
            {maxValue && <span className="text-sm text-fg-secondary">/{maxValue}</span>}
          </p>
          {change !== undefined && (
            <p className="text-sm text-success-text mt-2">
              +{change} {changeLabel}
            </p>
          )}
          {percentage && <p className="text-sm text-info-text mt-2">{percentage}%</p>}
        </div>
        {Icon && (
          <div className="text-fg-muted">
            <Icon size={24} />
          </div>
        )}
      </div>
    </div>
  );
}
