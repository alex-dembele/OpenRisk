// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Auth-screen copy and quotes, in French and English.
//
// The auth screens are the one place a user can be before any preference is
// known, so the language switcher lives here and the strings cannot lean on the
// authenticated app's locale. Everything the sign-in, reset and MFA screens say
// is in this file, in both languages, so a missing translation is a type error
// rather than an English sentence appearing in a French UI.

import type { Lang } from '../../store/uiStore';

// ---------------------------------------------------------------------------
// Quotes
// ---------------------------------------------------------------------------

/**
 * Real, attributed quotes on security, risk and evidence.
 *
 * Every one is a genuine attributed line — nothing invented, nothing
 * paraphrased into someone's mouth. A security product that fabricates a
 * Schneier quote on its own login screen has already lost the argument about
 * whether its data can be trusted.
 *
 * Both languages carry the same set so the rotation is identical whichever
 * language you read it in; the French rendering of an English original is a
 * translation, and vice versa.
 */
export interface Quote {
  fr: string;
  en: string;
  author: string;
}

export const QUOTES: readonly Quote[] = [
  {
    fr: 'La sécurité est un processus, pas un produit.',
    en: 'Security is a process, not a product.',
    author: 'Bruce Schneier',
  },
  {
    fr: 'Le seul système vraiment sûr est éteint, coulé dans un bloc de béton et scellé dans une pièce blindée sous bonne garde.',
    en: 'The only truly secure system is one that is powered off, cast in a block of concrete and sealed in a lead-lined room with armed guards.',
    author: 'Gene Spafford',
  },
  {
    fr: 'Les amateurs piratent des systèmes, les professionnels piratent des personnes.',
    en: 'Amateurs hack systems, professionals hack people.',
    author: 'Bruce Schneier',
  },
  {
    fr: 'Il y a deux types d’entreprises : celles qui ont été piratées, et celles qui ne le savent pas encore.',
    en: 'There are two types of companies: those that have been hacked, and those that don’t yet know they have been hacked.',
    author: 'John Chambers',
  },
  {
    fr: 'Il faut 20 ans pour bâtir une réputation et quelques minutes de cyber-incident pour la ruiner.',
    en: 'It takes 20 years to build a reputation and a few minutes of cyber-incident to ruin it.',
    author: 'Stéphane Nappo',
  },
  {
    fr: 'La complexité est le pire ennemi de la sécurité.',
    en: 'Complexity is the worst enemy of security.',
    author: 'Bruce Schneier',
  },
  {
    fr: 'Avec assez d’yeux, tous les bugs deviennent évidents.',
    en: 'Given enough eyeballs, all bugs are shallow.',
    author: 'Loi de Linus — Eric S. Raymond',
  },
  {
    fr: 'En Dieu nous croyons. Tous les autres doivent apporter des données.',
    en: 'In God we trust. All others must bring data.',
    author: 'W. Edwards Deming',
  },
  {
    fr: 'Le risque vient de ne pas savoir ce que l’on fait.',
    en: 'Risk comes from not knowing what you’re doing.',
    author: 'Warren Buffett',
  },
  {
    fr: 'Mieux vaut prévenir que guérir.',
    en: 'An ounce of prevention is worth a pound of cure.',
    author: 'Benjamin Franklin',
  },
  {
    fr: 'La mesure de l’intelligence, c’est la capacité de changer.',
    en: 'The measure of intelligence is the ability to change.',
    author: 'Albert Einstein',
  },
  {
    fr: 'Ce que l’on anticipe arrive rarement ; ce que l’on attend le moins arrive généralement.',
    en: 'What we anticipate seldom occurs; what we least expect generally happens.',
    author: 'Benjamin Disraeli',
  },
] as const;

/**
 * Index of the quote a given day starts on.
 *
 * Deterministic, and derived from the date rather than Math.random(), for three
 * reasons: everyone signing in on the same day sees the same opening quote, a
 * page reload does not reshuffle it, and the rotation is testable — a random
 * start can only be asserted loosely.
 *
 * The key is the local calendar day, not a UTC timestamp, so the quote turns
 * over at the user's midnight rather than at an arbitrary hour of their evening.
 */
