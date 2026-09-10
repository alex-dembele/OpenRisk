// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package onboarding

import "sort"

// ---------------------------------------------------------------------------
// Starter risk catalogue — step 2 of the guided tunnel (#438).
//
// Deliberately NOT an extension of RiskSuggestion. Three things differ and each
// one matters:
//
//   - It is BILINGUAL. RiskSuggestion carries French-only Title/Description, and
//     widening it would touch every existing consumer for no benefit to them.
//   - It is scoped by sector AND country, not sector alone.
//   - StarterRisksFor always returns EXACTLY EightStarterRisks, because step 2's
//     screen shows eight cards and asks the user to pick three. A screen whose
//     card count depends on the sector is a screen that reflows.
//
// These statements become REAL ROWS in a customer's register, written with
// domain.SourceStarter (D-012). That is why the copy is deliberately generic
// about the organisation and specific about the mechanism: a statement the user
// must edit before it is true would poison the register it lands in.
//
// Nothing here is a compliance claim. The sector scoping is an activation
// device — it must never be worded as "your regulator requires this".
// ---------------------------------------------------------------------------

// EightStarterRisks is the size of the set step 2 renders. Named because the
// number is a UI contract, not an arbitrary truncation.
const EightStarterRisks = 8

// StarterRisk is one pre-written risk statement the user may adopt.
//
// Probability is on [0,1] and Impact on [0,10] — the Score Engine's own scales,
// so the number the user sees in step 4's matrix is the number that gets scored.
type StarterRisk struct {
	Key             string            `json:"key"`
	TitleI18n       map[string]string `json:"title_i18n"`
	DescriptionI18n map[string]string `json:"description_i18n"`
	Probability     float64           `json:"probability"`
	Impact          float64           `json:"impact"`
	Category        string            `json:"category"`
	Tags            []string          `json:"tags,omitempty"`
	// Scope says where this statement came from, so the screen can group
	// "typical of your sector" above "applies to any organisation" instead of
	// showing eight equally weighted cards in storage order.
	Scope StarterScope `json:"scope"`
}

// StarterScope is why a statement is in the set.
type StarterScope string

const (
	ScopeSector  StarterScope = "sector"
	ScopeRegion  StarterScope = "region"
	ScopeGeneric StarterScope = "generic"
)

// Title returns the statement in the requested language, falling back to French
// then to any available string. Never empty for a catalogue entry.
func (s StarterRisk) Title(lang string) string { return pick(s.TitleI18n, lang) }

// Description returns the body in the requested language, same fallback.
func (s StarterRisk) Description(lang string) string { return pick(s.DescriptionI18n, lang) }

