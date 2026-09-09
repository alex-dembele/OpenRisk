import type { LocaleCode } from '../../i18n/locales';
// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: LicenseRef-OpenRisk-Commercial
//
// Static presentation metadata for the scan engine: provider identities,
// per-provider credential fields, status colors and small formatting helpers.

import {
  Cloud,
  Server,
  Network,
  Boxes,
  Container,
  Building2,
  GitBranch,
  Users,
  type LucideIcon,
} from 'lucide-react';
import type {
  AgentStatus,
  AssetCriticality,
  ScanJobStatus,
  ScannerProvider,
} from './scannerService';

export interface ProviderMeta {
  short: string;
  color: string;
  icon: LucideIcon;
  // cloud=true means an in-SaaS API collector (credential-based); false means an
  // on-prem agent target (CIDR/host list).
  cloud: boolean;
  // category groups the provider cards in the UI.
  category: 'cloud' | 'container' | 'identity' | 'forge' | 'onprem';
}

export const PROVIDERS: Record<ScannerProvider, ProviderMeta> = {
  aws: { short: 'AWS', color: '#ff9f0a', icon: Cloud, cloud: true, category: 'cloud' },
  azure: { short: 'Azure', color: '#0a84ff', icon: Cloud, cloud: true, category: 'cloud' },
  gcp: { short: 'GCP', color: '#30d158', icon: Cloud, cloud: true, category: 'cloud' },
  kubernetes: {
    short: 'Kubernetes',
    color: '#326ce5',
    icon: Boxes,
    cloud: true,
    category: 'container',
  },
  docker: {
    short: 'Docker',
    color: '#2496ed',
    icon: Container,
    cloud: true,
    category: 'container',
  },
  vmware: { short: 'VMware', color: '#8a9aa6', icon: Server, cloud: true, category: 'container' },
  active_directory: {
    short: 'Active Directory',
    color: '#00a4ef',
    icon: Building2,
    cloud: true,
    category: 'identity',
  },
  m365: {
    short: 'Microsoft 365',
    color: '#eb3c00',
    icon: Users,
    cloud: true,
    category: 'identity',
  },
  github: { short: 'GitHub', color: '#8957e5', icon: GitBranch, cloud: true, category: 'forge' },
  gitlab: { short: 'GitLab', color: '#fc6d26', icon: GitBranch, cloud: true, category: 'forge' },
  nmap: { short: 'On-Premise', color: '#7c6cff', icon: Network, cloud: false, category: 'onprem' },
  agent: { short: 'Agent', color: '#64d2ff', icon: Server, cloud: false, category: 'onprem' },
};

export interface CredField {
  key: string;
  label: string;
  placeholder: string;
  kind: 'text' | 'password' | 'textarea';
}