export function dailyQuoteIndex(date: Date = new Date(), total: number = QUOTES.length): number {
  if (total <= 0) return 0;
  // Days since the epoch, in local time. Constructing from the Y/M/D parts
  // discards the time of day, so every moment within a day maps to one number.
  const local = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const dayNumber = Math.floor(local.getTime() / 86_400_000);
  // JS % keeps the sign of the dividend; dates before 1970 would otherwise index
  // negatively.
  return ((dayNumber % total) + total) % total;
}

/** The quote shown at rotation `step` on `date`. */
export function quoteAt(step: number, date: Date = new Date()): Quote {
  const start = dailyQuoteIndex(date);
  const total = QUOTES.length;
  return QUOTES[(((start + step) % total) + total) % total];
}

// ---------------------------------------------------------------------------
// Screen copy
// ---------------------------------------------------------------------------

/**
 * The auth screens' strings.
 *
 * Typed as a record over both languages so adding a key to one without the other
 * fails to compile — the usual way half-translated UIs happen.
 */
export interface AuthCopy {
  // Shared
  productTagline: string;
  footerBy: string;
  footerCompany: string;
  languageToggle: string;
  themeToggle: string;

  // Sign in
  signInTitle: string;
  signInSubtitle: string;
  email: string;
  password: string;
  rememberMe: string;
  forgotPassword: string;
  signIn: string;
  signingIn: string;
  orContinueWith: string;
  noAccount: string;
  createAccount: string;
  signInFailed: string;

  // Register
  registerTitle: string;
  registerSubtitle: string;
  fullName: string;
  haveAccount: string;
  signInLink: string;
  registerNameRequired: string;
  registerEmailExists: string;
  registerFailed: string;

  // Forgot password
  forgotTitle: string;
  forgotSubtitle: string;
  forgotSubmit: string;
  forgotSending: string;
  forgotSentTitle: string;
  /** Deliberately conditional — see the backend's uniform acknowledgement. */
  forgotSentBody: string;
  forgotRateLimited: string;
  backToSignIn: string;

  // Reset password
  resetTitle: string;
  resetSubtitle: string;
  newPassword: string;
  confirmPassword: string;
  resetSubmit: string;
  resetting: string;
  resetDoneTitle: string;
  resetDoneBody: string;
  resetTokenInvalid: string;
  resetMismatch: string;
  resetMissingToken: string;

  // Password strength
  strengthLabel: string;
  strength0: string;
  strength1: string;
  strength2: string;
  strength3: string;
  strength4: string;
  strengthChecking: string;
  strengthBreached: string;

  // MFA
  mfaTitle: string;
  mfaSubtitle: string;
  mfaCode: string;
  mfaSubmit: string;
  mfaUseBackup: string;
  mfaInvalid: string;
  mfaSetupFailed: string;
  mfaEnrolTitle: string;
  mfaEnrolSubtitle: string;
  mfaEnrolScan: string;
  mfaEnrolManual: string;
  mfaEnrolConfirm: string;
  mfaEnrolBackupTitle: string;
  mfaEnrolBackupBody: string;
  mfaEnrolDone: string;

  // OAuth failures — keyed to the backend's ?error= codes.
  oauth: Record<OAuthErrorCode, string>;
  oauthConflictWith: (existing: string) => string;
}

/** The error codes internal/handler/oauth2_handler.go can redirect with. */
export type OAuthErrorCode =
  | 'access_denied'
  | 'consent_required'
  | 'provider_error'
  | 'state_missing'
  | 'state_invalid'
  | 'code_missing'
  | 'exchange_failed'
  | 'userinfo_failed'
  | 'unsupported_provider'
  | 'provider_not_configured'
  | 'email_unverified'
  | 'no_email'
  | 'account_disabled'
  | 'no_account'
  | 'provider_conflict'
  | 'internal';

