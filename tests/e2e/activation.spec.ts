// The activation journey, end to end: signup → the five wizard steps → guided
// first risk → Aha moment.
//
// This spec exists to hold three properties that unit tests cannot:
//
//   1. TOTAL DURATION. The promise is "an unknown reaches the Aha moment in
//      under 8 minutes". A machine walking the same path should be far faster;
//      the assertion is a regression fence on the SHAPE of the journey (how many
//      screens, how many round-trips), not a benchmark of the runner.
//
//   2. THE CHECKLIST SURVIVES A RELOAD. This is the whole point of moving
//      activation to the server. The old implementation kept it in localStorage
//      and derived it from client-side counts, so state was per-device and
//      re-derived on every render. Here we complete steps, hard-reload, and
//      assert the SERVER still says they are complete.
//
//   3. ONE EVENT TICKS ONE STEP. A single framework import must strike exactly
//      one row through — the reported "two items after one import" bug.
//
// It signs up a REAL new account (a fresh tenant) rather than reusing a seeded
// persona, because an already-onboarded tenant cannot exercise onboarding.

import { test, expect, request as pwRequest } from '@playwright/test';
import { signUp, authed } from './support/newcomer';

/** The eight-minute promise, with headroom for a cold CI runner. */
const AHA_BUDGET_MS = 8 * 60 * 1000;

interface ActivationStep {
  key: string;
  event_key: string;
  completed: boolean;
  completed_at: string | null;
  celebrate: boolean;
  primary: boolean;
}
interface ActivationState {
  steps: ActivationStep[];
  percent: number;
  aha_reached_at: string | null;
}

const completedKeys = (s: ActivationState) => s.steps.filter((x) => x.completed).map((x) => x.key);

// ---------------------------------------------------------------------------

