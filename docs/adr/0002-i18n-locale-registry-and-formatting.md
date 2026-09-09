# 0002 — Locale registry, CLDR pluralization and centralized formatting

Status: proposed
Proposed: 2026-09-08, on #315 (W1-06)
Implemented by: #315

## Context

OpenRisk targets France, Belgium, the Maghreb, Canada, the USA and Sub-Saharan
Africa. Before this change the frontend could express exactly two languages, and
it expressed them by writing `'fr' | 'en'` into 38 type annotations across 25
files. Measured on `master` at 2026-09-08:

| Symptom | Count |
|---|---|
| Inline `lang === 'fr' ? … : …` in components | 287 across 117 of 292 `.tsx` files |
| `'fr' \| 'en'` unions outside any registry | 38 in 25 files |
| Ad-hoc pluralization written as `n > 1 ? 's' : ''` | 17 sites |
| `toLocale*` call sites | 77 |
| …of which pinned a tag (`'fr-FR'`, `'en-US'`, `'en-GB'`) regardless of the user's language | 10 |
| Central formatting module | none |
| `Intl.PluralRules` / `Intl.RelativeTimeFormat` / `Intl.ListFormat` usage | none |
| Writing-direction concept (`<html dir>`) | none |
| Competing dictionaries | 2 (`locales/*.json`, 348 keys; `shared/uiStrings.ts`, flat) |

Two of these were not merely inelegant, they were wrong:

* `` `${n} result${n > 1 ? 's' : ''}` `` printed **"0 result"** in English.
  English takes the plural at zero; French does not. One rule cannot serve both,
  and no rule of that shape can serve Arabic, which has six plural categories.
* Ten call sites formatted in French for English users, and English date order
  was inconsistent between `en-GB` and `en-US` from screen to screen.

## Decision

### D1 — One registry owns the set of languages

`src/i18n/locales.ts` declares every locale: BCP-47 tag, writing direction,
endonym, default currency and an `enabled` flag. `LocaleCode` is derived from
that object with `keyof typeof LOCALES`, and `store/uiStore.ts` re-exports it as
`Lang`. Adding a language therefore *widens a type*, which makes every place
that still assumes two languages a compile error rather than a silent fallback.

That is exactly what happened when Arabic was registered: 129 type errors in 37
files, all of them latent bugs. They are fixed in #315.

### D2 — `enabled` separates *registered* from *offered*

Arabic is registered (`ar-MA`, RTL, six plural categories) and **disabled**. Its
direction and plural rules are exercised by the test suite; it has no catalogue,
so `resolveLocale()` refuses to make it active however it arrives — a stale
`localStorage` value, a deep link, or a switcher from an older build.

This is what lets RTL be *proved* rather than asserted, without claiming the
product ships Arabic. Enabling a language is one line, once its catalogue exists.

### D3 — Pluralization is `Intl.PluralRules`, never a `> 1` test

A catalogue value is either a string or a `PluralForms` object whose `other` key
is **required by the type** — `other` is the one CLDR category every language
defines, so a translator cannot physically ship a plural with no fallback.
`exact` overrides a literal count where the copy reads better ("Aucun résultat"
rather than "0 résultat").

### D4 — One formatting module, always bound to the active locale

`src/i18n/format.ts` is the only place the product turns a number, a date or an
amount into text: number, compact, percent, currency, date, date-time, time,
relative time, list. Every formatter takes a `LocaleCode` and resolves its tag
through the registry. `Intl` instances are memoized because these run in table
cells.

Money is left to `Intl` rather than special-cased: it already knows XAF has no
minor unit and prints "1 234 567 FCFA" in French, "FCFA 1,234,567" in English.

### D5 — Missing keys fall back, and are visible

`translate()` resolves active locale → default locale (`fr`) → `defaultValue` →
the key text. An empty string in a catalogue is treated as a hole, not a
translation. Nothing ever renders blank, and a gap reads as a dotted key in
review.

### D6 — The debt is ratcheted, not declared finished

#315 delivers the framework and migrates the shared layer. 260 inline bilingual
ternaries remain. `src/i18n/__tests__/ratchet.test.ts` freezes the counts and
fails when they rise, so the tree stops accumulating what the framework replaces
while the sweep is done screen by screen.

## Consequences

* Adding a language is: one `LOCALES` entry, one catalogue in `catalog.ts`,
  `enabled: true`. No component changes.
* **English dates are now consistent.** Fifteen sites previously formatted
  English as `en-GB`; the registry maps `en` to `en-US`, so bare dates move from
  `DD/MM/YYYY` to `MM/DD/YYYY` for English users. Logged for the owner as D-037.
* First-visit language stays French. `negotiateLocale()` exists and is tested but
  is deliberately **not** wired to `navigator.languages` — see D-038.
* Three `'fr' | 'en'` unions survive on purpose: `features/ai/Locale`,
  `types/report/ReportLocale`, `types/board/BoardLocale`. They describe what the
  **server** can render, not what the UI can display. Widening them would claim
  OpenRisk generates Arabic reports. They change when the backend gains a
  language.
* The backend still negotiates language with
  `strings.HasPrefix(c.Get("Accept-Language"), "en")` in at least four handlers.
  Out of scope here; tracked separately.
* `shared/uiStrings.ts` is untouched. It has its own accessor and no plural or
  format needs; folding it into the catalogue is a mechanical follow-up, not a
  prerequisite.

## Alternatives rejected

* **react-i18next / FormatJS.** Both solve this well and both are a new runtime
  dependency plus a bundle cost, which CLAUDE.md puts on the escalation list. The
  parts actually needed — plural rules, number/date/list/relative formatting —
  are `Intl`, already in every target browser and in Node. The framework here is
  seven files and no dependency. If the catalogue ever needs full ICU MessageFormat
  (select, selectordinal, nested arguments), that is the moment to escalate for
  FormatJS rather than grow the interpolator.
* **Widening `ReportLocale` / `BoardLocale` to `LocaleCode`.** Rejected under
  "invent nothing": it would type a capability the server does not have.
* **Extracting all 287 ternaries in this change.** Rejected as unreviewable. The
  ratchet makes the incremental path safe instead.
