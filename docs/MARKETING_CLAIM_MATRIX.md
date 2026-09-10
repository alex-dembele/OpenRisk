# Marketing Claim Matrix — OpenRisk

Every public statement about what OpenRisk does lives here with its proof.
Only the `product-verifier` agent may set a status to `VERIFIED`.

Status: `VERIFIED` · `PARTIAL` · `MOCKED` · `ABSENT` · `PLANNED`

| ID | Claim (EN) | Claim (FR) | Status | Evidence (file:line + test) | Surface | Verified on |
|----|-----------|-----------|--------|------------------------------|---------|-------------|
| C-001 | _seed row — run /verify-claims to populate_ | — | ABSENT | — | — | — |
| C-002 | A new organization reaches its first real risk insight — a Posture Reveal computed from its own risks and controls — in under 8 minutes. | Une nouvelle organisation atteint sa première information de risque réelle — une Révélation de posture calculée à partir de ses propres risques et contrôles — en moins de 8 minutes. | PARTIAL | **Point de mesure : l'événement `posture.revealed`** (`backend/internal/domain/activation.go`, `ActivationPostureRevealed`), enregistré une seule fois par tenant par `PostureUseCase.Execute` (`backend/internal/application/activation/posture.go`). **Test E2E qui le prouve : `tests/e2e/activation.spec.ts` › `posture reveal — the Aha moment (#438)` › `a revealed posture is computed from the tenant own rows, and is idempotent`** (assertion `AHA_BUDGET_MS = 8 * 60 * 1000` sur la durée signup → reveal). Mesure : `openrisk_time_to_aha_seconds{aha_definition="v2"}` (`backend/pkg/monitoring/activation.go`) — la série v1, définie par le tableau de bord exécutif, est gelée et non comparable (D-010). Refus explicite d'un reveal vide : `tests/e2e/activation.spec.ts` › `an empty posture is refused, records nothing, and measures nothing`. | Site, docs produit | 2026-09-10 — **non promu.** La suite E2E n'a pas été exécutée dans cet environnement (ni backend ni frontend démarrés), donc le test nommé ci-dessus existe mais n'est pas prouvé vert ici. Les tests unitaires backend qui couvrent la même logique sont verts (`go test ./internal/application/activation/`, 45 tests). `product-verifier` doit lancer la suite E2E avant tout passage à `VERIFIED`. |
| C-003 | The onboarding checklist reflects what the organization has actually done — including work completed before the checklist existed. | La checklist d'onboarding reflète ce que l'organisation a réellement fait — y compris le travail accompli avant l'existence de la checklist. | PARTIAL | `backend/internal/infrastructure/repository/gorm_activation_backfill.go` + `gorm_activation_backfill_test.go` (7 tests : amorçage, idempotence sur redémarrage, absence de preuve = absence de coche, isolation multi-tenant) ; `go test ./internal/infrastructure/repository/ -run BackfillDerived` | Docs produit | 2026-08-31 — tests unitaires verts ; l'étape `profile` reste non dérivable (aucun enregistrement ne la prouve). |

## Blocking rule

`MOCKED` and `ABSENT` claims must not appear on any public surface.
`PLANNED` claims must be future tense and visually marked.

## Time to first value — one number

The committed promise is **8 minutes**, and it is the only number that may
appear on a public surface. Two other numbers exist in the tree and are NOT the
promise:

- **12 minutes** — `SlowTimeToAha` in `deployment/monitoring/alerts.yml`. An
  operational warning threshold, deliberately set above the promise so the page
  has headroom and does not fire on every ordinary bad day. It is not a target
  and must never be quoted as one.
- **5 minutes** — appeared in an old `ROADMAP.md` row (17.6) and in the wording
  of issues #234 and #438. Corrected: no test, metric or alert has ever asserted
  it. #438 restates it as an internal design target that is **not published**
  (D-0xx+2 on that issue); the committed promise stays 8 minutes.

Changing the promise means changing `AHA_BUDGET_MS` in
`tests/e2e/activation.spec.ts`, this row, and the alert's `description` in the
same commit. See `docs/DECISIONS.md` D-008.
