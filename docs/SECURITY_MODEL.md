# Security Model for OpenRisk

## Overview
Security is prioritized with RBAC, encryption, and least privilege.

## Roles and Access
- Admin: Full access (create/users, configs).
- Analyst: Read/write risks/incidents.
- Viewer: Read-only.
Configure in Keycloak for OpenRMF, TheHive orgs, OpenCTI groups.

## Authentication
- OAuth/JWT via Keycloak.
- API keys for tools.
- HTTPS mandatory in prod.

## Data Protection
- Encrypt volumes (e.g., LUKS on Linux).
- Secrets in .env/K8s Secrets.
- Audit logs enabled in all tools.

## Network Security
- K8s NetworkPolicies to restrict traffic.
- Firewall rules: Only expose necessary ports (443, etc.).

## Compliance
- Aligns with NIST, CIS benchmarks.
- Scan images with Trivy.