// Credential fields per in-SaaS API provider — mirrors the `required` slices in
// internal/scanner/cloud.go (endpoint + secrets travel in the encrypted creds).
export const CLOUD_CRED_FIELDS: Partial<Record<ScannerProvider, CredField[]>> = {
  aws: [
    { key: 'access_key_id', label: 'Access Key ID', placeholder: 'AKIA…', kind: 'text' },
    {
      key: 'secret_access_key',
      label: 'Secret Access Key',
      placeholder: '••••••••',
      kind: 'password',
    },
    { key: 'session_token', label: 'Session Token (optional)', placeholder: '', kind: 'password' },
  ],
  azure: [
    { key: 'tenant_id', label: 'Tenant ID', placeholder: '00000000-0000-…', kind: 'text' },
    { key: 'client_id', label: 'Client ID', placeholder: '00000000-0000-…', kind: 'text' },
    { key: 'client_secret', label: 'Client Secret', placeholder: '••••••••', kind: 'password' },
    {
      key: 'subscription_id',
      label: 'Subscription ID',
      placeholder: '00000000-0000-…',
      kind: 'text',
    },
  ],
  gcp: [
    // The placeholder deliberately does NOT reproduce the key file's opening
    // type/account-kind pair. Trivy's gcp-service-account rule matches that
    // literal, so this hint text — which has never held a credential — was
    // reported as a CRITICAL leaked secret on every scan (#507). A permanent
    // false CRITICAL is how a real one gets skimmed past, and suppressing the
    // rule repo-wide would hide an actually leaked key. Keep the shape of the
    // value, never the marker — including in this comment.
    {
      key: 'service_account_json',
      label: 'Service Account JSON',
      placeholder: '{ … contents of the downloaded key file … }',
      kind: 'textarea',
    },
    {
      key: 'project_id',
      label: 'Project ID (optional)',
      placeholder: 'my-project-123',
      kind: 'text',
    },
  ],
  kubernetes: [
    {
      key: 'api_server',
      label: 'API Server URL',
      placeholder: 'https://10.0.0.1:6443',
      kind: 'text',
    },
    { key: 'token', label: 'ServiceAccount Token', placeholder: '••••••••', kind: 'password' },
    {
      key: 'ca_cert',
      label: 'CA Certificate (PEM, optional)',
      placeholder: '-----BEGIN CERTIFICATE-----',
      kind: 'textarea',
    },
  ],
  docker: [
    { key: 'host', label: 'Docker Host', placeholder: 'tcp://10.0.0.10:2376', kind: 'text' },
    {
      key: 'ca_cert',
      label: 'CA Certificate (PEM, optional)',
      placeholder: '-----BEGIN CERTIFICATE-----',
      kind: 'textarea',
    },
    {
      key: 'client_cert',
      label: 'Client Certificate (PEM, optional)',
      placeholder: '-----BEGIN CERTIFICATE-----',
      kind: 'textarea',
    },
    {
      key: 'client_key',
      label: 'Client Key (PEM, optional)',
      placeholder: '-----BEGIN PRIVATE KEY-----',
      kind: 'textarea',
    },
  ],
  vmware: [
    { key: 'url', label: 'vCenter URL', placeholder: 'https://vcenter.corp/sdk', kind: 'text' },
    {
      key: 'username',
      label: 'Username',
      placeholder: 'administrator@vsphere.local',
      kind: 'text',
    },
    { key: 'password', label: 'Password', placeholder: '••••••••', kind: 'password' },
    {
      key: 'insecure',
      label: 'Skip TLS verify ("true"/"false")',
      placeholder: 'false',
      kind: 'text',
    },
  ],
  active_directory: [
    { key: 'url', label: 'LDAP URL', placeholder: 'ldaps://dc1.corp.local:636', kind: 'text' },
    {
      key: 'bind_dn',
      label: 'Bind DN',
      placeholder: 'CN=svc-scan,OU=Service,DC=corp,DC=local',
      kind: 'text',
    },
    { key: 'password', label: 'Bind Password', placeholder: '••••••••', kind: 'password' },
    { key: 'base_dn', label: 'Base DN', placeholder: 'DC=corp,DC=local', kind: 'text' },
  ],
  m365: [
    {
      key: 'tenant_id',
      label: 'Directory (Tenant) ID',
      placeholder: '00000000-0000-…',
      kind: 'text',
    },
    {
      key: 'client_id',
      label: 'Application (Client) ID',
      placeholder: '00000000-0000-…',
      kind: 'text',
    },
    { key: 'client_secret', label: 'Client Secret', placeholder: '••••••••', kind: 'password' },
  ],
  github: [
    { key: 'token', label: 'Access Token (PAT)', placeholder: 'ghp_…', kind: 'password' },
    { key: 'org', label: 'Organisation (optional)', placeholder: 'my-org', kind: 'text' },
    {
      key: 'base_url',
      label: 'Enterprise API URL (optional)',
      placeholder: 'https://ghe.corp/api/v3/',
      kind: 'text',
    },
  ],
  gitlab: [
    { key: 'token', label: 'Access Token', placeholder: 'glpat-…', kind: 'password' },
    {
      key: 'base_url',
      label: 'Instance URL (optional)',
      placeholder: 'https://gitlab.corp',
      kind: 'text',
    },
  ],
};

