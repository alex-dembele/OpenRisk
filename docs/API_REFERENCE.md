# OpenRisk API Reference

**Version**: 1.0.0 | **Base URL**: `/api/v1` | **Auth**: Bearer JWT

## Endpoints

### Health
- `GET /health` - Health check

### Authentication
- `POST /auth/login` - Login (email, password)
- `GET /users/me` - Current user profile

### Risks
- `GET /risks` - List risks (query: page, limit, sort_by)
- `POST /risks` - Create risk
- `GET /risks/{id}` - Get risk
- `PATCH /risks/{id}` - Update risk
- `DELETE /risks/{id}` - Delete risk

### Mitigations
- `POST /risks/{id}/mitigations` - Add mitigation
- `PATCH /mitigations/{mitigationId}` - Update mitigation
- `PATCH /mitigations/{mitigationId}/toggle` - Toggle status (PLANNED↔DONE)

### Mitigation Sub-Actions
- `POST /mitigations/{id}/subactions` - Create sub-action
- `PATCH /mitigations/{id}/subactions/{subactionId}/toggle` - Toggle completion
- `DELETE /mitigations/{id}/subactions/{subactionId}` - Delete sub-action
- `GET /mitigations/recommended` - Get recommended mitigations

### Assets
- `GET /assets` - List assets
- `POST /assets` - Create asset

### Statistics
- `GET /stats` - Dashboard stats
- `GET /stats/risk-matrix` - Risk matrix (impact vs probability)
- `GET /stats/trends` - Risk trends

### Export
- `GET /export/pdf` - Export risks to PDF

### Gamification
- `GET /gamification/me` - User gamification profile

---

## Request/Response Examples

See `docs/openapi.yaml` for complete OpenAPI 3.0 specification with detailed schemas, validation rules, and examples.

## Authentication

All protected endpoints require:
```
Authorization: Bearer {token}
```

Obtain token via `POST /auth/login` (valid for 72 hours).

## Error Format

```json
{
  "error": "Error message",
  "code": 400,
  "details": {}
}
```

Common codes: 400 (Bad Request), 401 (Unauthorized), 404 (Not Found), 500 (Server Error)

---

## Deprecated endpoints

These three server-sent-event endpoints are **deprecated** and will be **removed in 1.2.0**.
Every response carries the announcement on the wire, per
[RFC 8594](https://www.rfc-editor.org/rfc/rfc8594):

```
Deprecation: true
Sunset: Wed, 02 Dec 2026 00:00:00 GMT
Link: <the replacement path>; rel="successor-version",
      <.../API_REFERENCE.md#deprecated-endpoints>; rel="deprecation"
```

| Deprecated | Replacement | Streaming? | Why |
| --- | --- | --- | --- |
| `GET /mitigations/events` | `GET /realtime/events?aggregates=mitigation` | **Yes** | Accepts the access token as a **query parameter**, because `EventSource` cannot set an `Authorization` header. A credential in a URL reaches access logs, proxy logs, browser history and any leaked `Referer`. The replacement authenticates with the HttpOnly session cookie. |
| `GET /scanner/events` | `GET /scanner/jobs` | No — polling | Requires a `Bearer` header no browser `EventSource` can send, so nothing in the console ever consumed it. |
| `GET /reports/{reportId}/progress` | `GET /reports/{reportId}` | No — polling | One connection per report, predating the shared hub. The polling endpoint returns the same `progress`, `step` and `run_state` this stream pushed. |

**Read the "Streaming?" column before you migrate.** Only the mitigation stream has a like-for-like
replacement. The realtime catalog carries `mitigation.*` events but **no `scan.*` and no
`report.*`** — see `GET /realtime/catalog` for the authoritative list — so a consumer of the other
two moves from push to poll and loses latency. If that trade is unacceptable for your deployment,
say so on issue #347 before the sunset date: adding those events to the hub is the prerequisite
for a streaming migration, and nobody has asked for it yet.

**What the mitigation replacement gives you**: one connection per tab instead of one per feature,
a single reconnect strategy, resumption from a cursor via `Last-Event-ID`, and the canonical event
envelope so a redelivery after a reconnect can be de-duplicated on `id`.

**Not deprecated**: `GET /scanner/agent/stream`. That is the Agent's own job channel, a different
endpoint with a different credential, and it is unaffected.

**Compatibility window.** These endpoints were never published in
[`openapi.yaml`](./openapi.yaml), so nothing about them was promised in the contract. They keep
working until 1.2.0; after that they are gone. If you operate a self-hosted deployment with a
script against any of them, migrate before the sunset date above.

---

**Full specification**: [openapi.yaml](./openapi.yaml)  
**Last updated**: September 3, 2026