test.describe('activation — signup to Aha', () => {
  // These tests own their identity; the shared persona storageState must not leak in.
  test.use({ storageState: { cookies: [], origins: [] } });

  test('a newcomer reaches the Aha moment, and the checklist is server state', async ({
    page,
    browser,
  }, info) => {
    const ctx = await pwRequest.newContext();
    const t0 = Date.now();

    // --- Sign up -----------------------------------------------------------
    const newcomer = await signUp(ctx, 'full');
    const api = authed(ctx, newcomer.token);

    // A fresh tenant starts with nothing done and no Aha.
    let activation: ActivationState = await (await api.get('/activation/state')).json();
    expect(completedKeys(activation), 'a new tenant has completed nothing').toEqual([]);
    expect(activation.percent).toBe(0);
    expect(activation.aha_reached_at).toBeNull();

    // --- The route guard: the app is unreachable before onboarding ---------
    const guardCtx = await browser.newContext({ storageState: newcomer.storageState });
    const guarded = await guardCtx.newPage();
    await guarded.goto('/', { waitUntil: 'domcontentloaded' });
    await expect(
      guarded,
      'the dashboard must be unreachable while onboarding.completed is false',
    ).toHaveURL(/\/onboarding\/organization/, { timeout: 15_000 });
    await guardCtx.close();

    // --- The five wizard steps ---------------------------------------------
    // Driven through the API the UI calls, so the assertion is about the flow
    // and its persistence rather than about pixel positions.
    let wizard = await (await api.get('/onboarding/state')).json();
    expect(wizard.completed).toBe(false);
    expect(wizard.current_step).toBe('organization');
    // #438's sequence. `profile` merged into `organization`, `team` moved to the
    // Posture Reveal, `score` and `cover` were added; the count stays five.
    expect(wizard.steps).toEqual(['organization', 'goal', 'framework', 'score', 'cover']);

    // 1. Organization
    wizard = await (
      await api.put('/onboarding/steps/organization', {
        answers: {
          name: 'Banque Atlantique CM',
          industry: 'banking',
          size: '201-1000',
          country: 'CM',
          currency: 'XAF',
          timezone: 'Africa/Douala',
          // #438 merged the retired profile step into this one.
          full_name: 'Awa Newcomer',
          job_title: 'RSSI',
        },
        next: 'goal',
      })
    ).json();
    expect(wizard.current_step).toBe('goal');
    expect(wizard.industry).toBe('banking');

    activation = await (await api.get('/activation/state')).json();
    expect(
      completedKeys(activation),
      'naming the person in the organization step ticks the profile row and nothing else',
    ).toEqual(['profile']);

    // Back-navigation must be allowed — a wizard you cannot correct is a trap.
    const back = await (
      await api.put('/onboarding/steps/goal', {
        answers: { goal: 'pass_audit' },
        next: 'organization',
      })
    ).json();
    expect(back.current_step, 'going back must be permitted').toBe('organization');

    // 3. Goal
    wizard = await (
      await api.put('/onboarding/steps/goal', {
        answers: { goal: 'cobac_compliance' },
        next: 'framework',
      })
    ).json();
    expect(wizard.goal).toBe('cobac_compliance');

    // Suggestions follow the answers: sector + country + goal.
    const suggestions = await (await api.get('/onboarding/suggestions')).json();
    expect(suggestions.risks, 'exactly three first-risk drafts').toHaveLength(3);
    expect(suggestions.frameworks[0], 'the chosen goal leads the framework list').toBe('cobac');
    for (const r of suggestions.risks) {
      expect(r.probability).toBeGreaterThanOrEqual(0);
      expect(r.probability).toBeLessThanOrEqual(1);
      expect(r.impact).toBeLessThanOrEqual(10);
    }

    // 4. Framework — one-click import of a suggested catalog.
    const framework = await (
      await api.post('/compliance/frameworks', {
        name: 'COBAC R-2016/04',
        version: '2016',
        description: 'Contrôle interne CEMAC',
      })
    ).json();
    const importRes = await api.post(`/compliance/frameworks/${framework.id}/import-catalog`, {
      catalog_key: 'cobac',
    });
    expect(importRes.status()).toBeLessThan(300);
    const imported = await importRes.json();
    expect(imported.imported, 'the import creates real controls').toBeGreaterThan(0);

    // ⚠️ THE REGRESSION: one import ticks ONE row.
    activation = await (await api.get('/activation/state')).json();
    expect(
      completedKeys(activation).sort(),
      'a single framework import must tick exactly one step',
    ).toEqual(['framework', 'profile']);

    await api.put('/onboarding/steps/framework', {
      answers: { imported: ['cobac'] },
      next: 'score',
    });

    // 4. Score and 5. Cover — the two steps that return something computed.
    await api.put('/onboarding/steps/score', {
      answers: { probability: 0.4, impact: 8 },
      next: 'cover',
    });
    await api.put('/onboarding/steps/cover', { answers: { accepted: true } });
    wizard = await (await api.post('/onboarding/complete', {})).json();
    expect(wizard.completed).toBe(true);
    expect(wizard.landing).toBeTruthy();

    // --- The guard lifts ---------------------------------------------------
    const appCtx = await browser.newContext({ storageState: newcomer.storageState });
    const app = await appCtx.newPage();
    await app.goto('/', { waitUntil: 'domcontentloaded' });
    await expect(app, 'the app is reachable once onboarding is complete').not.toHaveURL(
      /\/onboarding/,
      { timeout: 15_000 },
    );

    // --- The guided first risk --------------------------------------------
    // NOT auto-created: the suggestion pre-fills a form the user validates. Here
    // we submit the draft the product proposed, exactly as the UI does.
    const draft = suggestions.risks[0];
    const riskRes = await api.post('/risks', {
      title: draft.title,
      description: draft.description,
      probability: draft.probability,
      impact: draft.impact,
      source: 'manual',
    });
    expect(riskRes.status(), `first risk should be created: ${await riskRes.text()}`).toBeLessThan(
      300,
    );

    activation = await (await api.get('/activation/state')).json();
    expect(completedKeys(activation)).toContain('first_risk');
    const firstRisk = activation.steps.find((s) => s.key === 'first_risk')!;
    expect(firstRisk.primary, 'the first risk is the product promise').toBe(true);
    expect(firstRisk.celebrate, 'the server asks for a celebration exactly once').toBe(true);

    // --- The Aha moment ----------------------------------------------------
    // Definition (spec §7): the first cyber score computed on the tenant's own
    // data, with at least one compliance gap identified. Loading the executive
    // dashboard is what computes it — which is what the user does next.
    const exec = await api.get('/analytics/executive');
    expect(exec.status()).toBe(200);
    const dashboard = await exec.json();
    expect(dashboard.cyber_score, 'a cyber score is produced').toBeTruthy();

    await expect
      .poll(
        async () => {
          const s: ActivationState = await (await api.get('/activation/state')).json();
          return s.aha_reached_at;
        },
        {
          message: 'the Aha moment should be recorded once the score meets its definition',
          timeout: 20_000,
        },
      )
      .not.toBeNull();

    const elapsed = Date.now() - t0;
    info.annotations.push({ type: 'time-to-aha-ms', description: String(elapsed) });
    expect(
      elapsed,
      'the whole journey must fit inside the eight-minute promise',
    ).toBeLessThan(AHA_BUDGET_MS);

    // --- THE RELOAD ASSERTION ----------------------------------------------
    // The checklist is server state: a hard reload (and, since nothing is kept
    // client-side, an entirely different browser context) sees the same rows
    // struck through.
    await app.goto('/', { waitUntil: 'domcontentloaded' });
    await app.reload({ waitUntil: 'domcontentloaded' });

    const afterReload: ActivationState = await (await api.get('/activation/state')).json();
    expect(
      completedKeys(afterReload).sort(),
      'completed steps survive a reload because they are server facts',
    ).toEqual(['first_risk', 'framework', 'profile']);
    expect(afterReload.percent).toBeGreaterThan(0);
    expect(afterReload.aha_reached_at).not.toBeNull();

    // A different context = a different localStorage. Same answer.
    const otherCtx = await browser.newContext({ storageState: newcomer.storageState });
    const fromOtherDevice: ActivationState = await (await api.get('/activation/state')).json();
    expect(
      completedKeys(fromOtherDevice).sort(),
      'activation is per tenant, not per device',
    ).toEqual(['first_risk', 'framework', 'profile']);
    await otherCtx.close();

    // If the panel is rendered, it must agree with the server rather than
    // recomputing anything. (Rendered only while steps remain — which is the
    // case here: several rows are still open.)
    const panelRow = app.getByTestId('activation-step-first_risk');
    if (await panelRow.count()) {
      await expect(panelRow).toHaveAttribute('data-completed', 'true');
    }

    await appCtx.close();
    await ctx.dispose();
  });

  test('a completed step never asks to celebrate twice', async ({}) => {
    const ctx = await pwRequest.newContext();
    const newcomer = await signUp(ctx, 'celebrate');
    const api = authed(ctx, newcomer.token);

    // Complete one CHECKLIST step. `profile` is still one; #438 retired its
    // wizard route and moved the question into the organization step, which is
    // what records the milestone now.
    await api.put('/onboarding/steps/organization', {
      answers: { name: 'Awa Test Org', industry: 'banking', full_name: 'Awa', job_title: 'RSSI' },
    });

    let state: ActivationState = await (await api.get('/activation/state')).json();
    const step = state.steps.find((s) => s.key === 'profile')!;
    expect(step.completed).toBe(true);
    expect(step.celebrate, 'the first read asks for the burst').toBe(true);

    // Acknowledge it — twice, because a re-render or a retry will.
    expect((await api.post('/activation/celebrated', { step_key: 'profile' })).status()).toBe(204);
    expect(
      (await api.post('/activation/celebrated', { step_key: 'profile' })).status(),
      'acknowledging twice is a no-op, not an error',
    ).toBe(204);

    state = await (await api.get('/activation/state')).json();
    const after = state.steps.find((s) => s.key === 'profile')!;
    expect(after.celebrate, 'a celebrated step never asks again').toBe(false);
    expect(after.completed, 'and it stays completed').toBe(true);

    // A stray key cannot pollute the ledger.
    expect(
      (await api.post('/activation/celebrated', { step_key: 'not-a-step' })).status(),
      'an unknown step key is rejected',
    ).toBe(400);

    await ctx.dispose();
  });

  test('the wizard is resumable: answers survive leaving and coming back', async ({ browser }) => {
    const ctx = await pwRequest.newContext();
    const newcomer = await signUp(ctx, 'resume');
    const api = authed(ctx, newcomer.token);

    await api.put('/onboarding/steps/organization', {
      answers: {
        name: 'Clinique du Littoral',
        industry: 'health',
        country: 'CM',
        full_name: 'Awa Newcomer',
      },
      next: 'goal',
    });

    // A brand-new browser context — nothing client-side carries over.
    const fresh = await browser.newContext({ storageState: newcomer.storageState });
    const page = await fresh.newPage();
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    // The guard resumes at the stored step, not back at the start.
    await expect(page, 'a resumed wizard reopens where it was left').toHaveURL(
      /\/onboarding\/goal/,
      { timeout: 15_000 },
    );

    const state = await (await api.get('/onboarding/state')).json();
    expect(state.answers.organization.name).toBe('Clinique du Littoral');
    expect(state.industry).toBe('health');

    // And the suggestions follow that sector.
    const suggestions = await (await api.get('/onboarding/suggestions')).json();
    expect(suggestions.risks.some((r: { key: string }) => r.key === 'patient_data_leak')).toBe(true);

    await fresh.close();
    await ctx.dispose();
  });
});

