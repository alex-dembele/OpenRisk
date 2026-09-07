// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { api } from '../lib/api';

export type RiskStatus = 'open' | 'in_progress' | 'mitigated' | 'accepted' | 'closed';
export type RiskLevel = 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW';
// ISO 31000 lifecycle phases (Identifier → Analyser → Évaluer → Traiter → Surveiller → Clôturer).
export type RiskPhase =
  'identified' | 'analyzed' | 'evaluated' | 'treated' | 'monitored' | 'closed';

export interface Risk {
  id: string;
  title: string;
  description: string;
  score: number;
  impact: number;
  probability: number;
  status: RiskStatus;
  lifecycle_phase?: RiskPhase;
  level?: RiskLevel;
  tags?: string[];
  frameworks?: string[];
  assets?: Asset[];
  assigned_to?: string;
  created_by?: string;
  created_at?: string;
  updated_at?: string;
  source?: string;
  mitigations?: Mitigation[];
  residual_risk?: number;
  // Cyber Risk Quantification (CRQ). Inputs in XAF; ALE returned in XAF + USD.
  sle_xaf?: number | null;
  aro?: number | null;
  ale_xaf?: number;
  ale_usd?: number;
  ale_basis?: 'explicit' | 'reference';
  // Full financial-quantification drivers (spec §9), all XAF, all optional.
  downtime_hours?: number | null;
  hourly_downtime_cost_xaf?: number | null;
  data_loss_cost_xaf?: number | null;
  fines_xaf?: number | null;
  other_direct_cost_xaf?: number | null;
  remediation_cost_xaf?: number | null;
  mitigation_effectiveness?: number | null; // [0,1]
  // Review cadence.
  review_interval_days?: number;
  next_review_at?: string | null;
  last_reviewed_at?: string | null;
}

export interface Asset {
  id: string;
  name: string;
  type: string;
  criticality: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
  owner?: string;
}

export interface Mitigation {
  id: string;
  title: string;
  status: 'PLANNED' | 'IN_PROGRESS' | 'DONE';
  progress: number;
  assignee?: string;
}

export interface RiskListResponse {
  items: Risk[];
  total: number;
}

export interface RiskQueryParams {
  q?: string;
  status?: RiskStatus;
  min_score?: number;
  max_score?: number;
  framework?: string;
  assigned_to?: string;
  created_by?: string;
  source?: string;
  tag?: string;
  date_from?: string;
  date_to?: string;
  page?: number;
  limit?: number;
  sort_by?: string;
  sort_dir?: 'asc' | 'desc';
}

export interface CreateRiskInput {
  title: string;
  description: string;
  probability: number;
  impact: number;
  asset_criticality?: number;
  framework?: string;
  tags?: string[];
  asset_ids?: string[];
  source?: string;
  status?: RiskStatus;
}

export interface UpdateRiskInput {
  title?: string;
  description?: string;
  probability?: number;
  impact?: number;
  asset_criticality?: number;
  framework?: string;
  tags?: string[];
  asset_ids?: string[];
  status?: RiskStatus;
  sle_xaf?: number | null;
  aro?: number | null;
  downtime_hours?: number | null;
  hourly_downtime_cost_xaf?: number | null;
  data_loss_cost_xaf?: number | null;
  fines_xaf?: number | null;
  other_direct_cost_xaf?: number | null;
  remediation_cost_xaf?: number | null;
  mitigation_effectiveness?: number | null;
  review_interval_days?: number;
}

/**
 * POST /risks/bulk (#581).
 *
 * This shape is CORRECTED, not extended: it previously declared `action` and a
 * nested `payload`, while the API has always read `type` with the parameters
 * flat, and it omitted `remove_tags` entirely. Nothing called it, so the
 * mismatch never surfaced — the endpoint itself was mounted only inside a
 * RegisterRoutes function that main.go never invoked.
 */
export interface BulkRiskActionInput {
  type: 'change_status' | 'assign_to' | 'add_tags' | 'remove_tags' | 'delete';
  risk_ids: string[];
  /** change_status */
  status?: RiskStatus;
  /** assign_to */
  assign_to_id?: string;
  /** add_tags / remove_tags */
  tags?: string[];
  /** Recorded on each audit entry. */
  justification?: string;
}

/**
 * The outcome of a bulk action.
 *
 * All-or-nothing (decision D-036): the call either applied to every risk or to
 * none, so there is no per-item tally. A failure arrives as an HTTP error, not
 * as a `failed` count — the previous {success, failed, errors} shape could only
 * ever report "all" or "none" once the mutation became transactional, and a
 * structurally-always-zero `failed` invites handling that can never run.
 */
export interface BulkRiskActionResult {
  total: number;
  applied: number;
  risk_ids: string[];
  /**
   * How many audit entries were written. It falls short of `applied` only if the
   * trail itself was unavailable — the mutation still happened. Surfaced rather
   * than hidden so "changed but not fully journalled" is visible.
   */
  audited: number;
}

export const riskService = {
  listRisks: async (params: RiskQueryParams): Promise<RiskListResponse> => {
    const response = await api.get<RiskListResponse>('/risks', { params });
    return response.data;
  },

  getRisk: async (id: string): Promise<Risk> => {
    const response = await api.get<Risk>(`/risks/${id}`);
    return response.data;
  },

  createRisk: async (payload: CreateRiskInput): Promise<Risk> => {
    const response = await api.post<Risk>('/risks', payload);
    return response.data;
  },

  updateRisk: async (id: string, payload: UpdateRiskInput): Promise<Risk> => {
    const response = await api.patch<Risk>(`/risks/${id}`, payload);
    return response.data;
  },

  deleteRisk: async (id: string): Promise<void> => {
    await api.delete(`/risks/${id}`);
  },

  markReviewed: async (id: string): Promise<Risk> => {
    const response = await api.post<Risk>(`/risks/${id}/review`, {});
    return response.data;
  },

  transitionPhase: async (id: string, phase: RiskPhase, note?: string): Promise<Risk> => {
    const response = await api.post<Risk>(`/risks/${id}/transition`, { phase, note });
    return response.data;
  },

  acceptRisk: async (id: string, justification: string): Promise<Risk> => {
    const response = await api.post<Risk>(`/risks/${id}/accept`, { justification });
    return response.data;
  },

  duplicateRisk: async (id: string): Promise<Risk> => {
    const response = await api.post<Risk>(`/risks/${id}/duplicate`);
    return response.data;
  },

  bulkAction: async (payload: BulkRiskActionInput): Promise<BulkRiskActionResult> => {
    const response = await api.post<BulkRiskActionResult>('/risks/bulk', payload);
    return response.data;
  },

  exportRisks: async (params: RiskQueryParams, format: 'csv' | 'json' | 'xlsx' = 'csv') => {
    const response = await api.get<Blob>('/risks/export', {
      params: { ...params, format },
      responseType: 'blob',
    });
    return response.data;
  },

  importRisks: async (formData: FormData) => {
    const response = await api.post('/risks/import', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    return response.data;
  },
};
