# Integration Guide for OpenRisk

## Overview
OpenRisk integrates OpenRMF, TheHive, Cortex, and OpenCTI via APIs and the sync-engine. This guide covers configuration for inter-tool communication.

## API Configuration
1. OpenCTI: Generate API token in UI (Settings > API Access). Set in .env: `OPENCTI_TOKEN=your_token`
2. TheHive: Create API key in Organization > API Keys. Set `THEHIVE_API_KEY=your_key`
3. OpenRMF: Uses JWT from Keycloak. Configure realm in Keycloak UI, set `JWTAUTHORITY=http://keycloak:8080/realms/openrmf`
4. Cortex: No auth by default; enable in config if needed.

## Sync-Engine Setup
- Scripts in sync-engine/sync.py handle syncing.
- Customize queries in code (e.g., GraphQL for OpenCTI).
- Run manually: `python sync.py sync_opencti_to_openrmf`
- Cron: In Docker, built-in schedule; on Linux, add to crontab: `0 2 * * * /usr/bin/python /app/sync-engine/sync.py`

## Automations
- OpenCTI to OpenRMF: Pull STIX objects, post as risks.
- TheHive to OpenRMF: Update risk statuses from incidents.
- TheHive to Cortex: Trigger analyzers via webhooks (configure in TheHive UI).
- Reports: Generated daily, stored in /reports volume.

## Dashboard Integration
- Backend proxies APIs with auth headers.
- Frontend fetches from /api endpoints.
- Customize queries in backend/main.py.

## Testing Integrations
1. Create test threat in OpenCTI.
2. Run sync, verify in OpenRMF.
3. Monitor logs for errors.