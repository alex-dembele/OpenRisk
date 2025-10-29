# API Reference for OpenRisk

## Sync-Engine Endpoints
- GET /sync_opencti: Manual trigger OpenCTI sync.
- GET /sync_thehive: Manual TheHive sync.
- GET /generate_report: Create consolidated report.

## Dashboard Endpoints (FastAPI)
- GET /risks: List from OpenRMF.
- GET /threats: From OpenCTI.
- GET /incidents: From TheHive.
- GET /actions: From Cortex.
Auth: Bearer token from Keycloak.

## Tool APIs
Refer to official docs:
- OpenRMF: /api/risks
- TheHive: /api/v1/case
- Cortex: /api/job
- OpenCTI: /graphql