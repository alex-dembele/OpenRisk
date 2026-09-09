# Self-hosting OpenRisk

Self-hosting is a first-class product and our main acquisition channel. The goal:
**one command, works on the first try, zero manual steps.** A monthly CI job
([`.github/workflows/selfhost-install.yml`](../.github/workflows/selfhost-install.yml))
provisions a fresh Ubuntu 24.04 VM and runs this exact flow end-to-end.

## Requirements

- Docker Engine + Docker Compose plugin
- `openssl` (for secret/key generation)
- ~2 GB RAM, 2 vCPU, 10 GB disk to start
- Verified clean on **Ubuntu 24.04** and **Debian 12**

## Install (one command)

```bash
curl -fsSL https://raw.githubusercontent.com/opendefender/OpenRisk/master/scripts/install.sh | bash
```

Or from a clone:

```bash
git clone https://github.com/opendefender/OpenRisk.git
cd OpenRisk
./scripts/install.sh
```

The installer:

1. checks Docker + Compose,
2. generates an **RS256 keypair** (`deploy/selfhost/secrets/`) — the backend
   requires this and its absence is the classic "manual step" we remove,
3. writes a `.env` with **strong random secrets** (DB password, MFA/scanner/audit
   keys),
4. builds and starts postgres + redis + backend + frontend,
5. waits for `/api/v1/health`,
6. **creates the first administrator and prints its credentials.**

It is **idempotent**: re-running keeps your `.env`, your keys and your
administrator account — a second run reports the account already exists rather
than creating another.

App: `http://localhost:3000` · API: `http://localhost:8080/api/v1`.

### The first administrator

The installer ends with something like:

```
[openrisk] Sign in with:
[openrisk]    • Email:    admin@openrisk.local
[openrisk]    • Password: 8xKq2mRt7vNc4Wb9Ld3Yp6Zs1Hf5Gj0A
[openrisk] This password is shown once and stored nowhere. Save it now.
```

The password is 32 alphanumeric characters from OpenSSL's CSPRNG. It is printed
**once** and written to no file — not to `.env`, not to a log. Save it before
you close the terminal; if you lose it, reset it from the app.

The account is created through the product's own `POST /api/v1/auth/register`,
the same endpoint every other account uses, so it cannot drift from how
registration actually behaves. That endpoint also creates your organisation and
makes this user its root member.

Override the defaults with environment variables:

| Variable | Default |
|---|---|
| `OPENRISK_ADMIN_EMAIL` | `admin@openrisk.local` |
| `OPENRISK_ADMIN_USERNAME` | `admin` |
| `OPENRISK_ADMIN_NAME` | `OpenRisk Administrator` |
| `OPENRISK_ORG_NAME` | `My Organization` |
| `OPENRISK_SKIP_ADMIN` | unset — set to `1` to create no account and register through the UI instead |

```bash
OPENRISK_ADMIN_EMAIL=ops@your-bank.cm OPENRISK_ORG_NAME="Your Bank" ./scripts/install.sh
```

## What a self-hosted instance includes

Self-hosting gives you **the whole codebase**, not a crippled build: there is no
feature flag in this repository that a paid licence unlocks and a self-hoster
cannot. What differs is the **plan** your organisation resolves to, which is the
same open-core matrix the SaaS applies — enforced in
[`backend/pkg/entitlements/entitlements.go`](../backend/pkg/entitlements/entitlements.go)
and in the middleware, not in the frontend.

The table below is **generated from that file** by
`go test ./pkg/entitlements/ -run TestSelfHostDoc`, which fails if this page and
the code ever disagree. It is not a marketing table; it is the code.

<!-- BEGIN GENERATED: entitlements matrix -->
<!-- Generated from backend/pkg/entitlements/entitlements.go.
     Do not edit by hand: `go test ./pkg/entitlements/ -run TestSelfHostDoc -update`. -->

| Capability | Free | Pro | Business | Enterprise |
|---|---|---|---|---|
| Users | 2 | 10 | 50 | unlimited |
| Risks | 50 | 500 | unlimited | unlimited |
| Assets | 50 | unlimited | unlimited | unlimited |
| Integrations | 1 | 10 | unlimited | unlimited |
| REST API | limited | yes | yes | yes |
| Automation rules | — | yes | advanced | advanced |
| AI advisor | — | yes | advanced | advanced |
| Compliance frameworks | basic | standard | advanced | custom |
| SSO (SAML / OIDC) | — | — | yes | advanced |
| Multi-tenant | — | — | — | yes |
| On-premise entitlement | — | — | — | yes |
| Financial quantification | — | yes | yes | yes |
| SmartScore | — | yes | yes | yes |
| Executive dashboard | — | yes | yes | yes |
| Scanner | — | yes | yes | yes |
| Threat intelligence (CTI) | — | — | yes | advanced |
| Governance | — | — | yes | advanced |
| SLA | — | — | 99.5% | 99.9% |
| Support | community | email | priority | dedicated |

