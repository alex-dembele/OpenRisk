import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

i18n
  .use(initReactI18next)
  .init({
    resources: {
      en: {
        translation: {
          loading: 'Loading...',
          error: 'Error',
          toggleTheme: 'Toggle Theme',
          switchLang: 'FR',
          dashboard: 'Dashboard',
          overview: 'Overview',
          threatMap: 'Threat Map',
          incidentTimeline: 'Incident Timeline',
          risks: 'Risks',
          incidents: 'Incidents',
          threats: 'Threats',
          actions: 'Actions',
          unnamedRisk: 'Unnamed Risk',
          status: 'Status',
          unnamedAction: 'Unnamed Action'
        }
      },
      fr: {
        translation: {
          loading: 'Chargement en cours...',
          error: 'Erreur',
          toggleTheme: 'Changer Thème',
          switchLang: 'EN',
          dashboard: 'Tableau de bord',
          overview: 'Aperçu',
          threatMap: 'Carte des Menaces',
          incidentTimeline: 'Chronologie des Incidents',
          risks: 'Risques',
          incidents: 'Incidents',
          threats: 'Menaces',
          actions: 'Actions',
          unnamedRisk: 'Risque non nommé',
          status: 'Statut',
          unnamedAction: 'Action non nommée'
        }
      }
    },
    lng: 'en',  // Default language
    fallbackLng: 'en',
    interpolation: {
      escapeValue: false
    }
  });

export default i18n;