func pick(m map[string]string, lang string) string {
	if v, ok := m[lang]; ok && v != "" {
		return v
	}
	if v, ok := m["fr"]; ok && v != "" {
		return v
	}
	for _, v := range m {
		if v != "" {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Generic pool — true of any organisation with a network and employees.
// Six entries, so the padding below can always reach eight.
// ---------------------------------------------------------------------------

var starterGeneric = []StarterRisk{
	{
		Key: "starter_admin_credentials",
		TitleI18n: map[string]string{
			"fr": "Compromission d'un compte à privilèges",
			"en": "Compromise of a privileged account",
		},
		DescriptionI18n: map[string]string{
			"fr": "Un compte administrateur est compromis par hameçonnage, mot de passe réutilisé ou absence de second facteur, ouvrant un accès étendu au système d'information.",
			"en": "An administrator account is compromised through phishing, a reused password or a missing second factor, opening broad access to the information system.",
		},
		Probability: 0.35, Impact: 8.5, Category: "access_control",
		Tags: []string{"identity", "privilege"},
	},
	{
		Key: "starter_ransomware",
		TitleI18n: map[string]string{
			"fr": "Chiffrement des systèmes par rançongiciel",
			"en": "Systems encrypted by ransomware",
		},
		DescriptionI18n: map[string]string{
			"fr": "Un rançongiciel chiffre les serveurs de production et les sauvegardes accessibles depuis le réseau, interrompant l'activité jusqu'à une restauration complète.",
			"en": "Ransomware encrypts production servers and any backups reachable from the network, halting operations until a full restore completes.",
		},
		Probability: 0.30, Impact: 9.5, Category: "availability",
		Tags: []string{"ransomware", "continuity"},
	},
	{
		Key: "starter_customer_data_leak",
		TitleI18n: map[string]string{
			"fr": "Fuite de données personnelles de clients",
			"en": "Leak of customer personal data",
		},
		DescriptionI18n: map[string]string{
			"fr": "Des données personnelles de clients sont exposées ou exfiltrées, déclenchant une obligation de notification et une atteinte durable à la réputation.",
			"en": "Customer personal data is exposed or exfiltrated, triggering a notification obligation and lasting reputational damage.",
		},
		Probability: 0.30, Impact: 9.0, Category: "data_protection",
		Tags: []string{"personal data", "confidentiality"},
	},
	{
		Key: "starter_third_party_outage",
		TitleI18n: map[string]string{
			"fr": "Défaillance d'un prestataire critique",
			"en": "Failure of a critical third party",
		},
		DescriptionI18n: map[string]string{
			"fr": "Un fournisseur dont dépend un service essentiel devient indisponible ou est compromis, sans solution de repli immédiate.",
			"en": "A supplier an essential service depends on becomes unavailable or is compromised, with no immediate fallback.",
		},
		Probability: 0.25, Impact: 7.5, Category: "third_party",
		Tags: []string{"supplier", "dependency"},
	},
	{
		Key: "starter_departing_employee",
		TitleI18n: map[string]string{
			"fr": "Accès maintenu après un départ",
			"en": "Access left active after a departure",
		},
		DescriptionI18n: map[string]string{
			"fr": "Les accès d'une personne ayant quitté l'organisation restent actifs faute de procédure de sortie appliquée, laissant une porte ouverte que personne ne surveille.",
			"en": "A departed person's access stays active because the offboarding procedure was not applied, leaving an open door nobody is watching.",
		},
		Probability: 0.40, Impact: 6.0, Category: "access_control",
		Tags: []string{"offboarding", "identity"},
	},
	{
		Key: "starter_backup_never_restored",
		TitleI18n: map[string]string{
			"fr": "Sauvegardes jamais testées en restauration",
			"en": "Backups never tested by restoring them",
		},
		DescriptionI18n: map[string]string{
			"fr": "Les sauvegardes existent mais aucune restauration complète n'a été éprouvée : leur validité n'est découverte qu'au moment où l'on en a besoin.",
			"en": "Backups exist but no full restore has ever been exercised: whether they work is discovered at the moment they are needed.",
		},
		Probability: 0.45, Impact: 8.0, Category: "availability",
		Tags: []string{"backup", "continuity"},
	},
}

// ---------------------------------------------------------------------------
// Sector pool — three per sector, so sector ∪ region ∪ generic ≥ 8 always.
// ---------------------------------------------------------------------------

var starterBySector = map[string][]StarterRisk{
	"banking": {
		st("starter_core_banking_outage",
			"Indisponibilité du système bancaire central", "Core banking system outage",
			"Le core banking devient indisponible : opérations clients bloquées, obligation de déclaration au superviseur et exposition à des pénalités.",
			"The core banking platform becomes unavailable: customer operations stop, the supervisor must be notified and penalties become possible.",
			0.20, 9.5, "availability", "core banking", "continuity"),
		st("starter_fraudulent_transfer",
			"Virement frauduleux exécuté", "Fraudulent transfer executed",
			"Un ordre de virement frauduleux est exécuté après usurpation du donneur d'ordre ou compromission d'un poste, entraînant une perte financière directe.",
			"A fraudulent transfer order is executed after impersonation of the originator or compromise of a workstation, causing a direct financial loss.",
			0.30, 8.0, "fraud", "fraud", "payment"),
		st("starter_aml_screening_gap",
			"Défaut de filtrage sur une relation d'affaires", "Screening gap on a business relationship",
			"Une relation d'affaires entre en portefeuille sans que les contrôles d'entrée en relation aient été complets, exposant à une remédiation coûteuse.",
			"A business relationship is onboarded without the entry controls having been completed, exposing the firm to costly remediation.",
			0.25, 7.5, "compliance", "onboarding", "screening"),
	},
	"insurance": {
		st("starter_claims_platform_outage",
			"Indisponibilité de la plateforme de sinistres", "Claims platform outage",
			"La plateforme de gestion des sinistres est indisponible : les déclarations s'accumulent et les délais contractuels de traitement sont dépassés.",
			"The claims platform is unavailable: filings pile up and contractual handling deadlines are missed.",
			0.25, 8.0, "availability", "claims", "continuity"),
		st("starter_actuarial_data_integrity",
			"Altération des données de tarification", "Corruption of pricing data",
			"Les données alimentant la tarification sont altérées ou incomplètes, produisant des primes fausses sur un portefeuille entier avant détection.",
			"Data feeding pricing is corrupted or incomplete, producing wrong premiums across a whole portfolio before anyone notices.",
			0.20, 8.5, "data_integrity", "pricing", "data quality"),
		st("starter_broker_portal_exposure",
			"Exposition du portail courtiers", "Broker portal exposure",
			"Le portail ouvert aux courtiers expose, par un défaut de cloisonnement, des dossiers appartenant à d'autres intermédiaires.",
			"The broker-facing portal exposes, through a segregation flaw, files belonging to other intermediaries.",
			0.20, 8.0, "data_protection", "portal", "segregation"),
	},
	"telecom": {
		st("starter_network_core_outage",
			"Panne du cœur de réseau", "Core network outage",
			"Une défaillance du cœur de réseau interrompt le service pour une région entière, avec des engagements de qualité de service à honorer.",
			"A core network failure interrupts service for an entire region, against quality-of-service commitments that still stand.",
			0.20, 9.5, "availability", "network", "continuity"),
		st("starter_sim_swap",
			"Détournement de carte SIM", "SIM swap fraud",
			"Un attaquant obtient le transfert d'un numéro vers une carte qu'il contrôle et intercepte les codes d'authentification de l'abonné.",
			"An attacker has a number transferred to a card they control and intercepts the subscriber's authentication codes.",
			0.35, 7.5, "fraud", "sim", "identity"),
		st("starter_cdr_retention",
			"Conservation excessive des données de trafic", "Excessive retention of traffic data",
			"Les données de trafic sont conservées au-delà de la durée nécessaire, augmentant la surface exposée en cas d'intrusion.",
			"Traffic data is kept beyond the necessary period, enlarging the surface exposed if an intrusion occurs.",
			0.35, 6.5, "data_protection", "retention", "privacy"),
	},
	"health": {
		st("starter_patient_record_exposure",
			"Exposition de dossiers patients", "Exposure of patient records",
			"Des dossiers médicaux sont accessibles à des personnes non habilitées, faute de cloisonnement par service ou par motif de consultation.",
			"Medical records are reachable by people who should not see them, for want of segregation by department or by reason for access.",
			0.30, 9.5, "data_protection", "health data", "confidentiality"),
		st("starter_biomedical_device_offline",
			"Indisponibilité d'un équipement biomédical connecté", "Connected biomedical device unavailable",
			"Un équipement biomédical connecté devient inutilisable après une panne réseau ou une mise à jour, interrompant une activité de soins.",
			"A connected biomedical device becomes unusable after a network failure or an update, interrupting clinical activity.",
			0.25, 9.0, "availability", "medical device", "continuity"),
		st("starter_shared_clinical_account",
			"Comptes cliniques partagés", "Shared clinical accounts",
			"Un compte est partagé au sein d'un service : aucune action n'est imputable à une personne, ce qui rend toute investigation impossible.",
			"An account is shared inside a department: no action is attributable to a person, which makes any investigation impossible.",
			0.45, 7.0, "access_control", "traceability", "identity"),
	},
	"public": {
		st("starter_citizen_service_outage",
			"Interruption d'un téléservice", "Citizen service outage",
			"Un téléservice devient indisponible pendant une période de forte affluence, sans procédure de repli pour les usagers.",
			"An online public service becomes unavailable during a peak period, with no fallback procedure for citizens.",
			0.30, 8.0, "availability", "public service", "continuity"),
		st("starter_records_tampering",
			"Altération d'un registre administratif", "Tampering with an administrative record",
			"Une donnée d'un registre administratif est modifiée sans trace exploitable, rendant la version faisant foi indéterminable.",
			"A record in an administrative register is altered with no usable trace, making the authoritative version impossible to establish.",
			0.20, 8.5, "data_integrity", "records", "traceability"),
		st("starter_legacy_unpatched",
			"Application héritée sans correctifs", "Unpatched legacy application",
			"Une application héritée reste en production sans correctifs disponibles, son remplacement étant reporté d'exercice en exercice.",
			"A legacy application stays in production with no patches available, its replacement deferred from one budget year to the next.",
			0.45, 7.5, "vulnerability", "legacy", "patching"),
	},
	"energy": {
		st("starter_ot_it_bridge",
			"Passerelle non maîtrisée entre bureautique et industriel", "Uncontrolled bridge between office and industrial networks",
			"Une liaison non maîtrisée entre le réseau bureautique et le réseau industriel permet à une compromission bureautique d'atteindre la conduite.",
			"An uncontrolled link between the office network and the industrial network lets an office compromise reach operational control.",
			0.25, 9.5, "network", "OT", "segregation"),
		st("starter_scada_credentials",
			"Identifiants par défaut sur un équipement de conduite", "Default credentials on a control device",
			"Un équipement de conduite conserve ses identifiants d'usine, documentés publiquement par le constructeur.",
			"A control device keeps its factory credentials, publicly documented by the manufacturer.",
			0.35, 9.0, "access_control", "SCADA", "credentials"),
		st("starter_grid_data_loss",
			"Perte des données de comptage", "Loss of metering data",
			"Les données de comptage d'une période sont perdues, empêchant la facturation et la reconstitution des consommations.",
			"Metering data for a period is lost, preventing billing and any reconstruction of consumption.",
			0.20, 7.5, "data_integrity", "metering", "billing"),
	},
	"retail": {
		st("starter_pos_compromise",
			"Compromission des terminaux de paiement", "Point-of-sale compromise",
			"Les terminaux de paiement en magasin sont compromis et les données de carte capturées pendant plusieurs semaines avant détection.",
			"In-store payment terminals are compromised and card data is captured for weeks before detection.",
			0.25, 9.0, "fraud", "payment", "PCI"),
		st("starter_peak_traffic_outage",
			"Effondrement du site en période de pointe", "Storefront collapse under peak traffic",
			"Le site de vente s'effondre lors d'un pic commercial : le chiffre d'affaires de la journée est perdu et ne se rattrape pas.",
			"The storefront collapses during a commercial peak: the day's revenue is lost and is not recovered later.",
			0.35, 8.0, "availability", "peak", "revenue"),
		st("starter_supplier_data_sharing",
			"Partage excessif de données avec un prestataire", "Excessive data sharing with a provider",
			"Un prestataire marketing reçoit plus de données clients que sa mission ne le justifie, sans encadrement contractuel correspondant.",
			"A marketing provider receives more customer data than its mandate justifies, with no matching contractual framing.",
			0.40, 6.5, "third_party", "supplier", "minimisation"),
	},
	"tech": {
		st("starter_secret_in_repository",
			"Secret exposé dans un dépôt de code", "Secret exposed in a code repository",
			"Une clé d'API ou un identifiant de production est commité dans un dépôt et reste exploitable dans l'historique après suppression.",
			"An API key or production credential is committed to a repository and stays usable in the history after deletion.",
			0.45, 8.5, "access_control", "secrets", "source code"),
		st("starter_tenant_isolation_flaw",
			"Défaut d'isolation entre clients", "Tenant isolation flaw",
			"Un défaut d'isolation permet à un client d'atteindre les données d'un autre, ce qui met en cause le produit lui-même et non une donnée isolée.",
			"An isolation flaw lets one customer reach another's data, which calls the product itself into question rather than one record.",
			0.20, 9.5, "data_protection", "multi-tenant", "isolation"),
		st("starter_dependency_supply_chain",
			"Dépendance tierce compromise", "Compromised third-party dependency",
			"Une dépendance open source intégrée au produit est compromise en amont et distribuée à tous les clients avec la version suivante.",
			"An open-source dependency built into the product is compromised upstream and shipped to every customer with the next release.",
			0.20, 9.0, "third_party", "supply chain", "dependency"),
	},
	"industry": {
		st("starter_production_line_halt",
			"Arrêt d'une ligne de production", "Production line halt",
			"Une ligne de production s'arrête sur défaillance du système de pilotage, avec un coût par heure d'arrêt et des engagements de livraison à tenir.",
			"A production line stops on a control-system failure, at a cost per hour of downtime and against delivery commitments that still stand.",
			0.25, 9.0, "availability", "production", "continuity"),
		st("starter_warehouse_stock_integrity",
			"Écart entre stock physique et stock système", "Gap between physical and system stock",
			"Le stock enregistré diverge du stock réel sans réconciliation, faussant les engagements commerciaux et les approvisionnements.",
			"Recorded stock diverges from actual stock with no reconciliation, distorting both commercial commitments and replenishment.",
			0.40, 6.5, "data_integrity", "stock", "logistics"),
		st("starter_design_theft",
			"Exfiltration de plans industriels", "Exfiltration of industrial designs",
			"Des plans ou spécifications de fabrication sont exfiltrés, transférant un avantage concurrentiel construit sur plusieurs années.",
			"Manufacturing designs or specifications are exfiltrated, transferring a competitive advantage built over years.",
			0.20, 8.5, "data_protection", "intellectual property", "confidentiality"),
	},
	"other": {
		st("starter_no_asset_inventory",
			"Absence d'inventaire des actifs", "No asset inventory",
			"Aucun inventaire à jour des systèmes et des données n'existe : la surface à protéger n'est pas connue, donc pas protégeable.",
			"No up-to-date inventory of systems and data exists: the surface to protect is not known, so it cannot be protected.",
			0.50, 7.0, "governance", "inventory", "visibility"),
		st("starter_no_incident_procedure",
			"Absence de procédure d'incident", "No incident procedure",
			"Aucune procédure de réponse n'est écrite : le premier incident se gère par improvisation, au moment le plus coûteux pour improviser.",
			"No response procedure is written down: the first incident is handled by improvisation, at the most expensive possible moment to improvise.",
			0.45, 7.5, "governance", "incident", "readiness"),
		st("starter_shadow_it",
			"Outils non validés en usage courant", "Unvetted tools in everyday use",
			"Des outils non validés traitent des données de l'organisation, hors de toute revue de sécurité et de tout contrat.",
			"Unvetted tools process organisational data, outside any security review and any contract.",
			0.50, 6.5, "governance", "shadow IT", "third_party"),
	},
}

// ---------------------------------------------------------------------------
// Region pool — two per regulatory region, keyed to the markets OpenRisk serves.
//
// These are worded as OPERATIONAL exposures, never as "your regulator requires
// X". The sector and country scoping is an activation device; the compliance
// claim belongs to the framework catalogue and to nothing else (#438).
// ---------------------------------------------------------------------------

var starterByRegion = map[string][]StarterRisk{
	"cemac_uemoa": {
		st("starter_supervisor_reporting_delay",
			"Retard dans un reporting au superviseur", "Delay in a report to the supervisor",
			"Un état périodique destiné au superviseur est produit en retard ou incomplet, faute de collecte fiable à la source.",
			"A periodic return owed to the supervisor is produced late or incomplete, for want of reliable collection at the source.",
			0.35, 7.5, "compliance", "reporting", "supervision"),
		st("starter_cross_border_data",
			"Hébergement de données hors du pays d'exercice", "Data hosted outside the country of operation",
			"Des données d'exploitation sont hébergées hors du pays d'exercice sans que les conditions de cet hébergement aient été établies.",
			"Operational data is hosted outside the country of operation without the conditions of that hosting having been established.",
			0.30, 7.0, "data_protection", "hosting", "cross-border"),
	},
	"eu": {
		st("starter_breach_notification_delay",
			"Notification de violation hors délai", "Breach notification out of time",
			"Une violation de données est qualifiée trop tard pour que la notification parte dans le délai attendu, faute de détection et de chaîne de décision.",
			"A data breach is qualified too late for the notification to leave within the expected window, for want of detection and a decision chain.",
			0.30, 8.0, "data_protection", "notification", "detection"),
		st("starter_processor_oversight",
			"Sous-traitant non suivi", "Unmonitored processor",
			"Un sous-traitant traite des données personnelles sans revue périodique ni preuve de ses propres mesures.",
			"A processor handles personal data with no periodic review and no evidence of its own measures.",
			0.40, 6.5, "third_party", "processor", "oversight"),
	},
	"maghreb": {
		st("starter_local_hosting_requirement",
			"Localisation des données non établie", "Data location not established",
			"L'organisation ne sait pas établir où sont hébergées certaines de ses données, ce qui empêche toute réponse à une demande d'un tiers habilité.",
			"The organisation cannot establish where some of its data is hosted, which makes any answer to an authorised third party impossible.",
			0.35, 7.0, "data_protection", "hosting", "traceability"),
		st("starter_bilingual_records",
			"Documentation non disponible en langue locale", "Documentation unavailable in the local language",
			"La documentation de sécurité n'existe que dans une langue, ce qui la rend inutilisable par une partie des équipes qui doivent l'appliquer.",
			"Security documentation exists in one language only, making it unusable by part of the teams expected to apply it.",
			0.40, 5.5, "governance", "documentation", "language"),
	},
	"north_america": {
		st("starter_state_breach_patchwork",
			"Obligations de notification hétérogènes", "Heterogeneous notification obligations",
			"L'organisation opère dans plusieurs juridictions aux obligations de notification différentes, sans cartographie de celles qui s'appliquent.",
			"The organisation operates across jurisdictions with differing notification obligations, with no map of which ones apply.",
			0.35, 7.0, "compliance", "notification", "multi-jurisdiction"),
		st("starter_vendor_soc2_gap",
			"Absence d'assurance indépendante sur un prestataire", "No independent assurance on a provider",
			"Un prestataire hébergeant des données sensibles ne fournit aucune assurance indépendante sur ses contrôles.",
			"A provider hosting sensitive data supplies no independent assurance over its controls.",
			0.35, 6.5, "third_party", "assurance", "supplier"),
	},
}

// countryRegion maps an ISO-3166 alpha-2 country to its starter region.
var countryRegion = map[string]string{
	// CEMAC
	"CM": "cemac_uemoa", "GA": "cemac_uemoa", "CG": "cemac_uemoa",
	"TD": "cemac_uemoa", "CF": "cemac_uemoa", "GQ": "cemac_uemoa",
	// UEMOA
	"SN": "cemac_uemoa", "CI": "cemac_uemoa", "BJ": "cemac_uemoa",
	"BF": "cemac_uemoa", "ML": "cemac_uemoa", "NE": "cemac_uemoa",
	"TG": "cemac_uemoa", "GW": "cemac_uemoa",
	// Maghreb
	"MA": "maghreb", "DZ": "maghreb", "TN": "maghreb",
	// North America
	"CA": "north_america", "US": "north_america",
}

// regionFor resolves a country to its starter region, "" when unknown. EU
// membership is read from the existing euCountries map rather than duplicated.
func regionFor(country string) string {
	c := normaliseCountry(country)
	if r, ok := countryRegion[c]; ok {
		return r
	}
	if euCountries[c] {
		return "eu"
	}
	return ""
}

// StarterRisksFor returns EXACTLY EightStarterRisks statements for a (sector,
// country) pair, most specific first: the sector's own, then the region's, then
// the generic pool as padding.
//
// Deterministic and de-duplicated by key. An unknown sector falls back to
// "other" — never to an empty screen, because step 2 has no empty state that
// makes sense: a tunnel that asks the user to pick three of nothing is broken.
func StarterRisksFor(sector, country string) []StarterRisk {
	out := make([]StarterRisk, 0, EightStarterRisks)
	seen := map[string]bool{}

	add := func(pool []StarterRisk, scope StarterScope) {
		for _, r := range pool {
			if len(out) >= EightStarterRisks || seen[r.Key] {
				continue
			}
			seen[r.Key] = true
			r.Scope = scope
			out = append(out, r)
		}
	}

	sectorPool, ok := starterBySector[sector]
	if !ok {
		sectorPool = starterBySector["other"]
	}
	add(sectorPool, ScopeSector)
	add(starterByRegion[regionFor(country)], ScopeRegion)
	add(starterGeneric, ScopeGeneric)
	// The generic pool alone cannot be short: sector (3) + generic (6) = 9 ≥ 8
	// even with no region. The loop above is the only truncation.
	return out
}

// StarterRiskByKey looks one statement up across every pool. Used by the write
// path to re-read the canonical statement rather than trusting the client's copy
// of it — a client that could post arbitrary text would be an unvalidated write
// into a customer's risk register.
func StarterRiskByKey(key string) (StarterRisk, bool) {
	for _, pool := range starterBySector {
		for _, r := range pool {
			if r.Key == key {
				r.Scope = ScopeSector
				return r, true
			}
		}
	}
	for _, pool := range starterByRegion {
		for _, r := range pool {
			if r.Key == key {
				r.Scope = ScopeRegion
				return r, true
			}
		}
	}
	for _, r := range starterGeneric {
		if r.Key == key {
			r.Scope = ScopeGeneric
			return r, true
		}
	}
	return StarterRisk{}, false
}

// KnownStarterSectorKeys returns the sector keys carrying a bespoke starter set,
// sorted. Used by the tests; nothing depends on it at runtime.
func KnownStarterSectorKeys() []string {
	keys := make([]string, 0, len(starterBySector))
	for k := range starterBySector {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// KnownStarterRegionKeys returns the region keys, sorted. Tests only.
func KnownStarterRegionKeys() []string {
	keys := make([]string, 0, len(starterByRegion))
	for k := range starterByRegion {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// st builds a StarterRisk from positional arguments. A constructor rather than
// literals because the catalogue is long and the bilingual maps triple its
// height; the field order here is (key, fr title, en title, fr body, en body).
func st(key, frTitle, enTitle, frBody, enBody string, p, i float64, category string, tags ...string) StarterRisk {
	return StarterRisk{
		Key:             key,
		TitleI18n:       map[string]string{"fr": frTitle, "en": enTitle},
		DescriptionI18n: map[string]string{"fr": frBody, "en": enBody},
		Probability:     p,
		Impact:          i,
		Category:        category,
		Tags:            tags,
	}
}
