import React, { useState, useEffect, useRef } from 'react';
import { useTranslation, I18nextProvider } from 'react-i18next';
import i18n from './i18n';  // Import i18n config
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import { ResponsiveGridLayout, WidthProvider } from 'react-grid-layout';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import { Timeline } from 'vis-timeline/standalone';
import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';
import 'leaflet/dist/leaflet.css';
import 'vis-timeline/dist/vis-timeline-graph2d.min.css';
import 'tailwindcss/tailwind.css';

function App() {
  const { t } = useTranslation();
  const [activeSection, setActiveSection] = useState('risks');
  const [risks, setRisks] = useState([]);
  const [incidents, setIncidents] = useState([]);
  const [threats, setThreats] = useState([]);
  const [actions, setActions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [theme, setTheme] = useState(localStorage.getItem('theme') || 'light');
  const timelineRef = useRef(null);
  const timelineInstanceRef = useRef(null);

  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark');
    localStorage.setItem('theme', theme);
  }, [theme]);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [risksRes, incidentsRes, threatsRes, actionsRes] = await Promise.all([
          fetch('http://localhost:8000/risks'),
          fetch('http://localhost:8000/incidents'),
          fetch('http://localhost:8000/threats'),
          fetch('http://localhost:8000/actions')
        ]);

        if (!risksRes.ok) throw new Error('Error fetching risks');
        if (!incidentsRes.ok) throw new Error('Error fetching incidents');
        if (!threatsRes.ok) throw new Error('Error fetching threats');
        if (!actionsRes.ok) throw new Error('Error fetching actions');

        const risksData = await risksRes.json();
        const incidentsData = await incidentsRes.json();
        const threatsData = await threatsRes.json();
        const actionsData = await actionsRes.json();

        setRisks(risksData);
        setIncidents(incidentsData);
        setThreats(threatsData.data?.stixCoreObjects?.edges || []);
        setActions(actionsData);
        setLoading(false);
      } catch (err) {
        setError(err.message);
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  useEffect(() => {
    if (timelineRef.current && incidents.length > 0 && !timelineInstanceRef.current) {
      const items = incidents.map((incident) => ({
        id: incident.id,
        content: incident.title,
        start: incident.created_at || new Date(),  // Assume timestamp in data
        type: 'point'
      }));

      timelineInstanceRef.current = new Timeline(timelineRef.current, items, {
        height: '100%',
        zoomable: true
      });
    }

    return () => {
      if (timelineInstanceRef.current) {
        timelineInstanceRef.current.destroy();
        timelineInstanceRef.current = null;
      }
    };
  }, [incidents]);

  if (loading) return (
    <div className="flex items-center justify-center min-h-screen bg-gray-100 dark:bg-gray-900">
      <div className="text-xl">{t('loading')}</div>
    </div>
  );
  if (error) return (
    <div className="flex items-center justify-center min-h-screen bg-gray-100 dark:bg-gray-900 text-red-500">
      <div>{t('error')}: {error}</div>
    </div>
  );

  const chartData = [
    { name: t('risks'), value: risks.length },
    { name: t('incidents'), value: incidents.length },
    { name: t('threats'), value: threats.length },
    { name: t('actions'), value: actions.length }
  ];

  const layout = [
    { i: 'overview', x: 0, y: 0, w: 12, h: 8 },
    { i: 'map', x: 0, y: 8, w: 6, h: 8 },
    { i: 'timeline', x: 6, y: 8, w: 6, h: 8 },
    { i: 'section', x: 0, y: 16, w: 12, h: 10 }
  ];

  const ResponsiveGrid = WidthProvider(ResponsiveGridLayout);

  const renderSection = () => {
    switch (activeSection) {
      case 'risks':
        return (
          <div className="grid md:grid-cols-2 gap-4">
            {risks.map((risk, index) => (
              <div key={index} className="bg-white dark:bg-gray-800 p-4 rounded-lg shadow-md">
                <h3 className="font-semibold">{risk.name || t('unnamedRisk')}</h3>
                <p>{risk.description}</p>
              </div>
            ))}
          </div>
        );
      case 'incidents':
        return (
          <ul className="space-y-2">
            {incidents.map((incident) => (
              <li key={incident.id} className="bg-white dark:bg-gray-800 p-4 rounded-lg shadow-md">
                <h3 className="font-semibold">{incident.title}</h3>
                <p>{t('status')}: {incident.status}</p>
              </li>
            ))}
          </ul>
        );
      case 'threats':
        return (
          <ul className="space-y-2">
            {threats.map((threat) => (
              <li key={threat.node.id} className="bg-white dark:bg-gray-800 p-4 rounded-lg shadow-md">
                <h3 className="font-semibold">{threat.node.name}</h3>
                <p>{threat.node.description}</p>
              </li>
            ))}
          </ul>
        );
      case 'actions':
        return (
          <ul className="space-y-2">
            {actions.map((action, index) => (
              <li key={index} className="bg-white dark:bg-gray-800 p-4 rounded-lg shadow-md">
                <h3 className="font-semibold">{action.name || t('unnamedAction')}</h3>
                <p>{t('status')}: {action.status}</p>
              </li>
            ))}
          </ul>
        );
      default:
        return null;
    }
  };

  return (
    <I18nextProvider i18n={i18n}>
      <div className={`min-h-screen ${theme === 'dark' ? 'dark bg-gray-900 text-white' : 'bg-gray-100 text-gray-900'}`}>
        <div className="flex h-screen">
          {/* Sidebar */}
          <div className="w-64 bg-blue-800 text-white flex flex-col">
            <div className="p-4 flex items-center">
              <img src="/logo.png" alt="OpenRisk Logo" className="h-10 mr-2" />
              <h1 className="text-xl font-bold">OpenRisk</h1>
            </div>
            <nav className="flex-1">
              {['risks', 'incidents', 'threats', 'actions'].map((section) => (
                <button
                  key={section}
                  onClick={() => setActiveSection(section)}
                  className={`w-full text-left p-4 hover:bg-blue-700 ${activeSection === section ? 'bg-blue-700' : ''}`}
                >
                  {t(section)}
                </button>
              ))}
            </nav>
            <div className="p-4">
              <button onClick={() => setTheme(theme === 'light' ? 'dark' : 'light')} className="mr-2">
                {t('toggleTheme')}
              </button>
              <button onClick={() => i18n.changeLanguage(i18n.language === 'fr' ? 'en' : 'fr')}>
                {t('switchLang')}
              </button>
            </div>
          </div>
          
          {/* Main Content */}
          <div className="flex-1 flex flex-col overflow-hidden">
            <header className="bg-white dark:bg-gray-800 shadow p-4">
              <h2 className="text-2xl font-semibold">{t('dashboard')}</h2>
            </header>
            <main className="flex-1 p-4 overflow-auto">
              <ResponsiveGrid className="layout" layouts={{ lg: layout }} breakpoints={{ lg: 1200 }} cols={{ lg: 12 }}>
                <div key="overview" className="bg-white dark:bg-gray-800 p-4 rounded-lg shadow-md">
                  <h3 className="text-lg font-semibold mb-2">{t('overview')}</h3>
                  <ResponsiveContainer width="100%" height={300}>
                    <BarChart data={chartData}>
                      <CartesianGrid strokeDasharray="3 3" />
                      <XAxis dataKey="name" />
                      <YAxis />
                      <Tooltip />
                      <Legend />
                      <Bar dataKey="value" fill="#3b82f6" />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
                <div key="map" className="bg-white dark:bg-gray-800 p-4 rounded-lg shadow-md">
                  <h3 className="text-lg font-semibold mb-2">{t('threatMap')}</h3>
                  <MapContainer center={[48.8566, 2.3522]} zoom={3} style={{ height: '300px', width: '100%' }}>
                    <TileLayer url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png" />
                    {threats.filter(t => t.node.latitude && t.node.longitude).map((threat, i) => (
                      <Marker key={i} position={[threat.node.latitude, threat.node.longitude]}>
                        <Popup>{threat.node.name}</Popup>
                      </Marker>
                    ))}
                  </MapContainer>
                </div>
                <div key="timeline" className="bg-white dark:bg-gray-800 p-4 rounded-lg shadow-md">
                  <h3 className="text-lg font-semibold mb-2">{t('incidentTimeline')}</h3>
                  <div ref={timelineRef} style={{ height: '300px' }}></div>
                </div>
                <div key="section" className="bg-white dark:bg-gray-800 p-4 rounded-lg shadow-md">
                  <h3 className="text-lg font-semibold mb-2 capitalize">{t(activeSection)}</h3>
                  {renderSection()}
                </div>
              </ResponsiveGrid>
            </main>
          </div>
        </div>
      </div>
    </I18nextProvider>
  );
}

export default App;