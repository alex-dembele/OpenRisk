# 0003 — Residual risk is derived from control coverage, additively

Status: **accepted**
Proposed: 2026-09-10, on #438 (W1-05, PR 2)
Accepted: 2026-09-10 by the owner — D-042 (`docs/DECISIONS.md`)
Decided by: D-013 (`docs/DECISIONS.md`), Option A — approach; D-042 — constants
Implemented by: #438

## Context

#438 replaces the post-wizard activation cliff with a guided tunnel whose last
step shows the user something a spreadsheet cannot produce: *accept this control,
and watch your exposure fall*. That requires a residual number, and D-013 decided
it comes from `risk_control_mappings` rather than from a portfolio money figure.

D-013 also fixed the shape of the answer: **additive, in the SmartScore mould.**
`Risk.Score` from `pkg/scoring/engine.go` is frozen and is not in the output path
of anything below.

Three things were read rather than assumed, and two of them change the design the
brief implied.

**1. A residual already exists, and it is not the same one.**
`internal/domain/scoring` (`compute.go`, `model.go`) already returns `Residual`,
`ResidualBand` and `MitigationEffectiveness` on every `Result`. But it does not
compute effectiveness — it *receives* it:

```go
// internal/domain/scoring/compute.go
type RiskInput struct {
    ...
    // MitigationEffectiveness ∈ [0,1] drives the residual score.
    MitigationEffectiveness float64
}
```

Nothing in the codebase fills that field from control mappings. So the gap is not
"there is no residual", it is **"nothing knows what effectiveness the tenant's
controls have earned"**. Writing a second, competing residual number would give
the product two answers to one question, which for a GRC tool is worse than
having none.

**2. There are two scales, and they must not be mixed.**
`internal/domain/scoring` works on a canonical 0–100 scale where 100 is worst.
`Risk.Score` and `Risk.ResidualRisk` (`internal/domain/risk.go:267`,
`numeric(8,3)`) are on the Score Engine's own scale: `P × I × AC`, so 0–30. The
tunnel's illustration in #438 — "16 down to 6" — is on the Score Engine scale;
16 is not expressible on a 0–10 scale and is not a plausible 0–100 posture.

**3. `not_applicable` exists and is not a failure.**
`domain.ControlStatus` has four values (`internal/domain/compliance.go:19-22`):
`not_implemented`, `in_progress`, `implemented`, `not_applicable`. Scoring a
scoped-out control as an uncovered one would punish a tenant for correctly
scoping its framework, which is the opposite of what a GRC product should teach.

## Decision

**A new pure function in `pkg/scoring` computes an effectiveness from control
coverage, and the existing residual machinery consumes it.** We add an input to a
model that already has a residual; we do not add a second residual.

```go
// pkg/scoring/residual.go — stdlib only, deterministic, no GORM, no Fiber.
func CoverageEffectiveness(credits []ControlCredit) Coverage
func ResidualFromInherent(inherent float64, c Coverage) Residual
```

### The formula

Per mapped control, a credit in `[0,1]`:

| Control status | Evidence attached | Credit | Why |
|---|---|---|---|
| `implemented` | yes | **1.00** | Implemented and provable |
| `implemented` | no | **0.70** | Claimed, not evidenced — an auditor discounts it, so do we |
| `in_progress` | — | **0.30** | Work started reduces exposure; it does not remove it |
| `not_implemented` | — | **0.00** | — |
| `not_applicable` | — | *excluded* | Removed from numerator **and** denominator |

```
coverage      = Σ credit / count(applicable controls)      ∈ [0,1]
effectiveness = MaxEffectiveness × coverage                ∈ [0, 0.80]
residual      = round(inherent × (1 − effectiveness), 3)
```

`MaxEffectiveness = 0.80` is a **floor on the residual, not a cap on ambition**: a
fully covered risk keeps 20% of its inherent exposure. A product that lets a
control set drive a risk to zero teaches its users that paperwork eliminates
risk, and that is a lie a GRC tool must not tell. Twenty per cent is the constant
this ADR asks you to accept or move.

`residual` is on the **Score Engine's scale** and is banded by the Score Engine's
own thresholds (`critical ≥ 7.0 · high ≥ 4.0 · medium ≥ 2.0 · low < 2.0`), so a
residual and a score are always comparable and never need a conversion.

With no applicable control mapped, `coverage` is undefined: `Coverage.Measured`
is `false`, effectiveness is `0`, and **`residual == inherent`**. An absent
signal must never read as a good one — the same rule `combine()` already applies
by renormalising unavailable factors.

### What stays frozen

* `Risk.Score` and `pkg/scoring/engine.go` — untouched, not read by, not written
  by anything here.
* `SmartScore` and `pkg/scoring/smart.go` — untouched. Residual is a third,
  independent additive view, exactly as SmartScore is a second.
* `internal/domain/scoring`'s formula — untouched. It gains a *caller* that fills
  `MitigationEffectiveness` honestly; its arithmetic does not change.

### Versioning

`ResidualFormulaVersion = "residual-v1"` is stamped on every computed result and
persisted alongside any stored value. A later formula gets `residual-v2`; a
stored `residual-v1` figure is never silently reinterpreted under v2 rules.

## Consequences

* **Irreversible from the first write.** D-013 made this reversible only while
  nothing persisted; D-042 accepted the constants on 2026-09-10, so
  `Risk.ResidualRisk` may now be written — and from the first stored row,
  changing `MaxEffectiveness` or any credit weight silently restates tenant
  history. A later formula takes a NEW version: `ResidualFormulaVersion` is
  stamped on every result and must be persisted alongside any stored value, so a
  `residual-v1` figure is never reinterpreted under `residual-v2` rules.
* Evidence becomes load-bearing. `implemented` without evidence earning 0.70 will
  visibly cost tenants points, which is the intended teaching and will also be
  the first thing someone asks about. It is worth saying plainly in the UI copy
  rather than letting them discover it.
* The function is pure and takes already-fetched credits, so tenant isolation is
  the caller's duty, not the formula's. `risk_control_mappings` carries
  `tenant_id` (`internal/domain/risk_taxonomy.go:161`) and is already behind a
  tenant-scoped repository port; every call site must filter on it, and #438's
  acceptance criterion 10 is what proves it.

## Alternatives rejected

**A second residual number of its own, on the 0–100 scale.** Rejected: two
residuals for one risk, differing by scale, is a support ticket generator and an
audit finding. The existing `Result.Residual` is the place this belongs.

**Effectiveness from mitigation plans instead of controls.** Rejected for this
issue: D-013 chose controls, and the tunnel's step 5 is explicitly "accept the
proposed control". Mitigation-driven effectiveness is a legitimate second source
and can be combined later — the function signature takes a slice of credits
precisely so a second source can contribute credits without a formula change.

**Binary coverage (any implemented control ⇒ fully mitigated).** Rejected: it
makes the reveal's number a step function, which looks broken the moment a tenant
adds their second control and the number does not move.

**Weighting credits by control importance.** Rejected for v1: nothing in the
catalogue currently carries an importance or a weight, so any weighting would be
invented rather than sourced — RULE #12. If the framework catalogues later carry
one, that is `residual-v2`.