const fr: AuthCopy = {
  productTagline: 'La plateforme GRC open source',
  footerBy: 'Un produit',
  footerCompany: 'OpenDefender',
  languageToggle: 'Passer en anglais',
  themeToggle: 'Changer de thème',

  signInTitle: 'Content de vous revoir',
  signInSubtitle: 'Connectez-vous pour reprendre votre travail.',
  email: 'E-mail',
  password: 'Mot de passe',
  rememberMe: 'Rester connecté',
  forgotPassword: 'Mot de passe oublié ?',
  signIn: 'Se connecter',
  signingIn: 'Connexion…',
  orContinueWith: 'ou continuer avec',
  noAccount: 'Pas encore de compte ?',
  createAccount: 'Créer un compte',
  signInFailed: 'E-mail ou mot de passe incorrect. Vérifiez et réessayez.',

  registerTitle: 'Créer votre espace',
  registerSubtitle: 'Trois champs, et vous y êtes.',
  fullName: 'Nom complet',
  haveAccount: 'Vous avez déjà un compte ?',
  signInLink: 'Se connecter',
  registerNameRequired: 'Indiquez votre nom.',
  registerEmailExists: 'Un compte existe déjà avec cet e-mail. Connectez-vous.',
  registerFailed: 'La création du compte a échoué. Réessayez dans un instant.',

  forgotTitle: 'Mot de passe oublié',
  forgotSubtitle: 'Indiquez votre e-mail : nous vous enverrons un lien pour en choisir un nouveau.',
  forgotSubmit: 'Envoyer le lien',
  forgotSending: 'Envoi…',
  forgotSentTitle: 'Vérifiez votre boîte mail',
  forgotSentBody:
    "Si un compte existe pour cette adresse, un lien de réinitialisation vient d'être envoyé. Il expire dans 30 minutes.",
  forgotRateLimited: 'Trop de demandes pour cette adresse. Réessayez dans une heure.',
  backToSignIn: 'Retour à la connexion',

  resetTitle: 'Choisissez un nouveau mot de passe',
  resetSubtitle: 'Il remplacera l’ancien et déconnectera toutes vos sessions actives.',
  newPassword: 'Nouveau mot de passe',
  confirmPassword: 'Confirmez le mot de passe',
  resetSubmit: 'Enregistrer le mot de passe',
  resetting: 'Enregistrement…',
  resetDoneTitle: 'Mot de passe modifié',
  resetDoneBody:
    'Votre mot de passe a été modifié et toutes les sessions actives ont été déconnectées. Vous pouvez vous reconnecter.',
  resetTokenInvalid:
    "Ce lien n'est plus valide — il a peut-être expiré ou déjà été utilisé. Demandez-en un nouveau.",
  resetMismatch: 'Les deux mots de passe ne correspondent pas.',
  resetMissingToken: 'Lien incomplet. Ouvrez-le directement depuis votre e-mail.',

  strengthLabel: 'Robustesse',
  strength0: 'Très faible',
  strength1: 'Faible',
  strength2: 'Moyenne',
  strength3: 'Bonne',
  strength4: 'Excellente',
  strengthChecking: 'Vérification…',
  strengthBreached: 'Ce mot de passe figure dans des fuites de données connues.',

  mfaTitle: 'Vérification en deux étapes',
  mfaSubtitle: 'Saisissez le code à 6 chiffres de votre application d’authentification.',
  mfaCode: 'Code de vérification',
  mfaSubmit: 'Vérifier',
  mfaUseBackup: 'Utiliser un code de récupération',
  mfaInvalid: 'Code incorrect. Vérifiez votre application et réessayez.',
  // Distinct from registerFailed on purpose: at this point the account EXISTS.
  // Telling the user their sign-up failed sends them back to create it again.
  mfaSetupFailed:
    'Impossible de préparer la double authentification. Votre compte est créé — rechargez la page pour réessayer.',
  mfaEnrolTitle: 'Activez la double authentification',
  mfaEnrolSubtitle:
    'Votre rôle donne accès aux données de toute l’organisation : un second facteur est obligatoire.',
  mfaEnrolScan: 'Scannez ce QR code avec votre application d’authentification.',
  mfaEnrolManual: 'Ou saisissez cette clé manuellement :',
  mfaEnrolConfirm: 'Confirmer et activer',
  mfaEnrolBackupTitle: 'Vos codes de récupération',
  mfaEnrolBackupBody:
    'Conservez-les hors ligne. Chacun ne fonctionne qu’une fois et ils sont votre seul recours si vous perdez votre téléphone.',
  mfaEnrolDone: "J'ai enregistré mes codes",

  oauth: {
    access_denied: 'Connexion annulée. Vous pouvez réessayer ou utiliser votre mot de passe.',
    consent_required:
      'Votre organisation doit autoriser OpenRisk auprès de ce fournisseur. Contactez votre administrateur.',
    provider_error:
      'Le fournisseur a refusé la connexion. Réessayez ou utilisez votre mot de passe.',
    state_missing: 'Connexion interrompue. Relancez-la depuis cette page.',
    state_invalid:
      'La connexion a expiré ou a été ouverte dans un autre onglet. Relancez-la depuis cette page.',
    code_missing: 'Réponse incomplète du fournisseur. Réessayez.',
    exchange_failed: "Impossible de finaliser l'échange avec le fournisseur. Réessayez.",
    userinfo_failed: 'Impossible de récupérer votre profil auprès du fournisseur. Réessayez.',
    unsupported_provider: 'Ce fournisseur n’est pas pris en charge.',
    provider_not_configured:
      "Cette méthode de connexion n'est pas configurée sur ce serveur. Utilisez votre mot de passe.",
    email_unverified:
      "Votre adresse n'est pas vérifiée chez ce fournisseur. Vérifiez-la puis réessayez.",
    no_email: "Le fournisseur n'a communiqué aucune adresse e-mail. Utilisez votre mot de passe.",
    account_disabled: 'Ce compte est désactivé. Contactez votre administrateur.',
    no_account:
      "Aucun compte OpenRisk n'est associé à cette adresse. Demandez une invitation à votre administrateur.",
    provider_conflict: 'Cette adresse est déjà associée à un autre fournisseur.',
    internal: 'Une erreur est survenue pendant la connexion. Réessayez dans un instant.',
  },
  oauthConflictWith: (existing) =>
    `Cette adresse se connecte déjà avec ${existing}. Utilisez ${existing} ou votre mot de passe.`,
};