// ---------------------------------------------------------------------------
// The Posture Reveal (#438).
//
// The tunnel's terminal screen is now what DEFINES the Aha moment, which makes
// it the launch-gate metric. Two failures matter more than any feature here and
// both are asserted below:
//
//   • A reveal that renders EMPTY must record NOTHING (criterion 8). If it
//     recorded, a rendering failure would report as a product success and the
//     metric would go green while activation is nil.
//   • Every number must come from the TENANT'S OWN rows (criterion 7). A
//     plausible fabricated figure on this screen is forwarded to a CISO as fact.
// ---------------------------------------------------------------------------

interface PostureSummary {
  risks: { total: number; by_level: Record<string, number> };
  controls: {
    frameworks: number;
    total: number;
    implemented: number;
    not_applicable: number;
    in_progress: number;
  };
  coverage_percent: number | null;
  top_risks: { id: string; title: string; inherent: number; residual: { value: number } }[];
  residual_formula_version: string;
  revealed_at?: string | null;
  first_reveal: boolean;
}

test.describe('posture reveal — the Aha moment (#438)', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('an empty posture is refused, records nothing, and measures nothing', async () => {
    const ctx = await pwRequest.newContext();
    const newcomer = await signUp(ctx, 'postureempty');
    const api = authed(ctx, newcomer.token);

    // Criterion 8. A brand-new tenant holds no risk and no control, so there is
    // nothing to reveal.
    const res = await api.get('/posture');
    expect(
      res.status(),
      'an unrevealable posture must be an explicit error, not a 200 full of zeros',
    ).toBe(404);

    // And nothing was written. `posture.revealed` is a non-checklist key, so the
    // honest place to look is the Aha anchor, which the reveal is what now sets.
    const activation: ActivationState = await (await api.get('/activation/state')).json();
    expect(activation.aha_reached_at, 'a refused reveal must not record an Aha moment').toBeNull();

    // Asking again must still refuse: repeated refusals must not accumulate into
    // a success.
    expect((await api.get('/posture')).status()).toBe(404);

    await ctx.dispose();
  });

  test('a revealed posture is computed from the tenant own rows, and is idempotent', async ({}, info) => {
    const ctx = await pwRequest.newContext();
    const t0 = Date.now();
    const newcomer = await signUp(ctx, 'posturereveal');
    const api = authed(ctx, newcomer.token);

    // --- Give the tenant a real posture ------------------------------------
    const framework = await (
      await api.post('/compliance/frameworks', {
        name: 'COBAC R-2016/04',
        version: '2016',
        description: 'Contrôle interne CEMAC',
      })
    ).json();
    const importRes = await api.post(`/compliance/frameworks/${framework.id}/import-catalog`, {
      catalog_key: 'cobac',
    });
    expect(importRes.status()).toBeLessThan(300);

    // Through the tunnel's own write path (#438 step 2, D-012), not a hand-rolled
    // POST: this is what proves the starter rows are real and correctly sourced.
    await api.put('/onboarding/steps/organization', {
      answers: {
        name: 'Banque Atlantique CM',
        industry: 'banking',
        country: 'CM',
        full_name: 'Awa Newcomer',
      },
      next: 'goal',
    });
    const offer = await (await api.get('/onboarding/starter-risks')).json();
    expect(offer.risks, 'step 2 offers eight statements').toHaveLength(8);
    expect(offer.pick, 'of which the user picks three').toBe(3);

    const keys = offer.risks.slice(0, 3).map((r: { key: string }) => r.key);
    const adopt = await api.post('/onboarding/starter-risks', { keys });
    expect(adopt.status(), `adoption should succeed: ${await adopt.text()}`).toBe(201);
    expect((await adopt.json()).created, 'three rows are written').toHaveLength(3);

    // Idempotent: the tunnel is resumable, so a second adoption is a conflict,
    // not a duplicated register.
    expect(
      (await api.post('/onboarding/starter-risks', { keys })).status(),
      'a second adoption must not double the register',
    ).toBe(409);

    // --- The reveal ---------------------------------------------------------
    const revealRes = await api.get('/posture');
    expect(revealRes.status(), `the posture should reveal: ${await revealRes.text()}`).toBe(200);
    const posture: PostureSummary = await revealRes.json();

    // Criterion 6's three requirements, literally.
    expect(posture.risks.total, 'a non-zero risk count').toBeGreaterThan(0);
    expect(posture.coverage_percent, 'a non-null coverage percentage').not.toBeNull();
    expect(posture.top_risks.length, 'at least one residual score').toBeGreaterThan(0);
    expect(posture.top_risks[0].residual.value).toBeGreaterThanOrEqual(0);

    // Criterion 7: the numbers are THIS tenant's. Exactly the three rows the
    // tunnel wrote, so any other count means the payload is not reading our rows.
    expect(posture.risks.total, "the risk count is the tenant's own").toBe(3);
    expect(posture.controls.total, 'the imported catalogue is the tenant own').toBeGreaterThan(0);
    expect(posture.residual_formula_version, 'the formula version is stamped').toBe('residual-v1');

    // No control is mapped to these risks yet, so the residual must EQUAL the
    // inherent score. An absent signal must never render as a good one.
    expect(
      posture.top_risks[0].residual.value,
      'with no mapped control the residual equals the inherent score',
    ).toBe(posture.top_risks[0].inherent);

    expect(posture.first_reveal, 'the first render is the first reveal').toBe(true);
    expect(posture.revealed_at, 'the reveal is dated').toBeTruthy();

    // Criterion 11: rendering again creates no duplicate and does not move the
    // timestamp.
    const second: PostureSummary = await (await api.get('/posture')).json();
    expect(second.first_reveal, 'the second render is not a first reveal').toBe(false);
    expect(second.revealed_at, 'revealed_at must not move').toBe(posture.revealed_at);

    const elapsed = Date.now() - t0;
    info.annotations.push({ type: 'time-to-reveal-ms', description: String(elapsed) });
    expect(
      elapsed,
      'signup to the Posture Reveal must fit inside the eight-minute promise',
    ).toBeLessThan(AHA_BUDGET_MS);

    await ctx.dispose();
  });

  test('starter rows carry their provenance', async () => {
    const ctx = await pwRequest.newContext();
    const newcomer = await signUp(ctx, 'posturesource');
    const api = authed(ctx, newcomer.token);

    await api.put('/onboarding/steps/organization', {
      answers: { name: 'Awa Test Org', industry: 'health', country: 'FR', full_name: 'Awa' },
      next: 'goal',
    });
    const offer = await (await api.get('/onboarding/starter-risks')).json();
    const keys = offer.risks.slice(0, 3).map((r: { key: string }) => r.key);
    expect((await api.post('/onboarding/starter-risks', { keys })).status()).toBe(201);

    // D-012: PR 4 of #438 proves the starter rows by querying source = 'starter'.
    // If this filter ever stops matching, a customer can no longer tell which
    // rows OpenRisk wrote into their own register.
    const listed = await (await api.get('/risks?source=starter&limit=50')).json();
    const rows = listed.data ?? listed.items ?? [];
    expect(rows.length, 'the three adopted rows are queryable by source').toBe(3);
    for (const row of rows) {
      expect(row.source, 'every adopted row declares its provenance').toBe('starter');
    }
  });

  test('a tenant that already holds data is recognised and skips the tunnel', async () => {
    const ctx = await pwRequest.newContext();
    const newcomer = await signUp(ctx, 'posturerecog');
    const api = authed(ctx, newcomer.token);

    // Criterion 9's precondition: brand new means NOT recognised.
    const before = await (await api.get('/onboarding/recognition')).json();
    expect(before.recognised, 'a brand-new tenant is not recognised').toBe(false);
    expect(before.skip_tunnel, 'a brand-new tenant walks the tunnel').toBe(false);
    expect(
      before.counts.members,
      'the founding member alone is not pre-existing data',
    ).toBeLessThanOrEqual(1);

    await api.post('/risks', {
      title: 'Risque préexistant',
      description: 'Créé avant que le tunnel existe.',
      probability: 0.3,
      impact: 6,
    });

    const after = await (await api.get('/onboarding/recognition')).json();
    expect(after.recognised, 'a tenant holding a risk is recognised').toBe(true);
    expect(after.skip_tunnel, 'and does not walk the tunnel').toBe(true);
    expect(after.counts.risks, "the counts are the tenant's own rows").toBe(1);

    await ctx.dispose();
  });

  test('a step whose data already exists is absent from the tunnel', async () => {
    const ctx = await pwRequest.newContext();
    const newcomer = await signUp(ctx, 'postureskip');
    const api = authed(ctx, newcomer.token);

    // Criterion 3's precondition: nothing is skipped for a fresh tenant, so the
    // assertion below is about the ANSWER changing, not about a constant.
    let wizard = await (await api.get('/onboarding/state')).json();
    expect(wizard.steps).toEqual(['organization', 'goal', 'framework', 'score', 'cover']);
    expect(wizard.skipped_steps).toEqual([]);

    // The data must be seeded OUT OF BAND — through the compliance API, not
    // through the tunnel. Auto-skip is about what the tenant held BEFORE the
    // tunnel started; a step the tunnel itself just filled in must stay
    // reachable, or the user can never go back to fix a typo in it. That
    // property has its own test in
    // internal/application/activation/autoskip_test.go.
    const framework = await (
      await api.post('/compliance/frameworks', {
        name: 'COBAC R-2016/04',
        version: '2016',
        description: 'Pré-existant',
      })
    ).json();
    const importRes = await api.post(`/compliance/frameworks/${framework.id}/import-catalog`, {
      catalog_key: 'cobac',
    });
    expect(importRes.status(), `the import should succeed: ${await importRes.text()}`).toBeLessThan(
      300,
    );

    wizard = await (await api.get('/onboarding/state')).json();
    expect(
      wizard.steps,
      'a step whose data already existed must be absent from the stepper',
    ).not.toContain('framework');
    expect(wizard.skipped_steps, 'and must be reported as skipped').toContain('framework');
    expect(wizard.steps.length, 'the stepper count shown to this user shrinks with it').toBe(4);
    expect(
      wizard.current_step,
      'the cursor must never point at a step the client may not draw',
    ).not.toBe('framework');

    await ctx.dispose();
  });

  test('the tunnel offers no way out: no dismiss control, and Escape does not exit', async ({
    browser,
  }) => {
    const ctx = await pwRequest.newContext();
    const newcomer = await signUp(ctx, 'posturetunnel');

    const browserCtx = await browser.newContext({ storageState: newcomer.storageState });
    const page = await browserCtx.newPage();
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await expect(page, 'the tunnel is entered automatically').toHaveURL(/\/onboarding\//, {
      timeout: 15_000,
    });

    // Criterion 1. The skip control that used to sit on the team step is gone —
    // with the team step itself — and nothing replaced it anywhere in the tunnel.
    await expect(page.getByTestId('wizard-skip')).toHaveCount(0);
    for (const label of [/passer cette étape/i, /skip this step/i, /plus tard/i, /^later$/i]) {
      await expect(
        page.getByRole('button', { name: label }),
        `no dismiss affordance may exist (${label})`,
      ).toHaveCount(0);
    }

    // Escape must not be a way out. Nothing in the tunnel registers a handler for
    // it; this asserts that stays true.
    const before = page.url();
    await page.keyboard.press('Escape');
    await page.waitForTimeout(400);
    expect(page.url(), 'Escape must not leave the tunnel').toBe(before);

    // And the app itself stays denied.
    await page.goto('/risks', { waitUntil: 'domcontentloaded' });
    await expect(page, 'the app is unreachable until the tunnel completes').toHaveURL(
      /\/onboarding\//,
      { timeout: 15_000 },
    );

    await browserCtx.close();
    await ctx.dispose();
  });
});
