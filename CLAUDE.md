# OpenRisk — Constitution

Enterprise open-source GRC platform. Part of the OpenDefender suite.
Market: France, Belgium, Maghreb, CANADA, USA and Sub-Saharan Africa.
Repository: https://github.com/opendefender/OpenRisk

> This file is the permanent contract. It stays under 400 lines.
> History lives in `docs/JOURNAL.md`. Module status lives in `ROADMAP.md`.
> Domain specs live in `.claude/skills/`. **Never grow this file with a changelog.**

## Sources of truth — read in this order, never guess

| Question | File | How to read it |
|---|---|---|
| What is the work? | GitHub issues | `gh issue view <n> --comments` |
| Is a module done? | `ROADMAP.md` | Read the module section only |
| Was this done before? | `docs/JOURNAL.md` | `grep -n` first, never read whole |
| Is a claim true? | `docs/MARKETING_CLAIM_MATRIX.md` | Read the row |
| Why was it decided? | `docs/adr/` | Read the ADR |
| What is pending my call? | `docs/DECISIONS.md` | Read open items |

## Stack

- Backend — Go 1.25 · Fiber v2 · GORM · PostgreSQL 16 · Redis · golang-migrate
- Frontend — React 19 · TypeScript strict · Zustand · Tailwind 3 · Recharts · React Router v7
- Infra — Docker multi-stage · Kubernetes + Helm · GitHub Actions
- Obs. — Prometheus · Grafana · Loki · zerolog JSON · Sentry

## Architecture — mandatory

Backend, strict Clean Architecture:
```
/cmd/server/main.go        DI container, graceful shutdown
/internal/domain/          pure entities — NO Fiber, NO GORM here
/internal/application/     use cases — one use case = one file
/internal/infrastructure/  repositories, messaging, integrations
/internal/api/http/        Fiber handlers + middleware
/pkg/                      shared packages (scoring, cti, notify, export, ai, crq)
```

Frontend, feature-based:
```
/src/features/[module]/    pages, components, hooks, stores per feature
/src/shared/               design system, global hooks, utils
/src/services/             typed API client (generated from OpenAPI)
/src/locales/              fr.json, en.json
```

## ABSOLUTE RULES — never violated, by anyone, for any reason

1. Read every existing file of a module before writing a single line.
2. **Filter by `tenant_id` on EVERY DB query — no exception.** A missing tenant
   filter is a P0 security defect, not a bug. If a table has no `tenant_id`,
   gate through its parent entity and say so explicitly.
3. Typed errors only: `ErrNotFound`, `ErrForbidden`, `ErrConflict`, `ErrValidation`.
4. Minimum tests per use case: `TestXxx_Success` + `TestXxx_NotFound` + `TestXxx_Unauthorized`.
5. Zero `any` in TypeScript. Strict typing throughout.
6. Never log secrets — tokens, passwords, keys.
7. DB transactions on every multi-table operation.
8. Skeleton loaders on the frontend — never a full-page spinner.
9. Always handle the three UI states: loading + error + empty.
10. Optimistic updates on every critical mutation.
11. Zod validation client-side on every form.
12. **INVENT NOTHING.** No capability is documented, marketed, or reported as
    done unless the code exists and a user can reach it. Proof is a file path
    plus a passing test. A screenshot is not proof. A plan is not proof.

## Score Engine — the formula is frozen

```
Score = Probability (0.0–1.0) × Impact (0.0–10.0) × AssetCriticality (0.1–3.0)
critical ≥ 7.0 · high ≥ 4.0 · medium ≥ 2.0 · low < 2.0
```
`Risk.Score` from `pkg/scoring/engine.go` remains the source of truth.
SmartScore (`pkg/scoring/smart.go`, 8 weighted factors, 0–100) is **additive**
and never replaces it. Changing either formula requires an ADR and my approval.

## Project management — GitHub is the single source of truth

**No work exists outside an issue.** No issue, no work. If asked to do something
untracked, open the issue first or say one is needed.

### Ready definition
An issue is workable only with: a problem statement in user terms, numbered
testable acceptance criteria, a Definition of Done, a milestone, and the three
labels `type:` / `area:` / `priority:`. Otherwise it carries
`status:needs-refinement` and is not implemented.

### Label taxonomy — fixed, never invent one
| Prefix | Values |
|---|---|
| `type:` | feature · bug · chore · security · docs · design · debt |
| `area:` | backend · frontend · infra · design · marketing · docs · db |
| `priority:` | P0-critical · P1-high · P2-medium · P3-low |
| `status:` | needs-refinement · ready · in-progress · blocked · in-review |
| `tier:` | 0-trust · 1-product-engine · 2-killer-workflow · 3-ecosystem · 4-ux · 5-deferred |

`P0-critical` = production broken or exposed. Bypasses the milestone, worked now.

`tier:` was added by D-032 on 2026-09-07 and is the only prefix ever added
to this table. Six values, no seventh. It is orthogonal to `priority:`: priority says how
urgent, tier says how strategic. An issue may be `priority:P1-high` and `tier:5-deferred`
— urgent if we did it, and not what we are doing.

### Branch, commit, PR
- Branch: `gh issue develop <n> --checkout` → `<type>/<n>-<slug>`
- Commit: `type(scope): subject (#<n>)` — Conventional Commits, English, imperative
- PR body contains `Closes #<n>`. One issue → one branch → one PR.
- **Never merge a PR.** Merging is the owner's decision.

### The loop every agent follows on an issue
1. `gh issue view <n> --comments` — read the issue and its discussion.
2. If not `status:ready`, stop and report why.
3. `gh issue edit <n> --add-label status:in-progress --remove-label status:ready`
4. Work on the linked branch.
5. **Post progress as an issue comment before you finish** — see the report
   format below. This comment is what lets a fresh session resume your work
   with zero re-reading. It is not optional.
6. Open the PR with `Closes #<n>`, set `status:in-review`.
7. Never close an issue. The merge closes it.

### Mandatory issue comment format (this is the resume anchor)
```markdown
### <agent-name> — <date>
**Done** — what changed, with file paths.
**Verified** — the exact commands run and their result.
**Criteria** — 1 ✅ · 2 ✅ · 3 ❌ (reason)
**Next** — the precise next action for whoever picks this up.
**Blocked on** — nothing | @owner decision | issue #N
```

## Autonomy — what agents decide, what reaches me

Agents decide and proceed on: implementation approach within the architecture,
naming, test strategy, refactors under 200 lines inside one module, copy
wording, label and milestone hygiene, bug fixes with an existing issue.

Agents **stop and escalate** by appending to `docs/DECISIONS.md` and reporting
in the next brief, on: any change to the Score Engine or SmartScore formula ·
any new external dependency or paid service · any schema migration that drops
or renames a column · anything touching auth, crypto, or tenant isolation
design (fixes are fine, redesign is not) · pricing, licensing, or brand naming ·
cutting a milestone's scope · anything costing money · anything irreversible.

**Escalation is cheap, silence is expensive.** When in doubt, escalate and keep
working on something else. Never block waiting for me.

## Agent operating rules

- Read before you write. Never assume file content.
- Smallest correct change. No opportunistic refactors.
- Ambiguity: state it, state your chosen interpretation, proceed.
- Never report done without pasting the verification command output.
- Never say "should work". Either it is verified or it is not done.
- Honest remainders: every report ends with what is NOT done and why.