const en: AuthCopy = {
  productTagline: 'The open-source GRC platform',
  footerBy: 'A product by',
  footerCompany: 'OpenDefender',
  languageToggle: 'Switch to French',
  themeToggle: 'Toggle theme',

  signInTitle: 'Welcome back',
  signInSubtitle: 'Sign in to pick up where you left off.',
  email: 'Email',
  password: 'Password',
  rememberMe: 'Keep me signed in',
  forgotPassword: 'Forgot password?',
  signIn: 'Sign in',
  signingIn: 'Signing in…',
  orContinueWith: 'or continue with',
  noAccount: 'No account yet?',
  createAccount: 'Create an account',
  signInFailed: 'Incorrect email or password. Please check and try again.',

  registerTitle: 'Create your workspace',
  registerSubtitle: 'Three fields and you’re in.',
  fullName: 'Full name',
  haveAccount: 'Already have an account?',
  signInLink: 'Sign in',
  registerNameRequired: 'Please enter your name.',
  registerEmailExists: 'An account already exists for this email. Please sign in.',
  registerFailed: "We couldn't create your account. Please try again shortly.",

  forgotTitle: 'Forgot your password',
  forgotSubtitle: 'Enter your email and we’ll send a link to choose a new one.',
  forgotSubmit: 'Send the link',
  forgotSending: 'Sending…',
  forgotSentTitle: 'Check your inbox',
  forgotSentBody:
    'If an account exists for this address, a reset link is on its way. It expires in 30 minutes.',
  forgotRateLimited: 'Too many requests for this address. Try again in an hour.',
  backToSignIn: 'Back to sign in',

  resetTitle: 'Choose a new password',
  resetSubtitle: 'It replaces the old one and signs out every active session.',
  newPassword: 'New password',
  confirmPassword: 'Confirm password',
  resetSubmit: 'Save password',
  resetting: 'Saving…',
  resetDoneTitle: 'Password changed',
  resetDoneBody:
    'Your password has been changed and every active session was signed out. You can sign in again.',
  resetTokenInvalid:
    'This link is no longer valid — it may have expired or already been used. Request a new one.',
  resetMismatch: 'The two passwords don’t match.',
  resetMissingToken: 'Incomplete link. Open it directly from your email.',

  strengthLabel: 'Strength',
  strength0: 'Very weak',
  strength1: 'Weak',
  strength2: 'Fair',
  strength3: 'Good',
  strength4: 'Excellent',
  strengthChecking: 'Checking…',
  strengthBreached: 'This password appears in known data breaches.',

  mfaTitle: 'Two-step verification',
  mfaSubtitle: 'Enter the 6-digit code from your authenticator app.',
  mfaCode: 'Verification code',
  mfaSubmit: 'Verify',
  mfaUseBackup: 'Use a recovery code',
  mfaInvalid: 'Incorrect code. Check your app and try again.',
  mfaSetupFailed:
    'Could not set up two-factor authentication. Your account exists — reload the page to try again.',
  mfaEnrolTitle: 'Turn on two-factor authentication',
  mfaEnrolSubtitle:
    'Your role can reach data across the whole organisation, so a second factor is required.',
  mfaEnrolScan: 'Scan this QR code with your authenticator app.',
  mfaEnrolManual: 'Or enter this key manually:',
  mfaEnrolConfirm: 'Confirm and enable',
  mfaEnrolBackupTitle: 'Your recovery codes',
  mfaEnrolBackupBody:
    'Keep them somewhere offline. Each works once, and they are your only way back in if you lose your phone.',
  mfaEnrolDone: 'I’ve saved my codes',

  oauth: {
    access_denied: 'Sign-in cancelled. You can try again or use your password.',
    consent_required:
      'Your organisation needs to approve OpenRisk with this provider. Contact your administrator.',
    provider_error: 'The provider refused the sign-in. Try again or use your password.',
    state_missing: 'Sign-in was interrupted. Start it again from this page.',
    state_invalid:
      'That sign-in expired or was opened in another tab. Start it again from this page.',
    code_missing: 'The provider’s response was incomplete. Please try again.',
    exchange_failed: 'We couldn’t complete the exchange with the provider. Please try again.',
    userinfo_failed: 'We couldn’t read your profile from the provider. Please try again.',
    unsupported_provider: 'That provider isn’t supported.',
    provider_not_configured:
      'This sign-in method isn’t configured on this server. Please use your password.',
    email_unverified: 'Your address isn’t verified with that provider. Verify it and try again.',
    no_email: 'The provider returned no email address. Please use your password.',
    account_disabled: 'This account is disabled. Contact your administrator.',
    no_account:
      'No OpenRisk account is linked to this address. Ask your administrator for an invitation.',
    provider_conflict: 'This address is already linked to a different provider.',
    internal: 'Something went wrong during sign-in. Please try again shortly.',
  },
  oauthConflictWith: (existing) =>
    `This address already signs in with ${existing}. Use ${existing} or your password.`,
};

// Partial: a registered language without auth copy falls back in `authCopy`.
const BUNDLES: Partial<Record<Lang, AuthCopy>> = { fr, en };

/** The auth copy for a language. */
export function authCopy(lang: Lang): AuthCopy {
  return BUNDLES[lang] ?? fr;
}

/** Human label for a zxcvbn score, 0..4. */
export function strengthLabel(copy: AuthCopy, score: number): string {
  return [copy.strength0, copy.strength1, copy.strength2, copy.strength3, copy.strength4][
    Math.max(0, Math.min(4, score))
  ];
}

/** Display name for a provider id, for the conflict message. */
export function providerLabel(id: string): string {
  switch (id) {
    case 'google':
      return 'Google';
    case 'github':
      return 'GitHub';
    case 'azure':
      return 'Microsoft';
    default:
      return id;
  }
}