A self-hosted instance registers its first organisation with no plan set, so it resolves to **Free**: 2 users, 50 risks, 50 assets, 1 integration(s).

<!-- END GENERATED: entitlements matrix -->

To lift those caps on an instance you run yourself, set the organisation's plan
in the database or attach a subscription; the entitlement service reads the
subscription first and the organisation's stored plan otherwise
(`backend/internal/application/entitlements/service.go`). **Whether a
self-hosted instance should default to something other than Free is an open
question for the owner — D-040 in [DECISIONS.md](DECISIONS.md).** Until it is
answered, this page describes what the code does today rather than what the
offer might become.

### Licensing

- The core is **GNU AGPL-3.0-only** ([`LICENSE`](../LICENSE)). Every source file
  carries `SPDX-License-Identifier: AGPL-3.0-only`. You may run it, modify it and
  self-host it, including commercially, provided you honour the AGPL — notably
  §13: if you offer a modified version to users over a network, they are entitled
  to its source.
- The commercial edition is a separate licence,
  [`LICENSE.commercial`](../LICENSE.commercial) (`LicenseRef-OpenRisk-Commercial`),
  for organisations that cannot accept the AGPL's terms. See
  [LICENSING.md](../LICENSING.md).
- This project is **not** BUSL-licensed; if you have read that somewhere, it is
  out of date.

### Configuration

Everything lives in `deploy/selfhost/.env` (template: `.env.example`). Notable
optional keys:

- **Payments:** `STRIPE_SECRET_KEY`, `NOTCHPAY_PUBLIC_KEY`, `CINETPAY_API_KEY` +
  `CINETPAY_SITE_ID`. Empty ⇒ Free plan + manual/sales upgrades (no fake URLs).
- **Telemetry:** `OPENRISK_TELEMETRY=off` hard-disables telemetry (see
  [TELEMETRY.md](TELEMETRY.md)). Default: opt-in from the app.
- **Public URL / CORS:** `APP_BASE_URL`, `CORS_ORIGINS`, `VITE_API_URL` when you
  put OpenRisk behind a real domain / reverse proxy.

## Upgrade

```bash
cd OpenRisk
git pull
./scripts/backup.sh                 # always back up first
cd deploy/selfhost
docker compose pull || true         # if using prebuilt images
docker compose up -d --build        # rebuild + restart; migrations run on boot
```

Schema migrations (GORM AutoMigrate + SQL migrations) run automatically on
backend start. Your `.env` and `secrets/` are preserved across upgrades.

## Backup & restore

```bash
./scripts/backup.sh                 # → backups/openrisk-backup-<stamp>.tar.gz
./scripts/restore.sh backups/openrisk-backup-<stamp>.tar.gz
```

The tarball contains the Postgres dump, the `secrets/` (encryption keys) and the
generated data exports. **Store it off-box** — it holds your keys. Restore
overwrites the current database and secrets.

## ARM64 (AWS Graviton, Apple Silicon, Raspberry Pi)

Base images (`postgres`, `redis`, `alpine`, `golang`) are multi-arch, so the
stack builds and runs natively on ARM64 — `./scripts/install.sh` works unchanged
on a Graviton VM. To build and publish a multi-arch image:

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t openrisk/backend:latest ./backend --push
docker buildx build --platform linux/amd64,linux/arm64 \
  -t openrisk/frontend:latest ./frontend --push
```

## Kubernetes (Helm)

A maintained chart lives in [`helm/openrisk`](../helm/openrisk) with
`values-dev.yaml` / `values-staging.yaml` / `values-prod.yaml`. It schedules on
ARM64 nodes when you provide ARM64 images (above). Provide the same secrets
(`RSA_*`, `MFA_ENCRYPTION_KEY`, `SCANNER_CREDENTIAL_KEY`, `AUDIT_EXPORT_KEY`) and
optional payment/telemetry env via the chart's `values` / a `Secret`.

## Troubleshooting

- **Backend restarts / "RSA keys required":** the `secrets/` keypair is missing —
  re-run `./scripts/install.sh` (it regenerates only what's absent).
- **Port already in use:** set `BACKEND_PORT` / `FRONTEND_PORT` in `.env`.
- **Logs:** `cd deploy/selfhost && docker compose logs -f backend`.