// SCOPE_HINTS relabels the optional comma-separated "regions/scope" field per
// provider. Providers absent from this map hide the field entirely.
export const SCOPE_HINTS: Partial<
  Record<ScannerProvider, { fr: string; en: string; placeholder: string }>
> = {
  aws: { fr: 'Régions (optionnel)', en: 'Regions (optional)', placeholder: 'eu-west-1, us-east-1' },
  azure: { fr: 'Régions (optionnel)', en: 'Regions (optional)', placeholder: 'westeurope, eastus' },
  gcp: {
    fr: 'Régions (optionnel)',
    en: 'Regions (optional)',
    placeholder: 'europe-west1, us-central1',
  },
  kubernetes: {
    fr: 'Namespaces (optionnel)',
    en: 'Namespaces (optional)',
    placeholder: 'default, production',
  },
};

export function jobStatusColor(s: ScanJobStatus): string {
  switch (s) {
    case 'completed':
      return 'var(--low)';
    case 'running':
    case 'claimed':
      return 'var(--info)';
    case 'queued':
      return 'var(--medium)';
    case 'failed':
    case 'timeout':
      return 'var(--critical)';
    default:
      return 'var(--fg-muted)';
  }
}

export function agentStatusColor(s: AgentStatus): string {
  switch (s) {
    case 'online':
      return 'var(--low)';
    case 'scanning':
      return 'var(--info)';
    case 'error':
      return 'var(--critical)';
    case 'revoked':
      return 'var(--fg-muted)';
    default:
      return 'var(--fg-muted)'; // offline
  }
}

export function severityColor(sev: string): string {
  switch (sev.toLowerCase()) {
    case 'critical':
      return 'var(--critical)';
    case 'high':
      return 'var(--high)';
    case 'medium':
      return 'var(--medium)';
    case 'low':
      return 'var(--low)';
    default:
      return 'var(--info)';
  }
}

// criticalityFromFactor mirrors internal/scanner/normalize.go CriticalityLabel.
export function criticalityFromFactor(f: number): AssetCriticality {
  if (f >= 2.75) return 'CRITICAL';
  if (f >= 2.0) return 'HIGH';
  if (f >= 1.0) return 'MEDIUM';
  return 'LOW';
}

export function scheduleLabel(minutes: number, lang: LocaleCode): string {
  switch (minutes) {
    case 60:
      return lang === 'fr' ? 'Horaire' : 'Hourly';
    case 1440:
      return lang === 'fr' ? 'Quotidien' : 'Daily';
    case 10080:
      return lang === 'fr' ? 'Hebdo' : 'Weekly';
    case 0:
      return lang === 'fr' ? 'Manuel' : 'Manual';
    default:
      return lang === 'fr' ? `${minutes} min` : `${minutes} min`;
  }
}

export function timeAgo(iso: string | null | undefined, lang: LocaleCode): string {
  if (!iso) return '—';
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '—';
  const secs = Math.max(0, Math.round((Date.now() - then) / 1000));
  const ago = lang === 'fr' ? 'il y a' : '';
  const suffix = lang === 'fr' ? '' : 'ago';
  const fmt = (n: number, u: string) =>
    (lang === 'fr' ? `${ago} ${n}${u}` : `${n}${u} ${suffix}`).trim();
  if (secs < 60) return lang === 'fr' ? "à l'instant" : 'just now';
  if (secs < 3600) return fmt(Math.floor(secs / 60), 'm');
  if (secs < 86400) return fmt(Math.floor(secs / 3600), 'h');
  return fmt(Math.floor(secs / 86400), 'd');
}
