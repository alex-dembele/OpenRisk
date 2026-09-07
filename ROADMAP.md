# OpenRisk — ROADMAP (source de vérité unique)

> **Aligné sur le Master Prompt V5.0** (avril 2026). Réécrit et re-cartographié le **2026-07-10**.
> Mission : devenir le standard mondial du GRC, en commençant par la France, la Belgique, le Maghreb
> et l'Afrique subsaharienne (marchés **COBAC / BCEAO / ANSSI / ANTIC**). Concurrents directs :
> Vanta, Drata, OneTrust, ServiceNow GRC, Archer.
> Vision détaillée : `docs/MASTER_PROMPT_V4.md` (le **V5** la supersède — déposer le fichier V5 propre
> dans `docs/MASTER_PROMPT_V5.md`). Ce ROADMAP est la carte **module par module** du V5 avec le statut réel.

---

## Comment lire ce document

**Légende de statut**
- **✅ Fait** — implémenté ET prouvé (test live et/ou suite de tests verte + vérification manuelle documentée).
- **🟡 Partiel** — code présent mais incomplet, non câblé de bout en bout, ou jamais prouvé live.
- **❌ Absent** — aucun code, ou seulement mentionné ailleurs.
- **N/A** — étape ponctuelle (audit) ou méta.

**Règle d'or (répétée trois fois par l'histoire de ce projet, voir §2 « SetContext »)** :
aucun « ✅ » n'est accordé sans **preuve live**. Les affirmations « ça marche » du passé ont été
fausses sur `SetContext`, sur l'architecture réelle des Assets, et sur toute la chaîne
`RequireRole`/dashboard. Ne jamais faire confiance à un statut sans vérification quand l'enjeu est réel.

---

## Priorisation — tiers de backlog et trains de release (2026-09-04)

> Ajouté le 2026-09-04 d'après la note stratégique du fondateur. Cette section dit **dans
> quel ordre** on travaille. Les sections module-par-module qui suivent disent **où on en
> est** ; elles ne changent pas. Épic de suivi : **#570**. Décisions en attente :
> **D-029** (retirer ou non les 8 Waves), **D-030** (modèle canonique + OSCAL),
> **D-031** (les 9 reports en Tier 5), **D-032** (préfixe de label `tier:`).

### Les six tiers

Le backlog compte plus de 200 issues ouvertes et elles n'ont pas le même poids stratégique.
Le problème n'est pas qu'elles soient mauvaises — c'est qu'un défaut d'isolation multi-tenant
et un graphique Sankey occupent le même espace visuel dans une liste.

| Tier | Contenu | Règle |
|---|---|---|
| **0 — Trust** | multi-tenancy, authz, intégrité de l'audit, intégrité des données, secrets, SSO, RBAC, sauvegarde et restauration | Rien d'autre ne sort tant qu'un P0 de ce tier est ouvert |
| **1 — Moteur produit** | modèle de données canonique, ingestion d'événements, corrélation, moteur de risque, moteur de preuve, moteur de workflow, provenance | La plateforme. Tout le reste en dépend |
| **2 — Killer workflows** | Signal → Risque · Contrôle → Preuve · Finding → Mitigation · Risque → Impact financier | Ce que le client achète |
| **3 — Écosystème** | connecteurs, webhooks, SDK, API, OSCAL, marketplace, plugins | Ce qui nous rend difficiles à remplacer |
| **4 — UX** | design system, graphiques, animations, finition des tableaux de bord | Du vrai travail, mais qui suit le moteur |
| **5 — Reporté** | gamification, visualisations exotiques, fonctionnalités que personne ne demande | Garé, délibérément, et par écrit |

Le test à poser sur chaque issue, et désormais dans le gabarit d'issue (#563) :
**cette fonctionnalité crée-t-elle de la valeur, ou seulement l'impression de sophistication ?**

### Les sept phases

1. **Stabiliser** — multi-tenancy, RBAC, auth, piste d'audit, intégrité des données, sécurité, CI → `trust-v1`
2. **Risk Data Plane** — schéma canonique, bus d'événements, normalisation, provenance, corrélation, graphe d'entités → `Release 1.2`
3. **Deux killer workflows** — Signal → Risque → Action → Preuve · Contrôle → Test → Preuve → Écart → Risque → Remédiation → `Release 1.3` et `Release 1.4`
4. **IA gouvernée** — l'IA propose, l'humain approuve, le système enregistre → `Release 1.5`
5. **Evidence fabric** — preuve continue, historique immuable, portail auditeur, dossier d'audit → `Release 1.4`
6. **Écosystème** — SDK, webhooks, API, OSCAL, connecteurs, plugins → `Release 1.6`
7. **Échelle** — multi-org, entreprise, SaaS, marketplace, benchmarking → à planifier

### L'étoile polaire : Time to Govern

Le temps entre **le premier signal détecté** et **un risque compris + un propriétaire assigné +
une action créée + une preuve disponible**. Le GRC classique met 2 à 6 heures de travail
d'analyste. La cible est de quelques minutes. Mesure : #560.

