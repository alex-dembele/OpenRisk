# OpenRisk — Owner Decision Register

Anything an agent may not decide alone lands here. `po-openrisk` consolidates,
recommends, and surfaces these in the daily brief. Run `/decide` to clear them.

## Open

### D-029 — release trains replace the eight waves as the planning unit · raised 2026-09-04 · **pending**
**Raised by** — `po-openrisk`, executing the founder's strategic note of 2026-09-04.
**Issues** — #570 (epic) and the six new milestones.

**The question** — Axe F of the note asks for "des releases thématiques (ex: Release 1.2 -
Risk Auto-Draft)" instead of technical waves. I have created six milestones — `Release 1.2 —
Risk Data Plane` through `Release 1.6 — IT-native & Ecosystem`, plus `Backlog — deferred
(Tier 5)` — and moved 46 open issues into them. **The eight `Wave` milestones still exist and
still hold 44 open issues.** That is the decision: do the waves get retired, with their
remaining issues redistributed, or do both families coexist?

**Recommendation** — **retire the waves, redistribute their issues.** Two milestone families
is the same mistake D-003 accepted for labels, and the cost there is visible: every issue now
carries two priority labels that can disagree, and they do. Milestones are worse than labels
for this, because a milestone is what a burn-down and the `/plan-milestone` skill read. An
issue in `Wave 4 — GRC` and an issue in `Release 1.4 — Evidence Fabric` cannot both be "the
current milestone". If the owner prefers to keep the waves as an archive, they should be
closed rather than left open, so nothing new can be filed into them.

**What I did not do** — I did not retire them. Reshaping milestone scope is an escalation
under CLAUDE.md, so the waves are untouched and the moves I made are reversible one by one.

**Cost of delay** — moderate and rising. Until this is answered the backlog has two competing
plans, and every new issue forces an agent to guess which family it belongs to. Answer before
the next milestone planning round.

---

### D-030 — adopt the OpenRisk Canonical Model, and commit to OSCAL as an interchange format · raised 2026-09-04 · **pending**
**Raised by** — `po-openrisk`. **Issues** — #541, #542, #543, #566, epic #535.

**The question** — two commitments, one decision, because the second is only worth making if
the first is:

1. **A canonical domain model** — 17 entities with a mandatory envelope (`source`,
   `source_id`, `source_version`, `observed_at`, `confidence`, `provenance`, `relationships`)
   and a fixed relationship vocabulary, published as `openrisk-schema.json` with a semver
   compatibility promise. This is the largest architectural commitment in the project's
   history: it touches every table, every integration and every module, and once external
   parties build against the published schema, breaking it costs them, not just us.
2. **OSCAL** — NIST's machine-readable format for controls, profiles, implementations and
   assessment results, as our import/export interchange. This is a dependency on an external
   standard's evolution.

**Recommendation** — **accept both, in that order, with 1 as an ADR before any code.** The
note's argument is right and it is the strategic core of the whole document: without a
canonical model we accumulate connectors until they are unmaintainable, and every automation
ambition in the roadmap sits on sand. OSCAL is a smaller decision and a large credibility
return with regulated and public-sector buyers — but it is only coherent *after* the canonical
model, because OSCAL then maps onto something stable rather than onto our current table shapes.

**What this costs if wrong** — a canonical model that turns out to be badly factored is
expensive to change after external adoption, which is exactly why #541 asks for an ADR and an
explicit out-of-scope list rather than an implementation.

**Cost of delay** — high. Eleven open issues are blocked behind #541, and both killer
workflows depend on it. This is the decision to answer first.

---

### D-031 — nine issues are deferred to Tier 5 · raised 2026-09-04 · **pending confirmation**
**Raised by** — `po-openrisk`. **Issues** — #509, #510, #511, #512, #513, #397, #222, #205, #403.

**The question** — the note is explicit that exotic visualisations, the Digital Twin and
Attack Path work should not precede the risk-to-evidence-to-mitigation chain. I have moved
those nine to `Backlog — deferred (Tier 5)` and commented the reason on each. Cutting scope is
an escalation, so this is here for confirmation rather than as a fait accompli.

- **Charts** (#509 Sankey, #510 Choropleth, #511 Candlestick, #512 Scatter, #513 Brush) — none
  has an identified consumer. #512 says so in its own title.
- **Too early** (#397 and #222 Digital Twin, #205 Attack Graph, #403 blast radius) — all four
  are graph and simulation work whose output quality is capped by data quality. Built before
  the Risk Data Plane, they simulate our normalization gaps rather than the customer's
  environment.

**Recommendation** — **confirm.** Nothing is closed; every spec survives intact and returns
the moment a Tier 1 or Tier 2 issue names it as a requirement, which is a stronger position
than being built speculatively now.

**Cost of delay** — none. The moves are already made and are reversible in one command each.
Say the word if any of the nine should come back.

---

### D-032 — a `tier:` label prefix, or tiers stay in milestones only · raised 2026-09-04 · **pending**
**Raised by** — `po-openrisk`. **Issue** — #570.

**The question** — the note's tier model (Tier 0 Trust · 1 Product engine · 2 Killer workflows ·
3 Ecosystem · 4 UX · 5 Deferred) is the sharpest instrument in the whole document, and it is
the one thing I could not implement. CLAUDE.md says the label taxonomy is fixed and no agent
invents a prefix. So tiers currently live only in milestone names and in #570's body, where
they cannot be filtered on.

**Recommendation** — **authorise a `tier:` prefix with exactly those six values, and no
seventh.** A tier is orthogonal to `priority:` and answers a different question: priority says
how urgent, tier says how strategic, and the note's entire argument is that we keep confusing
the two. The cost is one more label per issue on a repository that D-003 already committed to
double-labelling. The alternative — tiers as milestones only — means an issue's tier changes
when it is scheduled, which defeats the purpose.

**If the answer is no**, that is a perfectly workable outcome: #570 then records tiers in
`ROADMAP.md` as a static table, and the milestone name carries the signal. Say so and I will
stop asking.

**Cost of delay** — low. #570 proceeds either way.

---

### D-033 — the positioning line, after the name is settled · raised 2026-09-04 · **pending**
**Raised by** — `po-openrisk`. **Issues** — #571 (`status:blocked`), depends on #492.

**The question** — the note proposes replacing *Unified Risk & Threat Intelligence Management
Platform* with a line built on "continuous risk intelligence", offering three candidates, and
proposes replacing the internal "anti-spreadsheet" framing with "anti-manual-control-plane".
Brand naming and positioning are owner-only.

**Recommendation** — **do not decide this yet.** #492 has not settled OpenRisk versus Karath,
and a tagline chosen before the name is a tagline written twice. More importantly, every
candidate line is a *claim about continuous behaviour we have not shipped*: under ABSOLUTE
RULE 12 we may not put "continuous risk intelligence" on a website until #535 and #536 are
real. The right sequence is #492, then the killer workflow, then the line — tested on the
three pilot organisations from #494 rather than chosen in a room.

**Cost of delay** — none until the GTM milestone needs public copy. #571 is parked at
`status:blocked` and will not be picked up.

---

### D-028 — where the `Idempotency-Key` record lives, and which endpoints must carry it · raised 2026-09-03 · **pending**
**Raised by** — `po-openrisk`, splitting #335. **Issue** — #517 (`status:needs-refinement`).

**The question** — #335 was one issue bundling a P0 transactional-write bug with a new
cross-cutting mechanism. The bug is now #335 alone and ships this week. The mechanism is #517
and cannot start until two things are decided:

1. **Storage.** Redis (already in the stack, TTL for free, but a lost key silently degrades to
   today's behaviour and the guarantee we document becomes conditional) versus a PostgreSQL
   table (durable, commits in the same transaction as the write it protects, but a new table and
   a migration).
2. **Reach.** Which mutation endpoints must carry the header, and whether it is optional or
   required. Requiring it is a breaking change for every existing client.

Three further design questions — key scope (key alone, or key + tenant + body hash),
concurrency semantics, and retention — are `tech-lead`'s to settle, not the owner's, once 1 and
2 are answered.

**Recommendation** — **PostgreSQL, and optional on every mutation endpoint.** Redis is
attractive until you write the sentence we would have to publish: "retrying is safe unless the
cache evicted your key", which is not a guarantee an integrator can build on. A durable record
that commits with the write is the only version that is true. Optional-not-required because
requiring the header breaks existing clients for a benefit only retrying clients need; make it
honoured when present, and document it. Recommend an ADR: this touches every mutation endpoint
and the reasoning should outlive the issue.

**Cost of delay** — low and bounded, which is exactly why the split was made. #335 removes the
cause of the duplicates users are actually reporting; what remains uncovered is the retry after
a response that never arrived. Nothing blocks on this until an integrator asks for a retry
guarantee, or until #239 (W8-06, Wave 8) reaches its idempotency line and finds no design. It
should not, however, sit unanswered past Wave 8 planning.

**Not urgent. Answer at the next convenient decision round, not ahead of anything on Wave 1.**

---

## Resolved

<!-- Append: date · decision · rationale · issues unblocked -->

### D-028 — anime.js is admitted as a second animation runtime, for SVG entry draw only · 2026-09-03
**Decided** — Adopt anime.js as #445 specifies. It enters as a real dependency
(`animejs@^4.5.0`), confined to one hook — `frontend/src/shared/ds/useAnimeScope.ts` —
and used for one job: drawing chart geometry once on first view.

**Rationale (owner)** — Chosen **over the recommendation**, which was to decline it and
use `framer-motion`, already a dependency in 57 files, whose SVG `pathLength` animation
covers the same case. The standing argument against a second runtime is the repository's
own lint rule on the banned `motion` package — *"Two animation runtimes in one bundle is
the size budget spent twice."* The owner accepts that cost for anime.js's SVG and scope
primitives. Recorded plainly because the reasoning cuts the other way and a future reader
should not have to reconstruct that it was considered.

**What it costs, measured 2026-09-03 on this branch —**
```
dist/assets/anime-*.js    37.31 KB raw    14.95 KB gzip   ← its own lazy chunk
preloaded initial bundle  170.6 KB ≤ 180 KB               ← unchanged by this work
```
anime.js gets its **own** `manualChunks` entry rather than riding in `charts`. That is
what makes the ceiling assertable at all: mixed in with visx there would be no number to
measure. `check-bundle-budget.mjs` counts only preloaded JS and therefore cannot see it,
so `scripts/check-anime-budget.mjs` is a **new, separate gate**.

**Three corrections to #445, all measured —**
1. **The topology use case does not exist.** #445 justifies anime.js partly by *"topology
   graph edge sequencing"* and scopes itself *"SVG uniquement"*.
   `features/attackSurface/TopologyView.tsx` draws its edges with the **Canvas 2D** API
   (`ctx.beginPath/moveTo/lineTo/stroke`, 13 canvas references); its one `<svg>` is a 26×6
   legend swatch. anime.js cannot animate a canvas. **Struck** — one use case, not two.
2. **The 12 KB ceiling is not reachable as specified.** #445 budgets
   "Timer 5.60 + Animation 5.20 + SVG 0.35 + Stagger 0.48 = 11.63 KB" but omits the
   **Scope** module, which its own first task mandates (`createScope` + `scope.revert()`,
   without which StrictMode's double mount leaks). Scope is ~3 KB. The measured floor for
   the module set the issue REQUIRES is **14.95 KB**, so the gate is set at **15.5 KB**.
   This is a new gate at its measured floor, not the relaxation of an existing one, and
   like every budget here it may only ever be lowered.
3. **`createDrawable` is deliberately unused.** It draws a path by driving
   `stroke-dasharray`/`stroke-dashoffset`, and the chart layer already spends
   `strokeDasharray` on the per-series DASHES pattern that satisfies *"colour is never the
   only encoding"*. Using it would animate that encoding away and land every line solid.
   Entry draw is done by clipping instead, which leaves the dash pattern intact.

**What binds —**
- **`useAnimeScope` is the only entry point.** Named imports only; no namespace import.
- **The `Scroll` module stays banned** (#445, 4.30 KB): *"the console never animates on
  scroll."* First-view detection uses a plain `IntersectionObserver`.
- **Never re-exported from `shared/ds/index.ts`.** That barrel is on the preloaded path;
  a re-export is exactly how anime.js would stop being lazy. `check-anime-budget.mjs`
  fails the build if it reaches the preloaded graph.
- **Reduced motion short-circuits the hook entirely** — the scope is never created — and
  callers must render their FINAL state when `willAnimate` is false. *"Reduced motion is
  not 'less motion'"*: a draw that never plays must not leave an empty frame.

**Reversible** — yes, and cheaply. One hook, one chunk, one dependency; the chart falls
back to its finished state by construction, so removing anime.js means deleting the hook
and the clip animation, not redesigning anything.

**Unblocked** — #445. #444's unticked "entry draw" task, which closed unbuilt, lands here.

### D-027 — Prettier is adopted and the tree is swept now, not at a later boundary · 2026-09-02
**Decided** — Option A. Prettier is adopted, the sweep is taken immediately in its
own PR (#505), and `format:check` becomes a blocking CI gate with no `||` fallback.
**Rationale (owner)** — Accepted the revised recommendation. The original advice was
to defer to the ds-v1 boundary because a repo-wide mechanical diff would collide
with the open ds-v1 PRs; on re-measurement that reason had evaporated — #497, #498
and #499 had all merged and the only open PR in the repo was #500 itself. The tree
was quiet at the moment of asking, and #475, #457 and #438 are `status:ready` and
would have ended that as soon as anyone started them. Deferring would have bought
nothing and cost the window.
**Consequence** — `prettier@3` is a devDependency; `.prettierrc` encodes the
measured house style (single quotes, semicolons, 2-space, printWidth 100) rather
than Prettier's defaults; 414 files are reformatted in one mechanical commit kept
separate from the tooling commit so the diff can be reviewed by ignoring it.
`eslint-config-prettier` is deliberately absent — the ESLint config carries no
stylistic rules, and the lint ratchet holding at exactly 321 across a 414-file
reformat is the evidence. Sweeping also surfaced the one file in the tree PostCSS
cannot parse, `NotificationBadge.css`, which had two lines of JavaScript at the top.
**Depends on** — #500 (#341), which rewrote the frontend lint job this gate lives
in. #500 merged on 2026-09-03; #506 was rebuilt on the resulting master.
**Unblocked** — #341 criterion "eliminate ESLint errors" keeps its ratchet; the
frontend lint job gains its third real gate.

---

### D-025 — the ExecutiveDashboard doughnut is converted to bars plus a KPI figure · 2026-09-02
**Decided** — Option A. `features/analytics/ExecutiveDashboard.tsx`'s risk-distribution doughnut
becomes severity **bars**, with the total shown as a KPI figure rather than sitting inside a ring.
The hand-rolled `Ring` component in the same file (per-framework compliance donuts) is converted
with it, so the file is not left half-compliant.

**Rationale (owner)** — Matches the recommendation. The chart is verbatim what #444's anti-cliché
table bans `RingChart` for — *"never a doughnut with a number in the middle that a KPI tile would say
better"* — and it carries up to **four** slices, past the three-slice pie limit as well. #444 exists
in part to make that doctrine executable; shipping the new chart layer while leaving in place the one
chart the doctrine names by description would make the rule decorative. #446 landed the deny-list at
`error` on 2026-09-01 and is not reopened.

**The facts —** the doughnut is `innerRadius={58} outerRadius={82}` with an absolutely-positioned
centre showing `fmtInt(total)`; its slices are critical/high/medium/low. `Ring` is a separate
hand-rolled SVG ring, not a Recharts component, so no charting migration would have touched it.

**Consequence** — this is a **visible redesign of an executive-facing view**, deliberately. It also
unblocks the last step of D-024 option A: with `ExecutiveDashboard` migrated, `recharts` can leave
`package.json` and PR #498 can leave draft.

**Reversible** — yes, cheaply. Presentation only; no data, schema, licence or dependency decision
rides on it, and the previous chart is one revert away.

**Unblocked** — #444, step 2 of the checkpoint on PR #498.

---

### D-026 — `ComplianceReportDashboard` is deleted; `AnalyticsDashboard` is kept · 2026-09-02
**Decided** — Option C. `pages/ComplianceReportDashboard.tsx` is **deleted**.
`pages/AnalyticsDashboard.tsx` is **kept**, already migrated off Recharts.

**Rationale (owner)** — Matches the recommendation. Both files import Recharts, nothing imports
either of them, and neither appears in the build output at all — they are dead code, and Recharts
cannot be removed from `package.json` while their imports stand. `AnalyticsDashboard`'s migration was
already done, so keeping it costs nothing further; migrating `ComplianceReportDashboard`'s thirteen
Recharts symbols would be work on a file no user can reach.

**Reversible** — yes. The deletion is in git history and restorable.

**Unblocked** — #444, step 3 of the checkpoint on PR #498.

---

### D-024 — bklit ui: declined on re-measurement; the chart layer is built in-house on visx · 2026-09-02, amended 2026-09-03
> **AMENDMENT — 2026-09-03, and this is the decision of record. Everything below the
> amendment box is the ORIGINAL 2026-09-02 decision, kept for the reasoning it carries
> and SUPERSEDED on the vendoring question.**
>
> **Decided (owner, 2026-09-03)** — amend to Option A. No bklit source is vendored.
> `frontend/src/shared/ds/charts/` is original work: `CartesianChart`, `PieChart` and
> `RadarChart`, all `Copyright (c) 2026 OpenDefender Contributors`, `Apache-2.0`, built
> on `@visx/*` as a normal npm dependency in the lazy `charts` chunk. The
> THIRD-PARTY COMPONENTS table in `frontend/design-system/NOTICE` therefore has no rows
> for this work, and consequence 5's "record each vendored component with its upstream
> SHA" is void rather than outstanding.
>
> **Why the reversal** — the implementation reached Option A and the record was not
> updated to match, which is the discrepancy this amendment exists to close. Measured on
> `master` at `7c50d3c`, the outcome is better than what Option B was chosen for:
>
> | | Option B baseline (recharts) | shipped (visx, in-house) |
> |---|---|---|
> | lazy `charts` chunk, gzipped | 87.0 KB | **16.2 KB** |
> | preloaded initial bundle | 172.1 KB | **170.6 KB** |
>
> The chart payload fell 81% and the preloaded bundle ended up 1.5 KB *below* master.
> Option B's own "facts" section already recorded the maintenance case against it:
> `@bklitui/ui` is `version: 0.0.0`, `"private": true`, never published — *"no version to
> pin, no changelog and no upgrade path — only a SHA and a manual re-diff."* Building
> in-house also disposes of consequences 2, 3 and 4 by construction: there is no vendored
> source importing `motion/react`, no transitive `@base-ui/react@1.0.0-alpha.8`, and no
> `GaugeChart` to drop. Verified on `master`: 0 files reference any of them.
>
> **What still binds** — consequence 1 (charts stay lazy, the budget may only ever be
> lowered) and consequence 5's *destination* half (`frontend/src/shared/ds/`, the
> Apache-2.0 half, per D-020). Consequence 6 (the risk matrix is built, not vendored)
> was always the plan and shipped as `RiskMatrix.tsx`.
>
> **What this does NOT license** — vendoring stays governed by D-020. If bklit source is
> ever taken, its notice is sourced and recorded first. This amendment declines the
> vendoring; it does not pre-approve a later one.
>
> **Scope consequence** — #444 closes at **Recharts parity**, which is what was built.
> The rest of its "Composants à adopter" table is split into #509 (Sankey), #510
> (Choropleth), #511 (Candlestick + Profit/Loss), #512 (Scatter) and #513 (Brush,
> Reference Area, Legend), each `status:needs-refinement` pending a named consuming
> screen. They are deliberately not built speculatively.
>
> **Reversible** — yes. Nothing is relicensed and no upstream is now depended on, so
> reversing means vendoring later under D-020, not undoing anything.

**Superseded — the original decision, 2026-09-02:** Option B. A **hard subset** of
`bklit/bklit-ui` charts is vendored at the pinned SHA
`c57f66bfa7c3198edb677b567ce08cbf364ae159` (2026-07-28, tip of `main`). `@visx/*` is
accepted as a real dependency set, in the **lazy** `charts` chunk. **The bundle gate is
NOT raised** — see the correction below. *(The `@visx/*` and lazy-chunk halves of this
survived the amendment; only the vendoring did not.)*

**Rationale (owner)** — Chosen over the recommendation, which was A (decline and
build in-house). The owner accepts the pre-1.0 upstream in exchange for a chart
layer that exists now rather than one built primitive by primitive.

**Correction, 2026-09-02, after the decision was taken —** the weight argument put
to the owner was **wrong**, and it was the main cost cited against Option B. Two
errors, both measured and corrected here:
1. **The budget is 180 KB, not 248 KB.** #462 closed and superseded D-018's 248 KB.
2. **Charts are not counted by the gate at all.** `frontend/scripts/check-bundle-budget.mjs`
   sums only the JS the entry HTML *preloads*, and its header says so explicitly:
   "Route-split chunks and on-demand chunks (**charts**, zxcvbn dictionaries, the
   command palette, forms…) are NOT counted." Recharts already lives in a lazy
   `charts` chunk (87.0 KB gzipped) that the budget ignores.

Measured on `master` at `f310440` after `vite build`:
```
172.1 KB  TOTAL (preloaded)      Budget: 180 KB      ✓ 7.9 KB of headroom
 87.0 KB  assets/charts-*.js     ← lazy, NOT counted
```
So the ~8.1 KB of `@visx` lands in a chunk the gate does not measure, and costs
**nothing** against the 180 KB ceiling. The decision stands and is cheaper than it
was presented; the obligation to raise the gate is withdrawn, not deferred.

**The facts this was decided on, measured 2026-09-02 —**
- **Licence: clean.** A real MIT licence *text* with a copyright line exists at the
  repository root `LICENSE` ("MIT License / Copyright (c) 2026 uixmat"), so unlike
  the coss ui case of D-020 there is something to retain and a `NOTICE` entry can be
  written honestly. Two caveats: `packages/ui/package.json` is `"private": true`
  carrying a `"license": "MIT"` field — the exact coss smell, here backed by real
  root text — and the MIT grant is **not repo-wide**. `LICENSE-STUDIO.md` places
  `packages/studio` under a proprietary all-rights-reserved licence, and
  `packages/studio/src/components/charts/heatmap-studio-preview.tsx` is **not MIT**.
  **Vendoring is pinned to `packages/ui` only.**
- **Upstream: no release.** `@bklitui/ui` is `version: 0.0.0`, `"private": true`. It
  has never been published. There is no version to pin, no changelog and no upgrade
  path — only a SHA and a manual re-diff.
- **Weight.** #444's body describes bklit ui as "a bespoke renderer over `d3-scale` /
  `d3-shape` / `d3-geo`. That is wrong: `packages/ui` depends on **14 `@visx/*`
  packages** (most pinned at `4.0.1-alpha.0`), plus `d3-array`, `d3-geo`, `d3-scale`,
  `d3-shape`, `topojson-client`, `react-use-measure`, `tailwind-merge`,
  `motion ^12.27.0`, `@number-flow/react ^0.5.4` and **`@base-ui/react ^1.0.0-alpha.8`**.
  Gzipped ESM for the cheapest item on the adopt list, the Heatmap:
  `@visx/heatmap` 0.9 KB · `@visx/responsive` 3.0 KB · `@visx/scale` 4.2 KB =
  **~8.1 KB**, before any `d3-*`, before `motion`, before the vendored source. Per
  the correction above this lands in the **lazy** `charts` chunk, which the budget
  gate does not count, so it is a download cost on the routes that show a chart and
  **not** a charge against the 180 KB initial-bundle ceiling.
- **What it displaces: Recharts `^3.5.0`.** This is the D-019 test-3 question that was
  recorded as unanswered when the decision was put. Recharts is already a dependency,
  imported by six files — `features/financial/FinancialDashboard.tsx`,
  `features/risks/components/SmartRiskRadar.tsx`, `features/analytics/ExecutiveDashboard.tsx`,
  `pages/ComplianceReportDashboard.tsx`, `pages/Analytics.tsx`, `pages/AnalyticsDashboard.tsx` —
  and `frontend/src/shared/ds/chart.ts` is written as a token contract to be spread
  onto Recharts primitives. A vendored visx layer either replaces Recharts across
  those six files or ships **beside** it, and shipping both is the outcome to avoid.

**Consequence — what Option B obliges, none of it optional —**
1. **The bundle gate is NOT raised, and the charts stay lazy.** Superseded by the
   correction above. `check-bundle-budget.mjs` states "THIS BUDGET MAY ONLY EVER BE
   LOWERED. Raising it to accommodate a new dependency is the decision that makes it
   meaningless; the dependency goes in a lazy chunk instead, or it does not go in."
   The obligation is therefore the opposite of what was first written: keep every
   vendored chart out of the preloaded graph, and confirm with
   `node scripts/check-bundle-budget.mjs` that the preloaded total has not moved.
2. **`motion` and `@number-flow/react` stay banned.** They are `no-restricted-imports`
   at `error` (`frontend/eslint.config.js`) and D-024 does not lift that. Vendored
   source that imports `motion/react` — `heatmap-chart.tsx` does — must be rewritten
   onto `framer-motion`, already a dependency. Vendor the source; do not install
   these two packages.
3. **`@base-ui/react` arrives transitively at `1.0.0-alpha.8`** — the package D-019
   declined on its own merits. Either it is stripped from the vendored subset or
   D-019 is explicitly amended. It may not enter by omission.
4. **`GaugeChart` is still banned.** It sits in #444's "adopt" table *and* in the
   `no-restricted-imports` deny-list at `frontend/eslint.config.js:271` that #446
   landed. B does not resolve this contradiction: either `GaugeChart` is dropped from
   the subset, or the deny-list entry is removed by a decision that overrides the
   design guide's angle-encoding rule. Until then it is out.
5. **The destination path in #444 is outside the licence boundary.** #444 says vendor
   into `frontend/src/components/ui/charts/`. `frontend/design-system/NOTICE` scopes
   Apache-2.0 to exactly `frontend/design-system/` and `frontend/src/shared/ds/` and
   states everything else is AGPL. Per D-020 the destination must be one of those two,
   so charts land in **`frontend/src/shared/ds/`**, with each vendored component
   recorded in `frontend/design-system/NOTICE` with its upstream SHA.
6. **The Heatmap does not do what #444 wants.** It is asked for as a "5×5 risk matrix,
   inherent vs residual" — a categorical grid. Upstream's `heatmap-chart.tsx` imports
   `scaleTime` and the directory carries `heatmap-week-start`, `heatmap-week-range`
   and `generate-heatmap-skeleton-data`: it is a calendar/contributions heatmap. The
   risk matrix is built, not vendored.

**Reversible** — yes, up to first publication. Nothing is relicensed by this decision;
the vendored files are MIT-in-Apache-2.0 recorded in `NOTICE`, and the subset can be
dropped later. The raised bundle ceiling is the part that is hard to walk back.

**Unblocked** — #444 moves off `status:blocked`. It goes to `status:needs-refinement`,
not `status:ready`: consequences 1, 3, 4 and 5 above each contradict a task currently
written in its body, so its acceptance criteria must be corrected before it is
workable. #439's charts line follows #444.

---

### D-021 — `EmptyState.tsx` is relicensed to Apache-2.0 and moves into `shared/ds/` · 2026-09-02
**Decided** — Option A. `src/shared/EmptyState.tsx` becomes `Apache-2.0` and moves
into `frontend/src/shared/ds/`, exported from its `index.ts` as the design
system's `Empty`. Its 22 importers keep working through the barrel; per
Résolution 1 point 4 of #443 no call site is rewritten in that issue.

**Rationale** — `shared/ds/` is Apache-2.0 (D-014, D-016) and may not depend on
or re-export AGPL code; the dependency runs one way only, and `DangerConfirm`
(AGPL) importing `Button`/`Modal` (Apache) is the correct direction. That left
three options and only A leaves the product with **one** empty state AND a
complete primitive layer. Option B was declined because a generic `Empty` beside
`EmptyState` is a second empty state in practice, which #443 explicitly forbids
and which the codebase has already had to consolidate away once. Option C was
declined because an incomplete primitive layer is the thing #443 exists to fix.

**Irreversible.** Apache-2.0 publication cannot be withdrawn, exactly as in D-014
and D-016. This extends the boundary those two drew rather than opening a new
question: the artefact people copy is the components.

**Issues unblocked** — #443 (the `Empty` item of PR 2, carried unresolved through
the PR 3, 4 and 5 checkpoints). #443 is the last open blocker inside ds-v1's
primitive work.

---

### D-022 — the frontend lint gate is ratcheted at 321, not swept and not downgraded · 2026-09-02
**Decided** — Option A. The `Frontend Lint & Format` job freezes the current count
of **321 `@typescript-eslint/no-explicit-any` errors** as a ceiling and fails only
on an increase; `@vitest/coverage-v8` is added as a devDependency; and the
`type-check`, `format` and `format:check` scripts the CI job already invokes are
added to `frontend/package.json`.

**Rationale** — `master` CI has been red since at least `c1d0f8f`. `eslint .`
reports 337 problems / 321 errors, `vitest --coverage` aborts with
`MISSING DEPENDENCY Cannot find dependency '@vitest/coverage-v8'`, and CI calls
three npm scripts that do not exist. The practical effect is that **no frontend
PR is actually checked** — #472 and #473 both inherit a red gate they did not
cause, and a genuine regression would be indistinguishable from the standing
noise. Option B (sweep all 321 now) is the correct end state but is a repo-wide
mechanical diff across `src/utils/` and `src/features/` that would collide with
the open ds-v1 PRs. Option C (downgrade to `warning`) was declined: ABSOLUTE
RULE 5 is "zero `any`", and a rule nothing enforces is not a rule.

The ratchet is the compromise that restores a working gate this week while
keeping Rule 5 enforceable — new code cannot add an `any`, and the 321 are paid
down opportunistically under #341.

**Reversible.** The ceiling is one number in a config; B remains available at any
time and is the intended destination.

**Issues unblocked** — #341 (`[OR-P0-07] … establish clean-tree frontend CI
gates`), which now has a decided approach. Indirectly #472 and #473, whose red
checks are inherited rather than caused.

---

### D-023 — `Command` and `OtpField` get consumer-migration issues; OTP lands in ds-v1 · 2026-09-02
**Decided** — Option A. Two follow-up issues are opened. The **OTP call-site
migration** (`AuthScreen` ×2, `MFAEnrollmentDialog`) goes into **ds-v1**; the
**`CommandPalette` rewire** is opened and backlogged.

**Rationale** — #443's Résolution 1 point 4 forbids rewriting call sites, so both
primitives shipped ahead of their consumers. These are not equivalent: the three
hand-rolled OTP inputs carry a live user-facing defect — a code pasted as
"123 456" is rejected without reaching the server, and `MFAEnrollmentDialog`
accepts `maxLength={8}` for a six-digit code — which is **fixed in the primitive
but not yet for users**. That earns a milestone. `Command` having no call site is
a tidiness problem, not a defect, and waits.

**Reversible.** Both are ordinary scheduling calls.

**Issues unblocked** — closes the "Next" items left by the #443 PR 5 checkpoint.

---

### D-019 — Base UI is declined; only `@tanstack/react-table` is approved · 2026-09-01
**Decided** — Option B. `@tanstack/react-table@9.2.4` is approved as a pinned
dependency. **`@base-ui-components/react` is declined.** The ~17 primitives
`shared/ds/` still lacks are built in-house, against the WAI-ARIA authoring
practices — not by copying coss ui source.
**Rationale (owner)** — Matches the recommendation. `react-table` is headless,
stable, MIT and 6.3 KB gzipped, and it solves sort/filter/group coordination that
is expensive to build well. Base UI is roughly twenty times the weight for the
families #443 lists, has never shipped a stable release, and would replace the
internals of the eight primitives whose 34-test keyboard and focus contract is
the most valuable thing in `shared/ds/`.
**The facts this was decided on, measured 2026-09-01 from the published tarballs —**
- `@tanstack/react-table` — `latest` = `9.2.4` (stable), MIT, **6.3 KB** ESM gzipped.
- `@base-ui-components/react` — `latest` = **`1.0.0-rc.0`**, MIT. Version history
  runs `1.0.0-beta.4 … beta.7 → 1.0.0-rc.0`: **there has never been a stable
  release.** Per-component ESM gzip for exactly the families #443 names:
  button 0.8 · input 0.7 · field 7.4 · fieldset 1.2 · select 17.5 · checkbox 4.2 ·
  checkbox-group 2.5 · radio-group 2.3 · switch 2.9 · dialog 7.2 ·
  alert-dialog 1.3 · menu 18.0 · popover 11.9 · tooltip 10.1 · toast 12.9
  = **100.9 KB**, plus ~20.5 KB of shared internals.
- The console's initial budget is **248 KB with 245.8 KB used — 2.2 KB of
  headroom** (D-018), and it is already 66 KB above the guide's 180 KB target.
**Consequence** — **#443 is superseded as written.** Its title, its build-order
mapping and its ~50-component vendoring plan all assume Base UI, and none of that
survives this decision. It is re-scoped to "extend `frontend/src/shared/ds/`
in-house, and adopt `@tanstack/react-table` for the Table primitive", and moves to
`status:needs-refinement` — not `status:ready`, because the tasks no longer match
the decision. #439's DoD needs the same correction. #444 (bklit ui) is a sibling
vendoring issue and is **not** decided here, but the same three tests apply to it:
stability, weight against the budget, and what it would displace.
`@tanstack/react-table` is **not** added to `package.json` by this decision — it is
added, pinned, by the PR that first uses it, so no dependency lands without a consumer.
**Unblocked** — #443 blocker 3 · #439's dependency question.

### D-020 — a prose MIT grant is not enough to vendor; the notice must exist first · 2026-09-01
**Decided** — Option A. No third-party file is vendored into
`frontend/src/shared/ds/` until the MIT copyright line and permission notice are
sourced and recorded, per component, in `frontend/design-system/NOTICE`.
**Rationale (owner)** — Matches the recommendation. MIT requires the notice to be
retained in redistributions; at the candidate SHA there is nothing to retain, and
Apache-2.0 publication of a defective third-party notice in a public repository
cannot be withdrawn.
**The facts, verified 2026-09-01 at `cosscom/coss` SHA
`758e6535ae0143dce8c85f12e33eebf60b6b2ecb` (default branch `main`, 2026-08-31) —**
- Supporting the MIT claim: root `README.md` states *"**MIT**: The `apps/origin/`
  and `apps/ui/` directories are licensed under their original MIT license"*;
  `LICENSING.md` lists both directories under an `## MIT` heading;
  `apps/ui/package.json` carries `"license": "MIT"`.
- Against it: the repository root `LICENSE` is **AGPL-3.0** and GitHub reports the
  repo as AGPL-3.0. **No MIT licence text exists anywhere at that SHA** —
  `apps/ui/` contains no `LICENSE` file, and `LICENSING.md` asserts the grant
  without reproducing the notice, its sentence breaking off at *"licensed under
  their original"*. `apps/ui/package.json` is `"private": true`, so its `license`
  field describes a package that is never published.
**Consequence** — Standing policy, not a one-off: it binds #444 and any future
vendoring, not just #443. Under D-019 = B nothing is vendored from coss ui today,
so this is a constraint held in reserve rather than an active blocker. If vendoring
is ever revisited, the notice most likely has to come from Origin UI upstream,
which `apps/ui/` derives from.
**Unblocked** — #443 blocker 4 (by removing the need for it).

### D-018 — 248 KB is the standing bundle budget; 180 KB becomes its own issue · 2026-09-01
**Decided** — Option A. The ratcheted **248 KB** gate is the standing budget and
stays blocking. The guide's 180 KB target is pursued in a dedicated performance
issue, scheduled on its own merits.
**Rationale (owner)** — Matches the recommendation. Setting the gate to a number
the console is 66 KB away from would redden every PR, which is how a budget gets
deleted rather than met; putting the 66 KB where the work actually is keeps the
gate honest and green.
**The measurement, 2026-09-01** — initial (preloaded) JS, gzipped: vendor 79.8 ·
react 59.1 · index 38.8 · motion 24.6 · query 17.5 · icons 13.1 · router 12.8 =
**245.8 KB**.
**Consequence** — `frontend/scripts/check-bundle-budget.mjs` keeps
`BUNDLE_BUDGET_KB = 248`, blocking in CI, and the budget may only ever be lowered.
D-019 = B materially helps here: declining Base UI keeps ~121 KB out of the tree,
which is most of the 66 KB gap. The FR/EN Playwright snapshot work noted alongside
this decision also becomes its own issue — the visual harness has no language axis
today, and French running ~20% longer than English is a layout risk worth its own
review.
**Reversible** — Yes; the budget is one constant.
**Unblocked** — #446 closed out with no open remainder.

### D-017 — `react-leaflet` is removed and a real licence gate replaces the fake one · 2026-09-01
**Decided** — Option A, both halves. `react-leaflet` is removed from
`frontend/package.json`, along with `leaflet` and the orphan
`import 'leaflet/dist/leaflet.css'` in `src/main.tsx`. Separately,
`.github/workflows/security.yml`'s `license-check` job is replaced with a gate
that actually fails the build on a non-allowlisted licence, in **both** the Go
and npm trees.
**Rationale (owner)** — Matches the recommendation. The removal turned out to be
free, and the gate is the half with lasting value: this dependency entered
unnoticed, and without a gate the next one will too.
**How it was found** — While executing #452's task "check whether any repo-wide
licence tooling asserts everything under `frontend/src/**` is AGPL". The answer
was that no such tooling exists, and the job that looks like it does is not doing
it.
**The facts, verified 2026-09-01 —**
- `frontend/package.json:43` declared `react-leaflet ^5.0.0`.
  `require('react-leaflet/package.json').license` → `"Hippocratic-2.1"`, as does
  its own `@react-leaflet/core`. Hippocratic-2.1 imposes ethical-use restrictions
  on the licensee; it is **not OSI-approved and not a free-software licence**, so
  it is not compatible with redistributing a combined work under `AGPL-3.0-only`,
  and it sits worse with an EE build under a commercial agreement.
- **Nothing imports it.** Zero hits for `react-leaflet`, `MapContainer`,
  `TileLayer` or `useMap` in `frontend/src/`. The only leaflet trace is
  `src/main.tsx:11`, a CSS import belonging to `leaflet` itself
  (`BSD-2-Clause`, unproblematic but equally unused). No map feature appears in
  `ROADMAP.md` or in any open issue. It is declared, not bundled — the exposure
  is the dependency manifest and every SBOM built from it, not shipped code.
  **This corrects the entry as first raised**, which framed the choice as
  replacing a shipped map binding; there was nothing to replace.
- Full frontend licence census, same date: 356 MIT · 27 ISC · 21 Apache-2.0 ·
  9 BSD-2-Clause · 6 BSD-3-Clause · 4 MPL-2.0 · 4 Unlicense · 2 MIT-0 ·
  2 Hippocratic-2.1 (both the react-leaflet family) · 1 Python-2.0 · 1 CC-BY-4.0.
  The two packages reporting `UNKNOWN` (`fast-shallow-equal`,
  `react-universal-interface`, both transitive via `react-use`) carry the
  Unlicense public-domain text and are merely missing the SPDX field. **Nothing
  else in the tree is non-permissive.**
- `.github/workflows/security.yml` `license-check` writes `backend/modules.json`,
  never reads it, and `echo`s `"✅ License check completed"`. It covers Go modules
  only and asserts nothing; the frontend tree was never gated at all.
**Consequence** — Reversible. Re-adding a map binding is trivial if a map feature
ever lands, and an allowlist can be loosened. The gate must fail the build rather
than warn, or it reproduces the defect it replaces.
**Not touched by #452 / PR #456**, which relicenses the design system and changes
no dependency.
**Unblocked** — #457 opened to execute both halves.

### D-016 — the Apache-2.0 boundary is both design-system directories · 2026-08-31
**Decided** — Option A. "The design system" in D-014 means **`frontend/design-system/`
and `frontend/src/shared/ds/`**, both. Every file in both takes an `Apache-2.0`
SPDX header. The AGPL-3.0-only core and the `LicenseRef-OpenRisk-Commercial` EE
boundary are unchanged.
**Rationale (owner)** — Same reason as D-014: **the artifact people copy is the
components, not the token file.** Relicensing only `frontend/design-system/`
would leave the exact auditor argument D-014 was resolved to remove sitting one
directory over — `frontend/src/features/reports/BoardReportPage.tsx`
(`LicenseRef-OpenRisk-Commercial`) imports `shared/ds`
(`import { Button, cn } from '../../shared/ds'`), not the tokens. And #443 is
about to add ~50 vendored components to `frontend/src/shared/ds/`: executed now
they are born Apache-2.0; executed later that is ~50 fresh headers to rewrite
plus the 14 existing ones.
**Consequence** — Irrevocable for anything published, as D-014. Scope is 16 files
across the two directories. `LICENSING.md` gains **one row per directory**, not
one row. The boundary is precise and does not widen by adjacency:
`frontend/src/shared/ui.tsx` (61 importers) and
`frontend/src/shared/EmptyState.tsx` (22 importers) stay AGPL-3.0-only — they are
product code that happens to live nearby. `frontend/src/shared/ds/` carries no
`LICENSE`/`NOTICE` of its own; the ones in `frontend/design-system/` name it
explicitly and cover it.
**Unblocked** — #452 executes D-014 and D-016 together in one PR, before any
vendoring. It gates #443 and #444. #443 stays `status:blocked` on its three
remaining non-licensing blockers (#446, two undeclared dependencies, upstream MIT
verification at the vendored commit) — this decision closes the licensing line of
its blocking table only.
**Record note** — This entry was transcribed into the register on 2026-09-01 while
executing #452, from the issue body and `po-openrisk`'s timestamped comment of
2026-08-31. The decision is the owner's and dates from 2026-08-31; only the
register entry is late. `LICENSING.md` and `frontend/design-system/README.md`
cite it. **Transcription confirmed faithful by the owner on 2026-09-01** — the
entry is ratified, not assumed, and is not to be relitigated on the grounds that
it was written down late.

### D-015 — CLA acceptance is enforced by a DCO check · 2026-08-31
**Decided** — Option A. A GitHub Action asserts the `Signed-off-by` trailer on
every commit. No third-party service, revisit at the first corporate contribution.
**Rationale (owner)** — Matches the recommendation. The trailer is already a real,
timestamped, per-commit record in git history, so the gap was enforcement rather
than evidence, and enforcement costs one workflow file. `cla-assistant` is a new
external service holding contributor personal data — the stronger artifact, but
not worth the data-processing question before a single corporate contribution has
arrived.
**Consequence** — A DCO workflow is added under `.github/workflows/` and made a
required check. `CONTRIBUTING.md` must state that `git commit -s` is mandatory.
Switching to `cla-assistant` later is a workflow change; commits merged under DCO
keep their trailer as their record, so nothing has to be re-signed.
**Unblocked** — executed by #455 (#453, its duplicate, was closed). Enforcement is not live
until the owner flips the required-check setting by hand; #455 says so in its DoD.

### D-014 — `design-system/` is relicensed to Apache-2.0 · 2026-08-31
**Decided** — Option A. `design-system/` becomes `Apache-2.0` and gains a
`design-system/NOTICE`. The AGPL-3.0-only core and the
`LicenseRef-OpenRisk-Commercial` EE boundary are unchanged; only the design
system moves.
**Rationale (owner)** — Matches the recommendation. The design system is the
artifact the project *wants* copied and extended, and its value is protected by
trademark rather than by copyleft, so copyleft on it bought nothing and cost an
audit conversation on every enterprise deal. Option C — moving the two EE
consumers off the design system — was the worst available, because it forces the
EE surfaces to reinvent primitives, which is the exact fragmentation epic #439
exists to end.
**Consequence** — Irrevocable for anything published under it; future versions
can be relicensed but what ships cannot be clawed back. `LICENSING.md` gains a
`design-system/` row, every file under it takes an `Apache-2.0` SPDX header, and
`design-system/NOTICE` is created and thereafter records each vendored
third-party component with its upstream commit. `frontend/src/features/ai/` and
`frontend/src/features/reports/BoardReportPage.tsx` may import it with no
ownership argument required. Note the register's own path ambiguity: the vendored
design system lives at `frontend/design-system/` and no top-level
`design-system/` exists — the relicensing applies to the directory that exists.
**Unblocked** — #452 opened to execute the relicensing (`status:ready`, milestone `ds-v1`),
and it gates #443 and #444. Both of those remain `status:blocked` — D-014 was one of four
blockers on #443, not the only one; #446 must still land first, two dependencies are still
undeclared, and two spec gaps are still open. See the readiness report on #443.

### D-013 — Residual score is additive, in the SmartScore mould · 2026-08-31
**Decided** — Option A. A new `pkg/scoring` function computes residual from
`risk_control_mappings`. `Risk.Score` is untouched and stays frozen. One ADR is
written before PR 2 of #438 opens.
**Rationale (owner)** — Matches the recommendation. Option B answers a different
question — portfolio money, not this risk's posture — and putting a CFO figure
into a first-run teaching step is the wrong number in the wrong place. Option C
removes the one moment in the funnel where the product demonstrates something a
spreadsheet cannot, which is the reason step A5 exists.
**Consequence** — The ADR is the long pole, not the code, and it must land before
PR 2 opens. Reversible only while nothing persists to `Risk.ResidualRisk`; once
tenant rows carry computed residuals, changing the formula silently restates
their history, so the ADR fixes the formula before the first write.
**Unblocked** — #438, step A5 of PR 2.

### D-012 — `origin: "starter"` is a new `RiskSource` enum value · 2026-08-31
**Decided** — Option A. `SourceStarter RiskSource = "starter"` is added to the
enum and to `ParseRiskSource`.
**Rationale (owner)** — Matches the recommendation. `Risk.Source` is already the
field that answers "where did this risk come from", the column is `varchar(20)`
so no migration is needed, and the value inherits the existing validation. B adds
a second column answering the same question; C hides provenance in a
`CustomFields` blob that nothing indexes or validates, which PR 4 would then have
to grep to prove the starter rows are real.
**Consequence** — Reversible: an unused enum value is inert. PR 4 proves the
starter rows by querying `source = 'starter'`.
**Unblocked** — #438, PR 2.

### D-011 — `posture.revealed` is a non-catalogue key with its own assertion · 2026-08-31
**Decided** — Option A. `EventKeyPostureRevealed` is added,
`ValidateActivationSteps()` is left untouched, and a sibling
`ValidateNonChecklistEventKeys()` asserts that `posture.revealed` and
`aha.reached` are absent from `activationSteps`.
**Rationale (owner)** — Matches the recommendation. The mechanism the W1-05 brief
asked for already exists — `ActivationAhaReached` is a non-catalogue key today
and the validator iterates `activationSteps` only, so no amendment was ever
needed. What was missing is the assertion that a non-catalogue key never leaks
into the catalogue, which is exactly where a future edit would break it. Option C
would put a server-recorded outcome into a list of user chores and is refused by
the brief's own invariant 2.
**Consequence** — Reversible; an unused event key costs nothing to drop before
any row is written.
**Unblocked** — #438, PR 1.

### D-010 — Aha switches definition; `time_to_aha_seconds` gains an `aha_definition` label · 2026-08-31
**Decided** — Option A. `monitoring.TimeToAha` becomes a
`HistogramVec{aha_definition}`: v1 freezes, v2 starts empty, and
`SlowTimeToAha` in `deployment/monitoring/alerts.yml` selects
`aha_definition="v2"`.
**Rationale (owner)** — Matches the recommendation. Option B splices two
incomparable definitions into one P50 and makes the alert lie the moment the
switch lands; option C leaves the Posture Reveal decorative, which is the thing
the brief exists to fix. The label is what makes a wrong definition survivable.
**Consequence** — **Irreversible once v2 observations are written**: the v1
series cannot be reconstructed from them. Record for whoever implements this: the
real switch is executive-dashboard score computation → posture-summary
computation. The W1-05 brief's framing of it as "`first_risk` → `posture.revealed`"
is **wrong** and must not be copied forward — `aha.go:44` tests
`ScoreComputed && OwnDataPoints > 0 && ComplianceGaps > 0`, and `first_risk` is a
checklist step that has never defined the Aha. Carry forward one existing defect
that A must not inherit: `AhaReachedTotal.Inc()` sits *inside*
`ObserveTimeToAha` (`pkg/monitoring/activation.go:81`), so a tenant with no
signup anchor reaches Aha without being counted — `openrisk_reveal_reached_rate`
must not be derived from that counter.
**Unblocked** — #438, PR 1.

### D-009 — W1-05 base branch: resolved by events, #234 is merged · 2026-08-31
**Decided** — Option A, and it has already happened without the owner having to
act. The question was whether to merge #234 before cutting the W1-05 branches;
#234 merged as PR #437 on 2026-08-31 at 10:35Z.
**Rationale (owner)** — Not a judgement call any more. Verified rather than
assumed: `origin/234-w1-04-…` is **0 ahead of `master`** and 9 behind, and every
artefact the entry named is on `master` —
`backend/internal/infrastructure/repository/gorm_activation_repository.go`,
`tests/e2e/activation.spec.ts`, and `BackfillExistingMembers` in
`gorm_activation_backfill.go`, `membership/service.go` and `cmd/server/main.go`.
**Consequence** — All four W1-05 PRs are cut from `master`, not stacked. The
drift the entry warned about did not accumulate. Nothing to stack, nothing to
rebase.
**Unblocked** — #438, all four PRs.

### D-008 — One Time to First Value number: 8 minutes · 2026-08-31
**Decided** — Option A. The committed public promise is **8 minutes** from
signup to the Aha moment.
**Rationale (owner)** — Matches the recommendation. Eight is the only number an
automated test asserts, so it is the only one defensible under RULE #12. Five
minutes through a five-step wizard, a framework import and a first risk is not
provable today; publishing it would be the exact failure the claim matrix exists
to prevent, on a launch-gate claim.
**Consequence** — `AHA_BUDGET_MS` in `tests/e2e/activation.spec.ts` stays at
8 minutes and its assertion is the proof. `docs/MARKETING_CLAIM_MATRIX.md` C-002
carries the promise and names the test; the same file states explicitly that the
12-minute `SlowTimeToAha` threshold in `deployment/monitoring/alerts.yml` is an
operational warning with deliberate headroom and **not** the promise.
`ROADMAP.md` 17.6 no longer says 5. Changing the number later means changing all
three in one commit.
**Unblocked** — #234 needed no label change: it was never blocked on this, and
shipped on 8 in PR #437. The decision confirms what is already in review.

### D-007 — #411 criterion 14: universal drawer keeps an "Open full view" action · 2026-08-31
**Decided** — Option B. Every register row opens the universal drawer, and the
drawer carries a prominent action that opens the rich view where one exists.
**Rationale (owner)** — Matches the recommendation. Navigation becomes uniform
without deleting capability: the 9-tab risk surface (details · lifecycle · score
· smart · financial · miti · ai · timeline · cti, inline in
`features/risks/RiskRegisterPage.tsx`), the Evidence approve/reject editor and
the Infrastructure scanner-config editor all stay reachable, one click deeper.
Option D was rejected because it ships a visible capability regression on the
product's most important screen; option A because the issue's own PO note
forbids mixed navigation, and forbids it for a good reason.
**Consequence** — The universal drawer gains an "open full view" affordance, a
design change beyond #411 as originally written: `art-director` and
`ux-designer` should confirm the pattern and its copy before it is built. The
per-register mapping in criterion 14 still needs the per-feature check for
Incidents, Compliance and Vulnerabilities; Assets and Inventory remain pure gain.
**Unblocked** — nothing yet, and this needs saying: **#411 is already CLOSED**
(a merged PR closed it while criterion 14 was unfinished) and still carries a
stale `status:in-progress`. Option B therefore has no home. It needs either a
reopen of #411 or a new issue carrying criterion 14 plus this decision; see the
comment posted on #411.

### D-001 — Brand: OpenRisk everywhere · 2026-08-28
**Decided** — Option A. OpenRisk is both the company and the product name.
Karath is not adopted.
**Rationale (owner)** — The descriptive name's organic search equity is worth
more than a defensible mark at this stage. Recommendation was C (Karath company
/ OpenRisk product); the owner chose A.
**Consequence** — This is the status quo, so no rework: every existing package
identifier, domain and page already says OpenRisk. The trademark position is
knowingly weak — "OpenRisk" is descriptive, so a competitor may use the phrase.
Revisit only if a mark becomes commercially necessary, and note this decision is
effectively irreversible once the site is indexed.
**Unblocked** — every `area:marketing` and `area:docs` issue.

### D-002 — Cameroonian framework `cm-loi-2024-017`: dropped · 2026-08-28
**Decided** — Option C. The claim is removed entirely rather than shipped as a
placeholder or marked `PLANNED`.
**Rationale (owner)** — Recommendation was A-or-B; the owner chose C. A
regulatory claim with no resolvable citation to official source text is exactly
the liability the compliance doctrine exists to prevent, and carrying it as
`PLANNED` still advertises coverage the product does not have.
**Consequence** — `cm-loi-2024-017` comes out of the framework catalogue and out
of any marketing or docs that reference it. The African catalogue's differentiator
narrows to the frameworks that are actually sourced. If the official text is
obtained later, it returns as a new issue with a full citation.
**Unblocked** — the framework catalogue can ship without a placeholder row.

### D-003 — Label taxonomies: keep both · 2026-08-28
**Decided** — Option C. Both label families stay; every new issue carries both.
**Rationale (owner)** — Recommendation was A (declare the wave family canonical);
the owner chose C. Neither family is retired and no backlog migration happens.
**Consequence** — **Every new issue must be dual-labelled**, as #409-#412 already
are: one `area:` and `priority:` from the wave family, one from the Constitution
family, plus a `status:`. Agents opening issues must apply both. CLAUDE.md's
label table stays as written and is not amended. The cost is a permanently
doubled label set per issue; that is accepted deliberately.
**Unblocked** — `issue-triage` may proceed; dual-labelling is now the rule, not a
transitional state.

### D-004 — Audit trail: observable best-effort now, guaranteed later · 2026-08-28
**Decided** — Option A now, Option B before any regulated-customer commitment.
**Rationale (owner)** — Matches the recommendation. Failing a user's successful
mutation in order to record it (option C) is worse than the gap; but silent
incompleteness is the wrong default for the ISO 27001 A.8.15 / A.5.28 artefact
sold to COBAC- and BCEAO-supervised institutions.
**Consequence** — `middleware.AuditMutations` keeps `_ = appender.Append(...)`,
and a dropped write becomes observable through an error log and a counter. That
work is #410 criteria 40-41. The outbox/queue (option B) is **not** in any child
issue today and must be opened and scheduled before a regulated customer is
signed — it is a commitment, not an aspiration.
**Unblocked** — #410 criteria 40-41 · #412 criterion 22.

### D-005 — ADR-0001 accepted: panic at boot, raw sequential ids · 2026-08-28
**Decided** — Accept the ADR. D2-A (panic at boot on an incomplete registration)
and D4-A (keep raw sequential incident ids, mitigate enumeration rather than
hide it).
**Rationale (owner)** — Matches the recommendation. A registry that boots with an
ungated type is the exact defect W1-02 exists to prevent, and opaque ids are a
migration this milestone cannot absorb.
**Consequence** — `docs/adr/0001-polymorphic-entity-contract-and-registry.md`
moves to `Status: accepted`. D2-A was already implemented and verified in
`9fb2305`, so the code and the ADR now agree. D4-A is **irreversible once deep
links are in the wild**: incident URLs will carry sequential integers
permanently, and the enumeration mitigation (tenant filtering on every read,
identical 404s for forged and foreign ids) is what makes that safe. It is
declared in the registry as `EnumerationMitigation` and enforced by
`TestRegistry_SequentialIDRequiresAnEnumerationMitigation`.
**Unblocked** — #410 criteria 1-2 · the #200 umbrella DoD.

### D-006 — Plan matrix reverted out of the chore commit · 2026-08-28
**Decided** — Option B. The entitlements rewrite is reverted off
`feat/w1-02-universal-entity-drawer` and re-proposed as its own issue.
**Rationale (owner)** — Matches the recommendation. The new matrix may well be
right, but it changes what the product gives away, and nobody reviewing
"agent company v3" was reviewing pricing.
**Consequence** — `backend/pkg/entitlements/entitlements.go` and its test return
to their `master` state on this branch, which turns `TestAllowed_FeatureGate`
green again. The volume-throttled model is re-proposed on a dedicated issue so
it is reviewed as a pricing change, with the stale
`internal/application/entitlements/service_test.go` updated as part of THAT work.
**Unblocked** — `feat/w1-02-universal-entity-drawer` stops carrying a red test
unrelated to its own scope.