Les cinq KPI qui la soutiennent (#561), avec leur valeur d'aujourd'hui — dont les zéros honnêtes :

| KPI | Aujourd'hui | Cible |
|---|---|---|
| Time to First Value | non mesuré | < 30 min |
| Automation Rate (champs remplis automatiquement) | ~0 % | > 80 % |
| Evidence Automation Rate | ~0 % | > 60 % |
| Closed-loop Rate | 0 % | > 50 % |
| Human Touches per Risk | ~17 | 3 |

> **Aucun de ces chiffres n'est mesuré aujourd'hui.** Les valeurs « aujourd'hui » sont des
> estimations issues de la note stratégique, pas des mesures. #560 et #561 doivent fusionner
> **avant** #547, sinon la ligne de base n'en est pas une et la règle ABSOLUE 12 nous interdira
> de publier la moindre amélioration.

### Ce que ce document ne dit pas

Cette section réorganise. Elle ne livre rien. Aucun statut de module ci-dessous ne change du
fait de son ajout, et la valeur produit viendra des Tiers 1 et 2, pas de la classification.

---

## Gate 0 — Harnais E2E & Audit UX (2026-07-24) · N/A (instrument, pas une feature)

Branche `test/e2e-ci-and-ux-audit`. **Aucun bug produit corrigé** (règle de la session) ;
seule modification produit = ajout d'attributs `data-testid` (commit isolé).

- **Harnais E2E réel** (`tests/e2e/`, Playwright) piloté contre le stack live — remplace
  la spec fictive `workflows.spec.ts` (qui visait `localhost:3000` avec des sélecteurs
  inexistants). Seed déterministe idempotent + `storageState` frappé par API. Suites :
  `smoke.routes` (les **42 routes** de `App.tsx`/`navModel.ts`), `journey.newcomer`,
  `journey.settings`, `journey.rbac`, `a11y` (axe WCAG 2.1 AA), `workflows` (cycle de risque réel).
- **Preuve live (2026-07-24)** : `npm run test:e2e` → **95 passed / 0 failed / 19 fixme**
  (chromium + Mobile Chrome). **42 routes rendent, 0 cassé, 1 dégradé, 4 placeholders** ;
  **time-to-value 3,7 s** (création d'un 1ᵉʳ risque, UX-01) ; a11y : `/risks` propre,
  contraste serious/critical sur 5 écrans (quarantainés `OR-BUG-011/012`).
- **CI** : `e2e.yml` reconstruit (actions v4/v5, Node 20, job PR bloquant chromium+Mobile,
  job nocturne firefox/webkit, table route×statut en résumé) ; `ci.yml` gagne un job
  `frontend-typecheck` bloquant.
- **Livrables** : `docs/UX_CHARTER.md` (Annexe A, 33 règles UX-01..33), `docs/UX_AUDIT_2026-07.md`
  (12 bugs `OR-BUG-001..012` + 3 personas + écrans mock + testabilité), `docs/IA_NAVIGATION_PROPOSAL.md`
  (7 groupes → 5 intentions, parcours principal 7→4 clics), `docs/UI_ELEVATION_PROPOSAL.md`
  (+ 3 mockups avant/après), `docs/DOCS_INVENTORY.md` (192 `.md` classés — proposition).
- **Constats majeurs pour les sessions 2–4** : inscription/MFA = façades sans backend
  (**P0**, OR-BUG-001/009) ; pas de chemin d'ajout de membre à un tenant (**P0**, OR-BUG-003) ;
  écrans Paramètres/sidebar = fixtures (**P1**, OR-BUG-004/005). **Propositions IA + UI à
  valider par le fondateur avant implémentation (session 5).**

## Verdict global (2026-07-10)

- **Fondations GRC (Modules 0–13)** : le cœur métier est **livré et majoritairement prouvé live** —
  Score Engine, Risk Register, Mitigation, Compliance (ISO 27001 + catalogues africains), Assets,
  Dashboard/Analytics, Notifications, Reporting PDF + Board Report IA. Restent partiels : Auth
  au-delà du login (MFA/OAuth/SAML/refresh non prouvés), CTI (non câblé), Scanner (absent),
  IA Advisor complet (seule la fondation `pkg/ai` existe), SSE (pas de hub dédié).
- **Fonctionnalités avancées (Module 14.1–14.18)** : **1 partielle, 2 faites, 15 ❌.** Faits : 14.12
  Executive Board Report ; **14.1 Incident Management** (base — registre live + War Room sur incident réel,
  fait le 13/07/2026). Partiels : Custom Fields (14.8), PAM Audit Trail (14.9),
  Champions/Gamification (14.16), Plugin Marketplace (14.18). Tout le reste (Vendor, Policy, Trust
  Center, CRQ, BCP, Training, Access Review, Data Discovery, Digital Twin, Attack Path,
  Offline) est **non commencé** — c'est là que se trouve le **moat** vs Vanta/Drata.
- **Transversaux (15–17)** : Sécurité/Observabilité partiels, i18n fait, Billing/Super-Admin/
  Onboarding/Feature-Flags-gérables ❌.
- **Product Growth (Partie C)** : design system partiel, tout le go-to-market (pricing, landing,
  conversion, rétention) ❌.

En une phrase : **OpenRisk est déjà un GRC francophone/africain crédible et fonctionnel (Wave 0/1) ;
il n'est pas encore la plateforme différenciante du Master Prompt V5 (Wave 2/3).**

---

## 1. INVENTAIRE COMPLET DES MODULES V5 (ne rien oublier)

> Statut établi le 2026-07-10 par lecture directe du code (`backend/internal`, `backend/pkg`,
> `frontend/src/features`, `migrations`), pas par mémoire. « Preuves » = fichiers/faits vérifiés.

### 1.1 Fondations — Modules 0 à 13

| Module V5 | Statut | Preuves (code réel) | Ce qui reste |
|---|---|---|---|
| **0. Audit initial** (backend/sécu/frontend) | N/A | Étape ponctuelle d'analyse, réalisée de fait au fil des sessions. | Rejouer avant chaque grosse vague. |
| **1. Score Engine** | ✅ | `pkg/scoring/` pur (P×I×A, seuils 3 décimales), `scorer_test.go`, worker Redis `ScoreWorker`. Vérifié via M3 (flux `asset.criticality_changed`). | Rien de bloquant. |
| **2. Authentification (7 couches)** | 🟡 | JWT RS256 (`internal/auth`, `AuthMiddlewareRS256`), Argon2id, MFA (`domain/mfa.go`, `pkg/otp`), OAuth2 (`oauth2_handler.go`), SAML2 (`saml2_handler.go`), RBAC (`rbac.go`, `permission_service`), PAT (`token_service`), switch-org, audit auth. **Login+dashboard prouvés live 08/07.** | **MFA, OAuth2, SAML2, refresh-token, switch-org : non prouvés live.** `TestSetupMFA_Success`/`TestRiskCRUDFlow` en échec (pré-existants). Traiter chaque sous-flux comme non prouvé. |
| **3. Risk Register** | ✅ | Backend (`application/risk`, `gorm_risk_repository`) + frontend (`features/risks`) prouvés live. Filtres, full-text, bulk, import. **Identification** manuelle + auto (CTI `cti_auto`, scanner `scan_auto`, import). **Éval. qualitative** P×I×AC (Score Engine). **Éval. quantitative** CRQ `pkg/crq` (ALE = SLE×ARO en FCFA+USD) — désormais **exposée live** dans l'onglet « Financier » du drawer du registre. **Cycle de vie ISO 31000** (Identifier→Analyser→Évaluer→Traiter→Surveiller→Clôturer) porté sur l'entité `Risk` réelle : champ `lifecycle_phase`, `TransitionPhaseUseCase` tenant-safe + garde de transition + statut couplé, endpoint `POST /risks/:id/transition`, migration 0031, stepper « Cycle de vie » dans le drawer live. **Prouvé live 16/07** (transitions valides 200 / saut +2 invalide 400 / clôture→statut closed / réouverture→open / CRQ explicite 1 M FCFA). **`created_by` renseigné depuis le contexte réel** (vérifié live 16/07 : nouveau risque → `created_by` = id admin ; l'ancienne note « reste uuid.Nil » était périmée depuis le fix `SetContext` du 08/07 — seul le chemin CTI auto met volontairement `uuid.Nil`, « no human author »). **Legacy `/risk-management/*` SUPPRIMÉ** le 16/07 (branche `fix/remove-legacy-risk-management`) : 4 fichiers backend + câblage main.go + page/hook/service/composants front orphelins retirés. | Client OpenAPI non rétrofit (choix délibéré : le client typé écrit à la main fonctionne, rétrofit = risque sans valeur user). L'entrée d'audit `CreateAuditEntry`→`audit_logs` est un no-op pré-existant (schéma incompatible) ; les transitions sont tracées via `risk_histories` (hook AfterSave). `TestRiskCRUDFlow` rouge (pré-existant, confirmé au commit parent `62191679`). |
| **4. Mitigation Workflow** | ✅ | Backend (`application/mitigation`, sous-actions, progress→review) + Kanban frontend (`features/mitigations`). Routes d'écriture corrigées (bug `RequireRole`). | Auto-mitigation **détectée** par le scanner (diff findings scan N-1→N, onglet dédié du preview, prouvé live 14/07) ; **auto-complétion d'une sous-action** de plan reste à câbler sur ce signal. Vue Gantt à confirmer. |
| **5. CTI Engine** | 🟡 | `pkg/cti/` présent (NVD/CISA/MITRE, matcher CVE→asset). | **Non câblé** : pas de worker de sync émetteur, pas d'auto-création de risque active, endpoints non exposés. « En avance de phase Wave 2 ». |
| **6.5 Vulnerability Management** (intégrations + priorisation) | ✅ (prouvé live 16/07) | **Module dédié** (branche `feat/vulnerability-management`). Registre de vulnérabilités tenant-scoped `domain.Vulnerability` (CVE, CVSS, EPSS, KEV, exploit, criticité métier de l'actif, blast radius, statut de remédiation, priorité P1–P4 ; dans `AutoMigrate`) + repo Gorm (upsert par dedup, filtres/tri, stats). **Priorisation risk-based** `pkg/vulnprio` (pur, 8 tests) sur les 4 axes demandés : **CVSS** (0.40) · **exploitabilité** (0.30 — EPSS/KEV/exploit dispo, KEV plancher P1) · **criticité métier** (0.20 — facteur de l'actif lié) · **actifs concernés** (0.10 — blast radius log). **Intégrations** `internal/vulnscan` : normaliseurs pour **Nessus, OpenVAS, Qualys, Microsoft Defender, AWS Inspector, Azure Defender, CrowdStrike** (mapping du format natif → finding normalisé ; AWS Inspector = live-pull possible, les autres import + seam honnête). Ingest (normalise→priorise→upsert, lie l'actif par hostname/id), list/get/update-status/delete/stats (use cases 1-fichier tenant-safe + tests) → handler + routes `vulnerabilities:*` + `/vulnerability-connectors`. **Frontend** `features/vulnerabilities` (page registre triée par priorité + KPI + filtres, drawer détail avec explication de priorisation + cycle de statut, modal d'import multi-source, panneau connecteurs ; item sidebar « Vulnérabilités » ; tsc+vite verts). **Preuves live** (Postgres:5434) : 7 connecteurs ; ingest Nessus Log4Shell→CVE extraite/sévérité/actif web-01 CRITICAL/priorité ; CrowdStrike exploit→maturité high ; **KEV CVSS 5.0 → floored P1 (80)** ; **blast radius 3 actifs → 66.97** ; tri par priorité ; stats ; update-status ; delete 204/404 ; **capture headless de la page rendue**. **Les 5 deltas restants COMPLÉTÉS le 17/07 (branche `feature/vulnerability-management-connectors`, 8 commits)** : **(a) config des intégrations** (`domain.VulnIntegration` tenant-scoped, creds AES-256-GCM write-only jamais renvoyées, base URL, live-pull+schedule, token webhook, toggles auto-risque/auto-ticket ; CRUD `/vulnerabilities/integrations` + `/ticketing`) ; **(b) webhook entrant** `POST /vulnerabilities/webhook/:source` auth par token opaque, monté avant la porte JWT ; **(c) live-pull REST RÉEL** `internal/vulnscan/livepull` (Defender/CrowdStrike OAuth2, Nessus X-ApiKeys, Qualys XML, Azure ARM — vrais appels HTTP, httptest ; OpenVAS/AWS = seams honnêtes) + `POST /vulnerabilities/integrations/:id/pull` + scheduler `VULN_LIVEPULL_ENABLED` ; **(d) enrichissement CTI** (`WithCTIEnricher` : KEV/CVSS/sévérité depuis `cti_vulnerabilities` avant priorisation → une vuln CVSS modeste plancherée P1 si KEV) ; **(e) vuln→risque auto** (`WithRiskProposer`, P1/KEV+actif → `internal/infrastructure/vulnrisk` idempotent, `risk_id` lié) ; **(f) auto-ticketing** (`pkg/ticketing` Jira/ServiceNow réels + `POST /vulnerabilities/:id/ticket` manuel + auto-open P1/KEV). Frontend `IntegrationsPanel` (formulaires creds + URL webhook copiable + Pull now + toggles + onglet Ticketing) ; drawer montre lien ticket + risque lié. **Prouvé live (binaire :8095)** : creds jamais renvoyées, webhook 202/401/400, **live-pull → vrai appel Qualys → 401 réel enregistré** (aucun faux). | EPSS dédié absent du flux CTI (allumage futur) ; OpenVAS/AWS Inspector = seams live-pull (webhook/import OK) ; creds de vraie infra non exercées (prouvé par httptest + appel Qualys réel). |
| **6. Infrastructure Scanner** (cloud + Agent on-prem) | ✅ (prouvé live de bout en bout) | **Complet 14/07** — backend `internal/scanner/` (interface `Scanner`, pipeline `Validate→Scan→Normalize→Deduplicate→StorePreview→Notify` qui **n'écrit jamais** Asset/Risk, preview Redis 48h, dedup, **auto-mitigation par diff**, isolation tenant, ~30 tests) + **collectors SDK réels** `internal/scanner/collectors` (**AWS** aws-sdk-go-v2 EC2/S3+chiffrement/Security Hub, **Azure** Resource Graph KQL, **GCP** Compute aggregated list ; creds AES-256-GCM déchiffrées au scan) + **binaire Agent** (module `agent/`, **6,5 Mo** stdlib pur : register token 24h → SSE jobs + heartbeat → **nmap `-sV --script vuln`** + osquery locaux → parse XML → **push JWT scopé + HMAC-SHA256** ; stateless ; scope ≤/24 ; `-install` systemd ; auto-update GitHub 24h) + **notif in-app + email** sur fin de scan + **frontend** `features/infrastructure` (console live, ScanConfigDrawer, AgentDeployModal, ScanPreviewPage Actifs/Vulns/Mitigations + import criticité éditable ; tsc+vite verts). **Preuves live** : creds cloud chiffrées + 3 SDK réellement appelés (EC2 401 / AAD auth / GCP parse) ; agent scanne 127.0.0.1/32 → **1 actif + 22 CVE réels** (OpenSSH) → push HMAC vérifié → job completed → notif unread 0→1 → révocation 401. **Découverte automatique — 7 connecteurs supplémentaires (17/07, branche `feature/auto-asset-discovery`)** couvrant toutes les catégories de la spec « 6. Découverte automatique des actifs » : **Kubernetes** (client-go — Nodes/Pods, pod privilégié→finding), **Docker** (SDK Moby — containers/images, host-network→finding), **VMware** (govmomi — VMs, VMware Tools EOL→finding, testé contre le simulateur vcsim), **Active Directory** (go-ldap — computers/users, OS EOL + never-expire→findings), **Microsoft 365** (Graph + azidentity — users/devices, device non-conforme→finding), **GitHub** (go-github — repos, public→finding), **GitLab** (client officiel — projects, public→finding). Chacun réutilise le seam `CloudCollector` + le pipeline (chiffrement AES-256-GCM, planification cron, preview Redis, import) via le chemin SaaS non-agent — **zéro modif du pipeline**. Chaque collecteur a un test réel (httptest / fake clientset / vcsim, sans réseau). Frontend : 7 cartes providers + formulaires de credentials par provider dans `ScanConfigDrawer`. **Preuve live (17/07)** : les 7 configs créées (201), validation cred manquant (400 « missing required credential »), aucun credential fuité ; **scan GitHub exécuté de bout en bout → vrai appel `GET api.github.com/orgs/acme/repos` → 401 Bad credentials → preview vide honnête** (comme AWS/Azure/GCP) ; scan Kubernetes dispatché+exécutant (dial API server). | **Defender (Azure) + Security Command Center (GCP)** findings = SDK/paths supplémentaires (assets cloud OK, alerts cloud à ajouter). Livraison de release binaire Agent (self-replace) reste manuelle. Les 7 nouveaux connecteurs : chemins d'authentification réels non exercés contre une vraie infra (pas de credentials/cluster/vCenter/DC de test) — prouvés par tests d'intégration hors-ligne + l'appel API GitHub réel ; **Defender/SCC/SNMP** restent à ajouter. |
| **7. SSE Real-time Engine** | 🟡 | `useSSE` côté frontend ; références SSE dans `notification_service.go` ; `pkg/events`. | **Pas de hub SSE dédié** (`infrastructure/sse/hub.go` absent), endpoint `/api/v1/stream` non confirmé. La route `/risks/events` attendue par le client n'existe pas (repli en log dev). |
| **8. Dashboard & Analytics** | ✅ (+ **Tableau de bord exécutif spec §11**, prouvé live 18/07, branche `feature/executive-dashboard`) | `application/analytics`, `analytics_service`, `dashboard_data_service`, `stats_handler`, `features/dashboard`. **Prouvé live 08/07** (crash dashboard corrigé). **Tableau de bord exécutif (« 11. Tableau de bord exécutif ») : la page `/analytics` était une maquette 100 % fixtures (`AnalyticsCiso.tsx`, badge « Aperçu »)** → remplacée par un vrai dashboard piloté par données. **Agrégation consolidée** `internal/application/dashboard.GetExecutiveDashboardUseCase` (ports optionnels nil-safe composant les sources tenant-scoped existantes : `FinancialSummaryUseCase`, `GormRiskRepository`, `GetGapAnalysisUseCase`, `VulnerabilityRepository`, `IncidentService`) → **UN seul endpoint** `GET /analytics/executive` (`risks:read`, tenant via `mwCtx.OrganizationID`) au lieu de 8+ requêtes front. **Cyber score** composite déterministe `cyber_score.go` (0–100 + note A–F, 4 axes pondérés conformité .35/risques .30/vulns .20/incidents .15, renormalisation des axes absents ; 5 tests). Repo : `MonthlyRiskTrend` (risk_histories + ancrage mois courant sur le registre live) + `TopRisksByScore` ; `IncidentService.GetIncidentAnalytics` (open/critique/**MTTR**/taux de résolution/trend mensuel). **Frontend** `features/analytics/ExecutiveDashboard.tsx` (Recharts : jauge cyber score SVG, KPI exposition ALE, bande de KRI, LineChart évolution du risque, donut par criticité, table top-10 + ALE, RadarChart couverture des contrôles + anneaux par référentiel, histogramme empilé de tendance des incidents) + service/hook typés zéro `any` ; `tsc -b`/`vite build` verts. **Prouvé live (binaire :8097, Postgres:5434+Redis)** : `GET /analytics/executive` **HTTP 200**, payload consolidé cohérent — cyber score **F/31** (4 axes normalisés), ALE **117,5 M FCFA** (pire cas 235 M) depuis le moteur CRQ, **8 top-risks** triés par score avec ALE réel (Log4Shell KEV présent), **10 référentiels** de couverture, distribution 2 crit/1 haut/5 moyen (total 8 = total financier = total trend), 7 KRI, MTTR, tendance incidents. | 2 widgets *du dashboard `/` d'accueil* (`SecurityScore`, `AssetStatistics`) appellent `/analytics/security-score` & `/analytics/assets/statistics` **inexistants** (repli gracieux). Trend risque = 1 point (mois courant) tant que `risk_histories` est peu peuplé (ancrage live honnête). Frontend exécutif prouvé par build + endpoint live (pilotage CDP interactif bloqué par le sandbox, comme 14.5). Filtres temporels sur le dashboard exécutif = prochaine itération. |
| **9. Notifications** | ✅ (cœur) | `pkg/notify`, `notification_service`, `notification_handler`, centre de notifs frontend. | Canaux **Email (Resend/SMTP), Slack, Webhook signé** non prouvés live. Webhooks sortants à vérifier. |
| **10./12. IA (spec §12)** | ✅ (5 capacités, prouvé live 19/07, branche `feature/ai-integration`) | **Service IA unifié** `pkg/ai.Assistant` (généralise l'`Advisor` du Board Report) : `ClaudeAssistant` (SDK `anthropics/anthropic-sdk-go`, `claude-opus-4-8`, adaptive thinking, JSON strict) + `TemplateAssistant` (repli déterministe sans clé) + factory `NewAssistant`. **5 capacités = 5 endpoints** : (1) **synthèse de risque + plan de traitement** `POST /ai/risks/:id/treatment-plan` → onglet « IA » du drawer de risque (bouton « Générer avec l'IA », stratégie + plan d'actions priorisé) ; (2) **détection de risques émergents** `POST /ai/emerging-risks` (analyse texte threat-intel/news/logs, dédup des risques existants) → page `/ai/emerging-risks` ; (3) **assistant Q&A langage naturel** `POST /ai/assistant/query` (**RAG hybride** : recherche plein-texte sur les risques du tenant + mots-clés sur contrôles + recherche vulns → réponse sourcée) → chat `/recommendations` (remplace le mock) ; (4) **génération de rapport d'audit** `POST /ai/audits/:id/report` (compose audit + gap analysis + remédiations ouvertes) → bouton « Rapport IA » + modale sur `AuditsPage` ; (5) **analyse de preuve documentaire** `POST /ai/evidence/:id/analyze` → contrôle inline « Analyser (IA) » sous chaque preuve dans le drawer de contrôle (verdict coloré satisfies/partial/insufficient/unrelated + confiance). `GET /ai/status` (Claude actif vs mode local). App layer `internal/application/ai` (ports étroits nil-safe, repli template sur toute erreur LLM — même contrat que le Board Report). Service/hooks frontend typés zéro `any`. **Prouvé live (binaire :8098, Postgres:5434+Redis)** : les 6 endpoints répondent 200 avec données réelles du tenant — Q&A retrouve le risque Log4Shell CVE-2021-44228 + contrôles cités ; treatment-plan sur « Exposed admin panel » (ALE 20 M FCFA, stratégie mitigate, 3 actions) ; emerging-risks détecte 4 risques (ransomware/phishing/vuln/supply-chain) ; audit-report sur le gap réel (326 contrôles/274 écarts/16 %) ; evidence-analysis verdict « insufficient » honnête ; 404/400 corrects. `go build`/`vet`/tests (`pkg/ai` 5 + `application/ai` 10) verts, `tsc -b`/`vite build` verts. | Chemin `ClaudeAssistant` non prouvé live faute d'`ANTHROPIC_API_KEY` (compile + repli template fonctionnel — à revérifier avec une clé, comme le Board Report). Analyse de preuve = métadonnées (nom/description) ; extraction du contenu du fichier (OCR PDF/image) = prochaine itération (confiance abaissée sans extrait, jamais de faux « satisfies »). Pas de base vectorielle (RAG = recherche hybride mots-clés/plein-texte) ni de streaming/cache IA. Frontend prouvé par tsc/build + endpoints live (pilotage CDP interactif bloqué par le sandbox). `ai_risk_predictor_service` (code mort, non câblé) laissé tel quel. |
| **11. Reporting & Export** | 🟡 | `pkg/report` (PDF `fpdf`, **conformité + Board Report ✅**), export CSV risques (`export_risks.go`), `export_handler`. | Pas de `pkg/export` async (jobs Redis, XLSX `excelize`, MinIO/S3, TTL). **Templates officiels COBAC/BCEAO/ISO/PCI ❌.** `ReportsPage.tsx` = maquette non câblée. |
| **12. Compliance Frameworks** | ✅ (moteur + couverture large + audits + gap + remédiation) | Moteur M1 vérifié live ; **16 référentiels cités et importables** : ISO 27001:2022 (93) + **ISO/IEC 27005:2022 (19, processus de gestion du risque)** + **ISO 31000:2018 (22, principes/cadre/processus)** + **NIST CSF 2.0 (22)** + **NIST SP 800-53 Rev.5 (20 familles)** + **CIS Controls v8 (18)** + **PCI DSS 4.0 (12)** + **HIPAA Security Rule (22, cit. 45 CFR §164)** + **SOC 2 / AICPA TSC (51)** + **RGPD / UE 2016/679 (22 articles, FR)** + **DORA / UE 2022/2554 (19, 5 piliers, FR)** + **NIS2 / UE 2022/2555 (12, art. 21 mesures, FR)** + **SOX 2002 (10, sections + ITGC)** + BCEAO (35) + ANTIC-CM (25) + COBAC (45) — chacun cité article/section par contrôle (`TestNoOrphanControls` + `TestExpectedControlCounts`). Architecture plugin : un fichier `pkg/compliance/catalog_*.go` + `register()` → le catalogue remonte automatiquement dans `GET /compliance/catalogs` et devient importable, **zéro changement handler/frontend**. Frameworks **tenant-scoped** (migration 0030). **Gestion complète** : créer/importer/supprimer référentiel + contrôle (RBAC), **personnalisation** (référentiel vierge + contrôle ad-hoc), preuve en chip cliquable, **progression temps réel**, **seuil de preuve obligatoire (mode strict)**. **Analyse d'écarts (Gap Analysis) ✅ (16/07, branche `feature/compliance-management`)** : `GET /compliance/gap-analysis` (tous référentiels ou 1) → contrôles non satisfaits + roll-ups par référentiel ; page dédiée (jauge, filtres, écarts groupés) branchée sur le bouton « Voir les écarts » (auparavant inerte) ; « Remédier » par écart. **Audits ✅** : `domain.ComplianceAudit` (interne/externe/certification/surveillance ; planifié→en cours→terminé/annulé ; portée référentiel/programme ; auditeur/périmètre/résumé/score/dates), CRUD `/compliance/audits`, page de planification/exécution/historique. **Plans de remédiation ✅** : `domain.RemediationPlan` lié à un contrôle (l'écart) + à l'audit d'origine ; priorité/statut/assignation/échéance ; CRUD `/compliance/remediations`, page de suivi. **Auto-génération depuis un audit terminé ✅ (17/07)** : `POST /compliance/audits/:id/generate-remediations` ouvre un plan pour chaque écart du référentiel de l'audit (priorité déduite : not_implemented→high, in_progress→medium ; idempotent — saute les écarts déjà couverts) ; bouton « Remédier » par audit → toast créés/ignorés → registre. **Cross-mapping entre référentiels ✅ (17/07)** : `domain.ControlMapping` (crosswalk non-dirigé tenant-scoped entre deux contrôles + relation équivalent/partiel/lié), repo Gorm (Exists bidirectionnel, isolation tenant), use cases (Create = double garde cross-tenant + refus self/doublon, List enrichi, Delete) → `GET/POST /compliance/control-mappings` + `DELETE /:mappingId` ; section « Correspondances » dans le drawer de contrôle (lier vers un autre référentiel). **OpenAPI ✅ (17/07)** : gap-analysis/audits/remédiation/mappings dans `docs/openapi.yaml` (schémas + paths + `required`), types régénérés, `types/compliance.ts` re-pointé contract-first (enums dérivés). **Prouvé live 16-17/07** : 17 catalogues (16 dispo + 1 placeholder) bons comptes ; import DORA (19) ; gap-analysis (tous : 326 contrôles/274 écarts ; DORA 19/19/0 %) ; audit create→complete ; auto-remédiation 19 créés (201)→re-run 19 ignorés (idempotent) ; cross-mapping create 201/self-link 400/doublon inverse 409/list symétrique/delete 204 ; audit programme-wide→400. | Catalogues modélisés au niveau catégorie/exigence (sous-contrôles ajoutables en ad-hoc) ; 1 placeholder (`cm-loi-2024-017`, texte non fourni) ; cross-mapping = liens manuels (pas de crosswalk curé pré-rempli ISO↔NIST). |
| **13. Asset Management** | ✅ (inventaire + criticité + **dépendances** + historique, prouvé live 16/07) | M3 : Clean Architecture rétrofitée, snapshots historiques, criticité→Score Engine. `features/assets`. **Gestion centralisée complète (16/07, branche `feat/asset-dependency-mapping`)** : (1) **inventaire** étendu à la taxonomie GRC (Server/Application/Cloud/Database/SaaS/Storage/Network/Laptop/**Data/User/Supplier**) — icônes + chips de filtre ; (2) **classification par criticité** (déjà là, → Score Engine) ; (3) **cartographie des dépendances entre actifs** — nouveau modèle `AssetDependency` (arête dirigée tenant-scoped, 8 types de relation), repo+use cases (Create/List/Delete, gardes self-ref/doublon/cross-tenant, cascade à la suppression d'actif), handler `/asset-dependencies`, OpenAPI, `Asset Universe` **rebranché sur données réelles** (fixtures supprimées) avec éditeur de dépendances (ajout/retrait) dans le panneau ; (4) **historique des modifications** enfin exposé dans l'UI live (bouton « Historique » sur la modale d'édition → `AssetHistoryDrawer`). **Traçabilité « qui » complétée le 17/07 (branche `feature/centralized-asset-management`)** : le snapshot capturait le *quoi*/*quand*/*pourquoi* mais pas l'auteur ; ajout de `AssetSnapshot.ChangedBy` (uuid persisté, migration 0032) renseigné depuis `userID(c)` par les use cases update/delete, résolu en `changed_by_email` en lecture via un port optionnel `UserLookup` (`GormUserRepository.EmailsByIDs`, dégradation gracieuse), affiché « Modifié par … » dans le drawer (i18n FR/EN). **Prouvé live** (Postgres:5434) : login→create→2×PATCH→history = 2 snapshots newest-first, chacun `changed_by`=id admin réel + `changed_by_email`="admin@opendefender.io" ; delete 204→get 404. Preuves live antérieures : graph 7 actifs · 7 liens rendu, inventaire 11 types, endpoints 201/409/404/204, cascade + snapshot vérifiés. | Les 4 autres vues Universe (topology/bubbles/hierarchy) restent « coming soon ». Matching CVE via CPE dépend du CTI/Scanner (déjà câblé, item 21). Criticité = niveau unique (LOW→CRITICAL) ; décomposition en sous-scores DICT/CIA = enrichissement optionnel non demandé par la lettre de la spec. |

### 1.2 Fonctionnalités avancées — Module 14.1 à 14.18 (le **moat** vs Vanta/Drata)

| Module V5 | Statut | Preuves / absence | Note |
|---|---|---|---| 
| **14.1 Incident Management** | ✅ (base) | **Rendu fonctionnel + prouvé live le 13/07/2026** (branche `feat/ui-redesign-dc-html`). Tables ajoutées à `AutoMigrate` ; 3 bugs corrigés (`Preload("Risk")` inexistant, param `:id`/`:incidentId`) ; **registre d'incidents live** (`features/incidents/` : stats KPI, filtres, table, drawer détail/édition, création, statut inline, export CSV) ; **War Room câblée sur un incident réel** (`/incidents/:id/war-room` : en-tête + chronologie réels, durée live/figée, clôture persistée) ; tests handler sqlite (E2E + cross-tenant). | `RiskID *uint` ↔ risques uuid → `LinkRisk`/incidents-par-risque cassés (non exposés) ; roster/tasks/chat War Room = fixtures (pas de backend collaboration) ; service non retrofit Clean Architecture. |
| **14.2 Vendor Risk Management** | ❌ | Aucun package vendor. | Questionnaires publics, auto-scoring, rappels J-7/J-3/J-1. |
| **14.3 Policy Management** | ❌ | Aucun package policies. | Éditeur Markdown, versioning, acknowledgments. |
| **14.4 Trust Center Public** | ❌ | Aucun package trustcenter. | Page publique `trust.openrisk.io/{slug}`. |
| **14.5 Cyber Risk Quantification (FAIR) = « 9. Quantification financière »** | ✅ (moteur complet + dashboard CFO/CISO, prouvé live 17/07, branche `feature/financial-risk-quantification`) | Moteur `pkg/crq` étendu au modèle FAIR complet : **SLE** (explicite OU composé = coût des interruptions `downtime_hours × hourly_cost` + amendes + perte de données + autre), **ARO**, **ALE = SLE × ARO** (FCFA + USD), **pertes max/moyennes** (bande triangulaire/PERT autour du SLE), **coût de remédiation**, **ROSI = (ALE − ALE_résiduel − coût_remédiation) / coût_remédiation** (ALE_résiduel = ALE × (1 − efficacité)). 7 champs monétaires stockés sur `domain.Risk` (migration 0034). Endpoints `GET /risks/:id/financial` (assessment complet), `POST /risks/:id/simulate` (what-if non-persisté → simulateur d'investissement), `GET /analytics/financial` (posture agrégée tenant : ALE portefeuille/pire-cas/résiduel, budget remédiation, ROSI portefeuille, par criticité, top expositions). Frontend `features/financial` : **dashboard CFO/CISO** (`/analytics/financial` : KPI, projection Recharts pertes cumulées avec/sans contrôles, exposition par criticité, top expositions, **simulateur d'investissement** live) + onglet « Financier » du drawer enrichi (SLE/downtime/pire-cas/moyen/résiduel/ROSI + saisie des 7 drivers). Tests purs (`financial_test.go` : downtime, ROSI incl. négatif/indéfini, SLE composé vs explicite, bande, clamp) + use-case agrégat (`financial_summary_test.go`). **Prouvé live** : SLE composé 61 M FCFA, downtime 36 M, ALE 30,5 M, ROSI 1,14 ; simulate 5 M @ 90 % → ROSI 4,49 ; portefeuille pire-cas ≥ ALE. | Chemins d'auth réels non exercés (n/a) ; le dashboard authentifié n'a pas été capturé headless (artefact du shell auth en sandbox — vérifié sur une page existante ; SPA rend bien headless, login prouvé) — prouvé par tsc/vite + les 3 endpoints live. Le Board Report garde son modèle FCFA de référence simple (ordre de grandeur board), distinct de ce moteur par-risque. |
| **14.5b Smart Risk Calculation = « 8. Calcul de risque intelligent »** | ✅ (moteur multifactoriel 8 facteurs + radar + pondérations configurables, prouvé live 18/07, branche `feature/smart-risk-calculation`) | Nouveau moteur **pur** `pkg/scoring/smart.go` (stdlib, déterministe, 10 tests) : blende **8 facteurs** — (1) criticité métier, (2) exposition Internet, (3) vulnérabilités/CVSS, (4) maturité des contrôles, (5) historique d'incidents, (6) exploitabilité, (7) valeur financière, (8) menaces actives (CTI) — en un score **0–100** via `SmartScore = 100 × Σ (poids_normalisé_i × facteur_i)`. **Pondérations configurables par tenant** (`domain.RiskScoringWeights`, migration 0035, valeurs relatives normalisées). Le **Score Engine classique P×I×AC reste intact** (vue additionnelle). Assemblage `ComputeSmartScoreUseCase` : ports optionnels nil-safe (asset, vuln register, compliance, incidents, CRQ) → chaque facteur dégrade gracieusement ; fallback asset via la relation many2many. Colonnes `smart_score`/`smart_level`/`smart_factors`(jsonb)/`smart_computed_at` sur `Risk`. Endpoints `GET /risks/:id/smart-score` (calcul+cache), `POST /risks/:id/smart-score/simulate` (aperçu non-persisté = tuning live), `GET|PUT /risk-scoring/weights` (lecture / écriture admin). Frontend `features/risks` : **radar Recharts** (`SmartRiskRadar`) + onglet « Score intelligent » du drawer + écran `RiskWeightsSettings` (`/risks/weighting`, sliders + part effective normalisée). **Prouvé live 18/07** (Postgres:5434+Redis) : score 36→84 quand une vuln KEV Log4Shell (CVSS 9.8) est liée à l'actif (exploitability 0.95 CISA-KEV, threat_intel 1.0, vulnérabilités 0.773) ; criticité métier 3.0/3.0 depuis l'actif CRITICAL ; maturité contrôles 11 % (vraie donnée compliance) ; ALE 20 M XAF (vrai CRQ) ; simulate all-business → 90 critical ; PUT weights persistées + recalcul ; validations 400 (all-zero, poids>1) ; cache DB (smart_score/level/timestamp) ; 404. Tests use-case (success/not-found/dégradation/preview/défauts/validations). | Frontend prouvé par tsc/vite + les 4 endpoints live (pilotage CDP interactif bloqué par le sandbox, comme 14.5). Historique d'incidents = match best-effort sur `impacted_assets`/titre (le modèle Incident legacy n'a pas de FK propre vers l'actif). Facteurs 3/6/8 alimentés par le registre de vulnérabilités de l'actif. |
| **14.6 Business Continuity (BCP/PCA-PRA)** | ❌ | Aucun package bcp. | RTO/RPO, plans de reprise, tests. |
| **14.7 Security Awareness Training** | ❌ | Aucun package training. | Modules + quiz, génération IA. |
| **14.8 Custom Fields** | 🟡 | `domain/custom_field.go`, `custom_field_service`, `custom_field_handler`, `CustomFields.tsx`. | Rendu dynamique `<DynamicField>` à vérifier. |
| **14.9 PAM Audit Trail (append-only)** | 🟡 | `domain/admin_audit_event.go`, `admin_audit_service.go`. | **Trigger PostgreSQL append-only, anomaly detector, frontend `AdminAuditPage` : non confirmés.** Le cœur du 14.9 (immutabilité garantie) reste à prouver. |
| **14.10 Access Review & Certification** | ❌ | Aucun package accessreview. | Privilege-creep detector, campagnes JML, révocation auto. |
| **14.11 Sensitive Data Discovery** | ❌ | Aucun package datadiscovery. | Scanner PII/PCI/secrets multi-sources + auto-risque. |
| **14.12 Executive Board Report** | ✅ | **Fait 10/07** (branche `feat/m4-compliance-report-pdf`). `pkg/ai` + `application/board` + `domain.BoardReport` + `pkg/report/board_pdf.go` + handler + `features/reports/BoardReportPage`. Flux complet prouvé live (génération→édition→approbation→PDF). | **Chemin `ClaudeAdvisor` non prouvé live** (pas d'`ANTHROPIC_API_KEY`) — repli template déterministe fonctionnel. Voir §2 M4. |
| **14.13 Risk Digital Twin (Simulation)** | ❌ | Aucun package simulation. | Propagation BFS/DFS + suggestions IA. Gamechanger. |
| **14.14 Collaborative War Room** | ❌ | Aucun package warroom. | Chat/kanban/timeline SSE, auto-trigger score≥9. |
| **14.15 Attack Path Graph** | ❌ | Aucun package attackpath. | Chemins d'attaque + blast radius. |
| **14.16 Risk Champions Leaderboard** | 🟡 | `gamification_service.go`, `features/gamification`, `gamification_handler`. | Moteur points/badges/streak V5 complet + notifications de dépassement : partiel/non prouvé. |
| **14.17 Offline-First Mode** | ❌ | Rien (Workbox/Dexie/sync). | Différenciateur Afrique majeur. |
| **14.18 Plugin Marketplace** | 🟡 | `domain/marketplace.go`, `marketplace_service`, `marketplace_handler`, `Marketplace.tsx`. | **Structs sans tags `gorm:` → exclues d'`AutoMigrate`** → non fonctionnel. Dispatcher webhook/sandbox à faire. |
| **14.19 Security Automation / SOAR = « 10. Automatisation »** | ✅ (moteur event-driven + chaîne d'actions + SLA/escalade/clôture auto + connecteurs Slack/Teams, prouvé live de bout en bout 18/07, branche `feature/security-automation-workflow`) | Nouveau moteur SOAR : `domain.AutomationRule` (trigger + conditions + **chaîne d'actions ordonnée** + politique SLA), `AutomationExecution` (journal d'audit par déclenchement, steps jsonb), `SLATracker` (compte à rebours vivant + état d'escalade), `AutomationChannelConfig` (Slack/Teams/email par tenant, webhooks write-only) — migration 0036 + AutoMigrate. **Moteur pur** `application/automation.Engine` : matche les règles activées du tenant contre un `TriggerContext` normalisé et exécute la chaîne (scan → créer risque → assigner → ticket → notifier → démarrer SLA → résoudre/clôturer) ; **chaque port d'action est optionnel/nil-safe** (capacité absente → step « skipped », jamais d'échec dur). **Connecteurs** : `pkg/notify/chatops.go` (clients webhook **Slack** + **Microsoft Teams** réels, sans dépendance, httptest) + dispatcher `infrastructure/automation.Notifier` (résolution des destinataires owner/rôle) ; **Ticketer** réutilise le `VulnTicketingConfig` + `pkg/ticketing` (Jira/ServiceNow) ; RiskActions (créer/assigner/résoudre sur l'entité `Risk` réelle) ; ScanAction. **Event-driven** : l'ingest de vulnérabilités publie `vulnerability.detected` → `workers.AutomationWorker` (abonné Redis) alimente le moteur ; **`workers.SLAMonitor`** (cadence 1 min) escalade les remédiations dépassées (→ managers) et **clôture automatiquement** les SLA dont le risque est résolu (étapes 7 & 8 de la spec). Endpoints `/automation/rules` (CRUD + `/:id/test` dry-run), `/automation/executions`, `/automation/sla` (+`/stats`), `/automation/channels` (RBAC `automation:read/write`, canaux admin). **Frontend** `features/automation` : **constructeur de workflow** (chaque règle affiche sa chaîne trigger→actions), **dashboard SLA en cours** (KPI + barres de progression + escalade), historique d'exécutions, config des canaux. **Prouvé live 18/07** (Postgres:5434+Redis) : ingest CVE critique fraîche → `vulnerability.detected` → moteur exécute la règle (`success`, ref `cve:…`) → exécution journalisée + SLA créé (240 min) ; dry-run chaîne complète (créer risque idempotent → assigner admin → notifier teams+in_app → SLA) ; **escalade** (tracker dépassé → `escalated`, level 0→1, en ~20 s) ; **clôture auto** (risque → `mitigated` → SLA `met` en ~20 s) ; **captures headless** de la vue Règles (2 workflows visualisés) et du dashboard SLA (escaladé « dépassé de 37m » + 3 en cours). ~15 tests (moteur : gates conditions, chaîne, skip, dry-run ; SLA : escalade/auto-close/stats ; règles : validation + isolation tenant ; chatops httptest). | Chemins d'auth réels des webhooks Slack/Teams/ITSM non exercés contre une vraie infra (prouvés par httptest + le pipeline live) ; l'action `scan_asset` déclenche la 1re config de scan du tenant (le scanner est basé sur des configs, pas ciblé par actif — limitation documentée) ; triggers câblés : `vulnerability_detected` + `risk_score_updated` (les triggers `risk_created`/`incident_created` sont définis, à émettre depuis leurs modules). |

| **14.20 Gouvernance = « 15. Gouvernance »** | ✅ (piste d'audit immuable via hooks ORM + délégations temporelles + moteur Maker-Checker configurable, prouvé live 22/07, branche `feature/governance-audit-workflows`) | **Pilier 1 (RBAC)** existait déjà (`domain/rbac.go` : rôles custom + permissions granulaires, `/rbac/*`, `RequirePermission` wildcard, `RBACTab`) — conservé. **Piliers 2-4 livrés** : **(4) Piste d'audit immuable** — `domain.AuditEvent` (append-only, tenant-scoped, Qui/Quoi/Quand/IP/**Avant→Après**) alimentée **automatiquement** par un **plugin GORM** (`internal/infrastructure/audittrail`) qui intercepte create/update/delete de tout modèle implémentant `domain.Auditable` (opt-in 1 ligne : Asset, ComplianceControl) — *les développeurs ne peuvent pas oublier de logguer* ; snapshots par json-marshal → les champs `json:"-"` (secrets) ne sont **jamais** capturés (RÈGLE #6) ; best-effort (un échec d'audit ne casse jamais l'écriture métier). Un middleware estampille l'acteur+IP sur le contexte de requête → l'acteur réel est attribué (le « Qui »). **(2) Délégations** — `domain.Delegation` (fenêtre `[start,end]`, `IsActiveAt`), Create/List/Revoke + `ResolveEffectivePermissions` (union des droits délégués actifs). **(3) Approbations Maker-Checker** — `domain.ApprovalWorkflow` (chaîne d'étapes ordonnées liée à `(entity_type, action)`) + `domain.ApprovalRequest` (state machine) : `DecideApprovalStep` impose le **four-eyes** (le demandeur ne peut approuver sa propre demande), l'éligibilité par rôle et par étape, l'absence de double-signature, la barrière min-approbations avant d'avancer ; steps snapshotés à la soumission. Endpoints `/governance/{audit-events[/export],delegations[/effective],workflows,approvals}` (audit + config workflow = admin ; délégations + décisions = membre authentifié, les use cases imposent four-eyes/rôle). Frontend `features/governance` : `/governance` à 4 onglets (journal d'audit interactif avec **diff avant→après** + export CSV, boîte de réception d'approbation avec chaîne d'étapes, délégations, constructeur de workflow). **Prouvé live 22/07** (Postgres:5434+Redis) : login → création workflow 2 étapes (201, doublon 409) → soumission (pending, step 0) → **demandeur approuve sa propre demande → 403 four-eyes** → soumission sans workflow → 400 → délégation create/effective-perms/revoke → auto-délégation 400 → **création d'actif → audit_event auto-capturé (action=create, actor=admin@opendefender.io, IP)** → délégations/soumission/révocation journalisées explicitement → export CSV. Tests : state machine complète (chaîne 2 étapes, four-eyes, rôle inéligible, reject, dual-control min-2 + refus double-signature, conflict, not-found), logique de délégation, plugin (create/update/delete + diff + redaction secret + drop tenant-less). | Le plugin capture les écritures **struct-form** (create/delete d'entités auditées + diff sur update struct, prouvé par le test widget) ; les `Updates(map)` (ex. update d'Asset) sont journalisés sans diff — les flux où le diff importe passent par le `Recorder` explicite. L'application effective de la décision approuvée (muter l'entité sous-jacente) reste au demandeur via le `payload` (prochaine itération : appliquer automatiquement). L'ancien `AdminAuditEvent` (14.9, jamais câblé) est **supplanté** par ce système. Frontend prouvé par tsc/vite + endpoints live (pilotage CDP interactif bloqué par le sandbox). |

### 1.3 Sécurité, Observabilité & Transversaux — Modules 15, 16, 17.1–17.7

| Module V5 | Statut | Preuves / absence | Note |
|---|---|---|---|
| **15. Sécurité & Hardening** | 🟡 | `middleware/ratelimit.go`, `middleware/security_hardening.go`, helmet dans `main.go`, tenant isolation dans repos. **Découverte critique `SetContext` corrigée (voir §2).** | CSP/HSTS/audit-log global, tests d'isolation systématiques, grep `fmt.Sprintf` SQL : à auditer/compléter. |
| **16. Observabilité** | 🟡 | `pkg/monitoring`, `monitoring_handler`, `MonitoringDashboard.tsx`, zerolog JSON. | Endpoint `/metrics` Prometheus, dashboards Grafana, health `/ready`/`/deep`, Request-ID distribué : à confirmer/compléter. |
| **17.1 Internationalisation (i18n)** | ✅ | `locales/fr.json`, `locales/en.json`, i18n FR/EN « solide ». | Certaines features récentes (Board Report) hardcodent le FR (marché primaire). |
| **17.2 Billing & Plans (Stripe/Mobile Money)** | ❌ | Rien. | Middleware `CheckPlanLimits`, Stripe, Wave/MTN/Orange. |
| **17.3 Feature Flags** | 🟡 | Claim `FeatureFlags` dans le JWT (`internal/auth/jwt.go`). | **Pas de table `feature_flags`, pas de middleware `FeatureFlag()`, pas d'admin.** Scaffolding claim seulement. |
| **17.4 Super Admin Panel** | ❌ | Aucun package superadmin. | Tenants/impersonation/metrics globales/équipe OpenDefender. |
| **17.5 Accessibilité (WCAG 2.1 AA)** | 🟡 | Balayage `axe-core` automatisé (`tests/e2e/a11y.spec.ts`) : 15 écrans principaux + les 5 routes du wizard d'onboarding et la checklist d'activation, porte à zéro violation *serious/critical*, `A11Y_KNOWN` vide. | Le balayage couvre les écrans principaux, pas encore toutes les modales ni tous les tiroirs ; parcours clavier et lecteur d'écran non audités de bout en bout. |
| **17.6 Onboarding Flow (5 étapes)** | ✅ | Wizard 5 étapes reprenable (`frontend/src/features/onboarding/wizard/`), garde de route `OnboardingGuard`, état serveur `internal/application/activation/onboarding.go` + `internal/domain/activation.go`, endpoints `/onboarding/*` et `/activation/state`. Checklist d'activation à 7 étapes, dérivée d'événements serveur ; amorcée pour les tenants déjà configurés (`gorm_activation_backfill.go`). | Promesse d'activation : **Aha en < 8 min** (assertion `tests/e2e/activation.spec.ts` ; l'alerte `SlowTimeToAha` à P50 > 12 min est un seuil d'exploitation, pas la promesse). Étapes « intégrations » et « première évaluation » : hors périmètre, voir #234. |
| **17.7 Sync Engine & Intégrations** | 🟡 | `infrastructure/integrations/thehive` + `SyncEngine` lancé dans `main.go`. | OpenCTI/Splunk/Elastic/AWS Security Hub/Azure Defender/Jira : ❌. |

### 1.4 Product Growth — Partie C (go-to-market)

| Élément V5 | Statut | Note |
|---|---|---|
| **Design System** (tokens, RiskBadge, ScoreMeter, ProgressBar, EmptyState, CommandPalette…) | 🟡 | `shared/components` partiels. `CommandPalette` (Cmd+K) non confirmé. |
| **Dark mode** | 🟡 | Présent mais non audité sur toutes les pages. |
| **Animations & micro-interactions** | 🟡 | Framer Motion sur Dashboard + transitions de route ; pas généralisé. |
| **Raccourcis clavier globaux** (N, M, Esc, Cmd+K…) | 🟡 | Partiels (ex. `N` sur Risks). |
| **Stratégie de conversion** (PlanLimitBanner, FeatureGateModal, UsageDashboard) | ❌ | Dépend du Billing (17.2). |
| **Stratégie de rétention** (Weekly Digest, streak, tips contextuels, re-engagement) | ❌ | Gamification partielle seulement. |
| **Page Pricing publique** | ❌ | — |
| **Landing page marketing** | ❌ | — |

---

## 2. DÉTAIL DES MODULES LIVRÉS & PROUVÉS LIVE

> Historique condensé de ce qui a réellement été construit et vérifié (le détail complet vit dans
> l'historique git et dans `CLAUDE.md`). Toutes ces branches sont **poussées mais non mergées** (voir §3).

### M1 — Compliance engine ✅ (07–08/07/2026, branche `feat/m1-compliance-engine`)
Use cases + handlers + OpenAPI + client généré + frontend + upload de preuve réel + RBAC granulaire.
**Vérifié live de bout en bout le 08/07** après correction de **11 bugs** (l'app n'avait jamais eu de
login fonctionnel dans cet environnement). Voir §2 « SetContext » ci-dessous.

### M2 — Contenu réglementaire africain ✅ (08/07/2026)
Catalogue générique + **93 contrôles ISO 27001:2022** importables en un clic, **vérifié live**. Puis
**BCEAO (35, Règlement 15/2002/CM/UEMOA), ANTIC-CM (25, Loi 2010/012), COBAC (45, R-2016/04)** cités
article par article (branche `fix/dashboard-crash-mitigation-routes-and-ui-polish`). `TestNoOrphanControls`
garantit code de référence + citation source uniques. 1 placeholder subsiste (`cm-loi-2024-017`).

### M3 — Assets ✅ (08/07/2026, branche `feat/m3-assets-inventory`)
Clean Architecture rétrofitée (le handler existant touchait `database.DB` sans use case ni RBAC),
snapshots historiques, criticité enfin branchée sur le Score Engine via le flux Redis
`asset.criticality_changed` (bug de scan varchar→float64 dans `GetRisksByAssetID` corrigé au passage).

### M3+ — Gestion centralisée des actifs : dépendances + taxonomie + historique UI ✅ (16/07/2026, branche `feat/asset-dependency-mapping`)
Réponse à la demande explicite « inventaire complet (serveurs/applications/cloud/données/utilisateurs/fournisseurs) +
classification par criticité + **cartographie des dépendances** + historique des modifications ». État avant : 1 & 2 & 4
existaient côté backend mais (a) la taxonomie ne couvrait pas données/utilisateurs/fournisseurs, (b) l'historique
n'était **atteignable depuis aucun écran live** (le `AssetHistoryDrawer` ne vivait que dans l'orphelin `AssetsPage`),
(c) la **cartographie des dépendances n'existait pas** : l'`Asset Universe` était un canvas **piloté par des fixtures**
(`UNI_NODES`/`UNI_LINKS`) badgé « Aperçu », sans aucun modèle backend d'arêtes.
- **(1) Cartographie des dépendances (le vrai manque).** Nouveau `domain.AssetDependency` — arête **dirigée**
  `SourceAsset → TargetAsset`, tenant-scoped, `DependencyType` (8 relations : depends_on / runs_on / connects_to /
  hosted_by / stores_data_in / authenticates_via / backs_up_to / managed_by), dans `AutoMigrate`. Port
  `AssetDependencyRepository` + impl Gorm (isolation tenant sur chaque query, `Exists` anti-doublon, `DeleteByAsset`
  pour le cascade). Use cases (1 fichier chacun) : `Create` (valide que **les deux** actifs existent dans le tenant —
  double garde cross-tenant —, refuse l'auto-référence et le doublon), `List` (graphe complet du tenant), `Delete`.
  Handler + routes `GET/POST /asset-dependencies`, `DELETE /asset-dependencies/:id` (montées **en sœurs** de `/assets`
  pour que « dependencies » ne soit jamais parsé comme UUID). `DeleteAssetUseCase.WithDependencyRepository(...)` prune
  les arêtes à la suppression d'un actif (option, garde le constructeur 1-arg et ses tests). OpenAPI + types régénérés.
- **(2) Inventaire — taxonomie étendue.** `ASSET_TYPES` (front) + enums OpenAPI (`Asset`/Create/Update) passent à
  **Server, Application, Cloud, Database, SaaS, Storage, Network, Laptop, Data, User, Supplier** — couvre les 6
  catégories demandées. Icônes + chips de filtre par type mis à jour (InventoryPage + panneau Universe).
- **(3) Historique enfin exposé.** Bouton « Historique » ajouté à `EditAssetModal` → ouvre le `AssetHistoryDrawer`
  (endpoint `/assets/:id/history` inchangé, déjà prouvé en M3) depuis l'`InventoryPage` live.
- **(4) `Asset Universe` rebranché sur le réel.** Fixtures supprimées ; nœuds = `/assets`, arêtes =
  `/asset-dependencies` ; physique in-house conservée (répulsion/ressorts/gravité/amortissement, 160 warm-up) ;
  panneau latéral = **éditeur de dépendances** (liste entrantes/sortantes + ajout `cible × relation` + retrait),
  gardé par `assets:update` ; états loading/empty honnêtes.
- **Tests** : use cases (Success/Defaults/SelfRef/TargetNotFound/CrossTenant/Duplicate + List/Delete Success/NotFound/
  CrossTenant) + repo Gorm sqlite (isolation tenant, Exists, GetByID cross-tenant=nil, DeleteByAsset, ListByAsset
  bidirectionnel). `go build`/`vet`/tests verts ; `tsc -b`/`vite build` verts.
- **Preuves live (16/07, Postgres:5434 + Redis, admin@opendefender.io)** : create dep 201, list 200, self-ref 400,
  doublon 409, cible absente 404, type invalide 400, `GET /assets/:id` **non masqué** par la route sœur (200),
  cascade (delete actif → arêtes = `[]`), PATCH actif → snapshot d'historique créé. **Frontend (Chrome headless,
  1600×940)** : `Asset Universe` rend **7 actifs · 7 liens** (graphe force-directed, couleurs par criticité, hub
  web-01) ; `Inventaire` affiche les 11 types avec icônes/badges corrects (User/Cloud/Storage/Supplier…).

### M3++ — Historique des actifs : le « qui » ✅ (17/07/2026, branche `feature/centralized-asset-management`)
Diagnostic « 1. Gestion centralisée des actifs » : inventaire polymorphe (table générique + type flexible, taxonomie
couvrant Matériel/Logiciel-Cloud/**Données/Utilisateurs/Fournisseurs**), classification par criticité (→ Score Engine),
**cartographie des dépendances** (graphe interactif + éditeur) = **déjà complets** (M3+). Seul écart réel vs la spec
(« qui a modifié quoi, et quand ») : l'`AssetSnapshot` n'enregistrait **pas l'auteur**. Comblé :
- **Domaine** : `AssetSnapshot.ChangedBy uuid.UUID` (persisté, `index`, nullable pour lignes legacy/système) +
  `ChangedByEmail string` (`gorm:"-"`, calculé en lecture). Migration `0032_add_asset_snapshot_changed_by`
  (AutoMigrate l'ajoute aussi ; le fichier garde un déploiement migrations-only autosuffisant).
- **Écriture** : `UpdateAssetUseCase.Execute` et `DeleteAssetUseCase.Execute` prennent un `changedBy uuid` et le
  posent sur le snapshot ; le handler passe `userID(c)` (le JWT ne porte que l'id, pas l'email).
- **Lecture** : `ListAssetSnapshotsUseCase` gagne un port **optionnel** `UserLookup` (`WithUserLookup`) qui résout
  chaque `ChangedBy` distinct/non-nil en email via `GormUserRepository.EmailsByIDs` (une requête `WHERE id IN`),
  câblé dans `main.go`. Dégradation gracieuse : lookup absent/en erreur → UUID brut, email vide, jamais d'échec.
- **Frontend** : `AssetHistoryDrawer` affiche « Modifié par {email|id court|Système} » à côté de l'horodatage ;
  i18n `assets.changedBy` / `assets.changedBySystem` (FR + EN). OpenAPI + types régénérés (contract-first).
- **Tests** : use cases update/delete assertent `ChangedBy` ; `ListAssetSnapshots` (résolution email + dédup +
  acteur nil ignoré + dégradation sur erreur) ; repo Gorm — DDL sqlite dérivé corrigé (`changed_by` ajouté) +
  aller-retour `changed_by` en base. `go build`/`vet`/tests verts, `tsc -b`/`vite build` verts.
- **Preuve live (17/07, Postgres:5434 + Redis)** : login→create actif→2×PATCH→`GET /assets/:id/history` = 2 snapshots
  newest-first, chacun `changed_by`=id admin réel **et** `changed_by_email`="admin@opendefender.io" (résolu live) ;
  delete 204→get 404. **Reste optionnel** : décomposition de la criticité en sous-scores DICT/CIA (la lettre de la
  spec — « un niveau de criticité … (ex: Faible/Moyenne/Haute/Critique) » — est déjà satisfaite par l'enum actuel).

### M4 — Reporting officiel + Board Report ✅ (09–10/07/2026, branche `feat/m4-compliance-report-pdf`)
- **Rapport de conformité officiel (PDF, 1 clic)** — vérifié live 09/07. `GET /compliance/frameworks/{id}/report?locale=fr|en`
  → PDF soigné tenant-scoped (garde + synthèse exécutive graduée + tableau paginé). `pkg/report` **pur** ;
  piège `fpdf.SplitText` (panique sur rune > 255 : tiret cadratin, ligature œ, apostrophe typographique)
  contourné par word-wrap maison `wrapText`. Preuve : rapport ISO 27001, 11 pages, accents FR + texte EN corrects.
- **Passe UX + gouvernance** (09–10/07, vérifiée via Chrome CDP) : 5 bugs UI, modals à footer épinglé,
  **suppression de framework** (admin-only), ISO nettoyé (198→93), **frameworks rendus tenant-scoped**
  (migration `0030` + backfill, isolation prouvée live).
- **Board Report mensuel IA/FCFA (14.12) ✅** — fait + prouvé live **10/07**. **1er client LLM du repo.**
  `pkg/ai` (interface `Advisor` + `TemplateAdvisor` déterministe testé + `ClaudeAdvisor` `claude-opus-4-8`
  adaptive thinking, cf. skill `claude-api`) → `application/board` (`GenerateBoardReportUseCase` agrège la
  posture **réelle tenant-scoped** : risques par criticité via `CountRisksByCriticality`, conformité par
  référentiel, **exposition FCFA** via `ExposureModel`, **fallback template si l'appel LLM échoue**) →
  `domain.BoardReport` (snapshot gelé + narration éditable + draft→approved, dans `AutoMigrate`) →
  `pkg/report/board_pdf.go` → handler `reports:board:*` → front React Query. **Preuve live** : login →
  génération (4 référentiels du tenant, 21,1 % global, 1 risque moyen → 3 000 000 FCFA, `created_by`
  renseigné) → édition → approbation → 400 si ré-édition → PDF inspecté en PNG. **Chemin `ClaudeAdvisor`
  non prouvé live faute d'`ANTHROPIC_API_KEY`** (repli template OK) — à revérifier dès qu'une clé existe.

### M12 — IA (spec §12) ✅ (19/07/2026, branche `feature/ai-integration`)
Second client LLM du projet, après le Board Report. **Diagnostic préalable** : `pkg/ai` existait mais n'exposait
qu'une méthode (`GenerateBoardNarrative`) ; la page chat `AiAdvisor.tsx` était **100 % mockée** (réponses par
mots-clés en dur), `AIRiskInsights.tsx` et `ai_risk_predictor_service.go` étaient des stubs mockés/non câblés.
Les 5 capacités de la spec étaient donc **absentes ou simulées**. **Livré** — **(a) Service unifié** `pkg/ai.Assistant`
(interface + DTOs purs sans import domain) : `ClaudeAssistant` (une méthode par capacité, `complete()` → Messages
API `claude-opus-4-8` adaptive thinking → parse JSON strict) + `TemplateAssistant` (repli déterministe : jamais de
faux « satisfies » sur une preuve, « non trouvé » honnête en Q&A sans contexte) + factory `NewAssistant` +
`IsLLMBacked`. **(b) App layer** `internal/application/ai` (ports étroits nil-safe, helper `invoke` = appel primaire +
repli template + provenance) : `SuggestTreatmentPlan` (risque + actif lié), `DetectEmergingRisks` (texte + dédup
titres existants), `AssistantQuery` (**RAG hybride** : `RiskQuery.Search` plein-texte tenant + mots-clés sur contrôles
+ `VulnerabilityQuery.Search` → snippets sourcés), `GenerateAuditReport` (audit + `GetGapAnalysisUseCase` +
remédiations ouvertes), `AnalyzeEvidence` (preuve → contrôle → référentiel). **(c) Handler + 6 routes** `/ai/*`
(`risks:read`/`compliance:read`, locale body/query défaut FR). **(d) Frontend** : `aiService.ts`/`useAi.ts` typés zéro
`any` ; chat `/recommendations` réécrit sur `/ai/assistant/query` (sources en chips, badge Claude/local, état
« analyse… ») ; onglet « IA » du drawer de risque (plan de traitement) ; page `/ai/emerging-risks` (+ nav + i18n) ;
`AiEvidenceAnalysis` inline sous chaque preuve ; `AiAuditReportButton` (modale + copier) sur `AuditsPage`. **Preuve
live (binaire :8098)** : `/ai/status` 200 (mode template — pas de clé) ; Q&A retrouve Log4Shell CVE-2021-44228 +
contrôles cités ; treatment-plan « Exposed admin panel on web-prod-01 » (ALE 20 M FCFA, mitigate, 3 actions) ;
emerging-risks 4 risques ; audit-report gap réel 326/274/16 % ; evidence-analysis « insufficient » (contenu non
extrait → confiance abaissée) ; 404 risque/audit/evidence inconnus, 400 texte vide/uuid invalide. `go build`/`vet`/
`go test ./pkg/ai/... ./internal/application/ai/...` verts (5 + 10 tests), `tsc -b`/`vite build` verts. **Restes honnêtes** :
chemin `ClaudeAssistant` non exercé live (pas de clé — compile + repli OK) ; analyse de preuve = métadonnées
(extraction contenu fichier = prochaine itération) ; pas de base vectorielle ni de streaming/cache ; frontend prouvé
par tsc/build + endpoints live (CDP interactif bloqué par le sandbox).

### ⚠️ Découverte critique du 08/07/2026 — `middleware.SetContext()`
`SetContext()` n'était appelé **nulle part en production** (seulement dans un harnais de test). Les 8 handlers
qui lisent `GetContext(c)` retombaient donc silencieusement sur `tenant_id = uuid.Nil`. Corrigé en une ligne
dans `AuthMiddlewareRS256`. **Toute affirmation antérieure au 08/07 sur le bon fonctionnement multi-tenant
de Risk/Asset/Mitigation/Dashboard/Compliance doit être revérifiée.** C'est la source n°1 des « faux ✅ ».

---

## 3. BRANCHES GIT OUVERTES (poussées, **non mergées dans master**) — décision PR/merge requise

- `feat/m3-assets-inventory` — M3 Assets.
- `fix/dashboard-crash-mitigation-routes-and-ui-polish` — crash dashboard + RequireRole + catalogues africains.
- `feat/africa-compliance-catalogs-and-responsive` — base de la branche M4.
- `feat/m4-compliance-report-pdf` — **branche courante**, rapport de conformité + Board Report + tenant-scoping.
  Poussée sur `origin` le 10/07 (commit `412656ff`).

Aucune n'est mergée dans `master`. **Demander avant tout merge/PR.**

---

## 4. PROCHAINES PRIORITÉS (ordonnées par valeur × dépendances)

**Bloc UX — REFONTE UX STRATÉGIQUE (sprint courant, 2026-07-23)** — *transformer OpenRisk en outil
intuitif et centré utilisateur : prise en main immédiate, zéro charge cognitive, zéro dead-end.*
Focus produit défini : **LE registre des risques et sa réduction** (identifier → scorer → traiter →
prouver) est la fonctionnalité-cœur ; tout le reste (actifs, vulns, conformité, IA, dashboards) orbite
autour. **Aha! moment** = « je crée/importe mon premier risque et je vois immédiatement son exposition
financière + un plan de traitement suggéré ». Une branche par phase, commits atomiques, doc + test à chaque étape.

- [x] **UX-0 — Socle : architecture de l'information & navigation par intentions** ✅ 2026-07-23
  (`feature/ux-information-architecture`) — sidebar regroupée en **7 espaces par intention** (⭐ *Maîtriser
  les risques* en tête et accentué · *Piloter la posture* avec Dashboard épinglé · *Cartographier le
  patrimoine* · *Anticiper les menaces* · *Prouver la conformité* · *Décider & rapporter* · *Administration*).
  `NavItem.pinned` + `NavGroup.core` + `pinnedItems()`, clés i18n `g_*` refondées, en-tête core accentué.
  Action première (« Nouveau risque ») déjà épinglée, `soon`→ComingSoon (CTA, pas de dead-end). `tsc`/`vite` verts.
- [x] **UX-1 — Universal Search ⌘K** ✅ 2026-07-23 (`feature/ux-universal-search`) — endpoint
  `GET /search?q=` tenant-scoped + **RBAC par source** (une source n'est cherchée que si le demandeur a
  sa permission) + best-effort (nil-safe), sur **les 8 entités de la spec** : risques · actifs · vulns ·
  **contrôles · audits · rapports · CVE · utilisateurs** (users admin-only, CVE = threat-intel global).
  Palette ⌘K = vraie recherche d'entités (groupe « Résultats » débouncé 180 ms, icône par type, chip de
  sévérité). **Deep-open `?focus=<id>`** câblé sur risques/actifs/vulns (hook `useFocusParam`). **Preuves
  live** : `q=admin` → risque + 2 contrôles + 6 CVE + user `System Administrator` ; `q=log` → risque +
  actif + 4 vulns + 6 contrôles ; **capture headless** `/risks?focus=<id>` → drawer Log4j ouvert directement.
  7 tests use-case verts, `go vet`/`tsc`/`vite` verts.
- [x] **UX-2 — Dashboards intelligents par rôle** ✅ 2026-07-23 (`feature/ux-role-dashboards`) — le dashboard
  `/` s'adapte au `business_role` via un dispatcher (`dashboardPersona.ts`) → **6 personas** à données réelles :
  **posture** (RSSI/risk/admin, layout existant) · **analyst** (vulns : KPI P1/KEV + file de priorité
  deep-linkée) · **audit** (couverture par référentiel + écarts + audits) · **exec** (cyber score A–F + ALE
  FCFA + KRI) · **estate** (inventaire + criticité + actifs critiques deep-linkés) · **viewer** (aperçu
  lecture seule). Primitives partagées (`shared.tsx`). **Preuves headless** : Direction→F/117,5 M FCFA/7 KRI ;
  Auditeur→6 réf./ISO 46 %/180 écarts ; DSI→16 actifs/7 crit. ; Analyste→file P1/KEV. `tsc`/`vite` verts.
- [x] **UX-3 — Psychologie des interactions** ✅ 2026-07-23 (`feature/ux-inline-edit-autosave`) — **édition
  fantôme** (statut click-to-edit inline, autosave optimiste + toast, zéro bouton Enregistrer) sur risques
  & vulns & mitigations ; **soft-delete + undo** via hook réutilisable `shared/useSoftDelete` (la ligne
  disparaît + toast « Annuler » 5 s, l'API ne part qu'après → undo instantané) sur risques, vulns,
  incidents. Les suppressions **vitales** (tenant/user/rôle/token) gardent un confirm explicite (→ UX-4
  radiographie). `tsc`/`vite` verts, affordance inline prouvée live (risks + vulns).
- [x] **UX-4 — Actions critiques & erreurs** ✅ 2026-07-23 (`feature/ux-critical-actions`) — composant
  réutilisable **`shared/DangerConfirm`** (radiographie d'impact : conséquence + bilan label→valeur +
  **alternatives en 1re classe** + action destructive) sur les suppressions **vitales** live : révocation
  de membre (alternative « Désactiver — réversible »), révocation de jeton API (avait **zéro friction**),
  suppression de référentiel (cascade contrôles+preuves, montre nb contrôles/couverture). Messages d'erreur
  enrichis (« réessayez / contactez un admin »). **Empty states** des listes cœur (risques/vulns/actifs/
  incidents) **déjà** dotés d'un CTA actionnable (vérifié, pas de dead-end). Prouvé live (modale rendue sur
  /settings). `tsc`/`vite` verts.
- [x] **UX-5 — Onboarding & Aha moment** ✅ 2026-07-23 (`feature/ux-onboarding-aha`) — **onboarding par
  l'action** (`OnboardingChecklist` sur le dashboard) : étapes auto-cochées depuis les vraies données vers
  l'Aha (⭐ créer son 1er risque → exposition+traitement), puis actif + référentiel ; barre de progression
  honnête, actions 1-clic, dismissible (un tenant configuré ne la voit jamais). **Personnalisation
  post-victoire** débloquée après l'Aha (`PersonalizeCard` : thème clair/sombre + accent azure/iris, déjà
  branchés tokens) — aussi dans Paramètres › Apparence. **Aide contextuelle** (`shared/InfoHint`, tooltip
  progressive, pas de product tour) sur les jauges de score. Prouvé live (état new-tenant). `tsc`/`vite` verts.
- [x] **UX-6 — Notifications, rétention & upsell** ✅ 2026-07-23 (`feature/ux-notifications-upsell`) —
  **notifications catégorisées** : taxonomie `shared/notificationCategory.ts` (5 contextes Sécurité/
  Conformité/Tâches/Collaboration/Facturation + mapping `type→catégorie`) → **matrice de préférences par
  contexte** dans Paramètres › Notifications (switches In-app + E-mail par contexte, persistés localStorage)
  + chips de catégorie & filtre sur la cloche. **Upsell doux** : `shared/UpsellLock` (aperçu **flou** +
  bénéfice + CTA, jamais de mur dur ; label « moment ») appliqué au Classement (gamification premium).
  Prouvé live (matrice 5 contextes + overlay PREMIUM flouté). `tsc`/`vite` verts. **Honnête** : pas de
  backend billing → gate cosmétique (vrai gating = module Billing, Bloc D) ; persistance prefs = localStorage.
- [x] **UX-7 — Ergonomie & accessibilité (passe finale)** ✅ 2026-07-23 (`feature/ux-a11y-responsive`) —
  **raccourcis clavier globaux** (`GlobalShortcuts` monté dans le shell : `N` nouveau risque · `/` recherche ·
  `G` puis D/R/V/M/I/C/A/S = aller à · `?` aide · Esc) qui ne détournent jamais la frappe dans un champ, +
  **overlay d'aide** listant tout. Responsive (sidebar off-canvas < lg vérifiée à 414 px, tables overflow-x),
  no-dead-end (empty states + ComingSoon CTA, UX-4) et hiérarchie visuelle **déjà en place**. Prouvé live
  (overlay raccourcis + capture mobile). `tsc`/`vite` verts. **🎉 Les 8 phases de la refonte UX sont livrées.**

**Bloc A — Solidifier les fondations (avant d'empiler des features)**
1. **Prouver live l'Auth complète (Module 2)** : MFA, OAuth2, SAML2, refresh-token, switch-org. Corriger
   `TestSetupMFA_Success`/`TestRiskCRUDFlow`. Tant que non prouvé → 🟡.
2. **Câbler le SSE (Module 7)** : hub dédié + `/api/v1/stream` + route `/risks/events` réelle, sinon la
   plupart des « real-time » des autres modules restent des maquettes.
3. **`created_by` réel sur `CreateRisk`** + implémenter `/analytics/security-score` & `/analytics/assets/statistics`
   (widgets en repli gracieux aujourd'hui).
4. ~~**Incident Management (14.1)** : ajouter la table à `AutoMigrate` → premier module avancé *fonctionnel*.~~
   ✅ **fait le 13/07/2026** (branche `feat/ui-redesign-dc-html`) — tables migrées + 3 bugs corrigés + registre live
   (drawer détail/édition, statut inline, export CSV) + War Room câblée sur incident réel (chronologie + clôture).
   **Reste (dette legacy) :** `RiskID *uint` ↔ risques uuid (LinkRisk cassé), collaboration War Room = fixtures,
   pas de retrofit Clean Architecture du service.

**Bloc B — Différenciateurs à fort levier (le moat V5, faible dépendance infra)**
5. **IA Advisor complet (Module 10)** : réutiliser `pkg/ai` (déjà branché sur Claude) pour analyze/mitigations/
   prioritize/narrative + `AIAdvisorTab`. Bloc le plus rentable car la fondation existe déjà.
6. ~~**CRQ FAIR (14.5)** : `pkg/crq` (ALE/ARO/SLE)~~ ✅ **FAIT 17/07** (branche `feature/financial-risk-quantification`) — moteur complet (SLE composé, downtime, pertes max/moy PERT, ROSI) + dashboard financier CFO/CISO + simulateur d'investissement. Voir tableau 14.5.
7. **Reporting complet (Module 11)** : templates officiels COBAC/BCEAO/ISO/PCI + export XLSX + jobs async.
8. **PAM Audit Trail (14.9)** : finir l'append-only (trigger PG) + anomaly detector + `AdminAuditPage`.

**Bloc C — Gros chantiers infra (déblocage en cascade)**
9. **Infrastructure Scanner + Agent (Module 6)** : débloque l'auto-mitigation (4.1), le matching CVE des
   Assets (13), la Data Discovery (14.11), l'Attack Path (14.15).
10. **CTI Engine câblé (Module 5)** : worker de sync + auto-création de risque.

**Bloc D — Monétisation & croissance (Partie C + 17)**
11. **Billing & Plans (17.2)** + conversion (Partie C) + **Onboarding (17.6)** + **Super Admin (17.4)**.

**Bloc W1 — Wave 1, fondations produit**
- [x] **W1-03 — Action Center contextuel** (épique #201, scindée en #429 backend / #430 frontend).
  **Backend livré** (#429, branche `429-feat-build-the-action-center-aggregation-api-backend`) :
  `GET /api/v1/action-center` — agrégation **en lecture seule** de six sources existantes (mitigation en
  retard · risque critique sans mitigation active · approbation dont l'appelant est l'approbateur de
  l'étape courante · incident ouvert · preuve expirée/expirante · plan de remédiation en retard), tri
  déterministe (rang de catégorie 1→6 puis clé secondaire propre à chaque catégorie), périmètre par
  `BusinessRole`, `tenant_id` filtré sur **chacune** des six requêtes (test de mutation à l'appui).
  Aucune table nouvelle, aucune migration. `internal/application/actioncenter/`,
  `internal/infrastructure/repository/actioncenter_repository.go`,
  `internal/handler/action_center_handler.go`. Contrat publié dans `docs/openapi.yaml` +
  `frontend/src/types/openapi.generated.ts`.
  **Frontend livré** (#430, PR #432 puis **PR #434**) : panneau « Centre d'actions » monté sur le
  tableau de bord (`/`) pour **toutes** les personas — `DashboardShell` pour les cinq personas dédiées,
  `PostureDashboard` qui n'utilise pas ce shell le monte lui-même. `frontend/src/features/action-center/`
  (`actionCenterService.ts`, `useActionItems.ts`, `actionLinks.ts`, `ActionCenterPanel.tsx`). Types importés
  de `openapi.generated.ts`, **jamais** saisis à la main ; zéro `any`. Ordre serveur rendu tel quel (aucun
  tri client). Trois états explicites (skeleton / erreur / vide, le vide se lisant comme un « tout est à
  jour » et non comme un échec). Chaque ligne est une vraie ancre : `deep_link` est validé contre
  `shared/routeModel.ts` avant d'être rendu, et un lien qui ne résout pas — ou qui quitte l'origine
  (`https://`, `//host`, `javascript:`) — fait **supprimer** l'élément avec un warning, jamais afficher une
  ligne morte. FR/EN dans `src/locales`. 14 tests (`__tests__/actionCenter.test.tsx`), dont axe-core sur le
  panneau rempli et vide.
  **Atteignable sur `master` depuis le 2026-08-31** : backend mergé par PR #431 (07:54Z), frontend par
  **PR #434** (08:15Z). PR #432 avait été mergée dans la branche `429-*` — déjà absorbée par `master` — et
  laissait donc l'UI sur une branche morte ; #434 est la reprise, re-vérifiée contre le `master` courant
  (`tsc` propre, eslint propre, 367/367, build OK, budget 245,7/250 Ko, `check:tokens` 164 paires 0 échec).
  **Reste à faire**
  - Ligne W1-03 dans `docs/MARKETING_CLAIM_MATRIX.md` : **non ajoutée**. Le fichier réserve explicitement
    le statut `VERIFIED` à l'agent `product-verifier` ; la ligne doit venir de `/verify-claims`, pas d'une
    saisie manuelle. C'est le dernier élément de la DoD de #201.
  - ~~Route dédiée `/action-center`~~ — **livrée par #433** : page pleine à `/action-center`, entrée
    `routeModel` (parent `/`, donc fil d'Ariane et retour), entrée sidebar, pagination serveur
    (`limit`/`offset`) dont le numéro de page vit dans l'URL (`?page=`), donc partageable et compatible
    avec le bouton Retour. Le lien « Tout voir » du panneau remplace le texte mort « N autres en attente ».
    Une seule implémentation de ligne (`ActionItemRow.tsx`) partagée par le panneau et la page : c'est ce
    qui empêche une deuxième surface de contourner la validation des `deep_link`. 22 tests
    (`__tests__/actionCenterPage.test.tsx`), axe compris.
  - Jamais exercé contre un backend vivant : tous les tests bouchonnent à la frontière du service. Le
    panneau n'a pas été rendu contre un `GET /api/v1/action-center` en fonctionnement, et la passe axe
    Playwright sur `/` avec des éléments présents n'a pas été faite.

**Bloc E — Wave 2/3 (gamechangers restants)**
12. Digital Twin (14.13), War Room (14.14), Attack Path (14.15), Access Review (14.10), Offline (14.17),
    Plugin Marketplace complet (14.18), Vendor (14.2), Policy (14.3), Trust Center (14.4), BCP (14.6),
    Training (14.7), Champions complet (14.16).

---

## 5. RÈGLES D'EXÉCUTION (Master Prompt V5 — Partie A, non négociables)

**Sécurité (critique)**
1. Filtrer par `tenant_id` sur **chaque** query DB — dans le repository, jamais dans le handler.
2. Objet d'un autre tenant → **404** (jamais 403).
3. JWT **RS256** uniquement. Jamais de secrets dans les logs. Credentials **AES-256-GCM** en DB.
4. `admin_audit_events` **APPEND-ONLY** (trigger PG rejetant UPDATE/DELETE) — jamais violé, même en migration.

**Architecture**
5. Erreurs typées uniquement (`ErrNotFound`/`ErrForbidden`/`ErrConflict`/`ErrValidation`).
6. Transactions DB sur toute opération multi-table.
7. Le Score Engine n'est **jamais** appelé depuis un handler — toujours via event Redis.
8. **Lire TOUS les fichiers existants d'un module avant d'écrire une ligne.**

**Qualité & UX**
9. Zéro `any` TypeScript. Zod sur tous les formulaires. Tests min. par use case (Success/NotFound/Unauthorized).
10. Skeleton loaders (jamais de spinner pleine page). Toujours les 3 états (loading/error/empty).
    Optimistic updates sur les mutations critiques.

**Méthode (5 étapes/module)** : LIRE → PLANIFIER → IMPLÉMENTER (backend puis frontend, tests inclus) →
VALIDER (`go test ./...` + `npm test` + live) → COMMITER. **Règle des 2 h** : bloqué > 2 h → nouvelle
session, commencer par « Lis [fichier] et explique-moi le problème avant de proposer une solution ».

**Discipline branche/doc** : une branche par feature ; à chaque fin de module, commit + mise à jour de
`ROADMAP.md` et `CLAUDE.md`. Vérifier chaque page **live** avant de la déclarer faite.
continue