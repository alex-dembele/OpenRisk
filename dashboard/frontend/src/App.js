import React, { useState, useEffect, useRef } from 'react';
import { motion } from 'framer-motion';
import { Responsive, WidthProvider } from 'react-grid-layout';
import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import 'leaflet/dist/leaflet.css';
import { Timeline } from 'vis-timeline/standalone';
import 'tailwindcss/tailwind.css';
import './App.css';

const ResponsiveGrid = WidthProvider(Responsive);

function App() {
  const [activeSection, setActiveSection] = useState('overview');
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);
  const [theme, setTheme] = useState('light');
  const [risks, setRisks] = useState([]);
  const [incidents, setIncidents] = useState([]);
  const [threats, setThreats] = useState([]);
  const [actions, setActions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const timelineRef = useRef(null);

  // Dark mode seamless (auto OS prefs + toggle)
  useEffect(() => {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    setTheme(mediaQuery.matches ? 'dark' : 'light');
    const listener = (e) => setTheme(e.matches ? 'dark' : 'light');
    mediaQuery.addEventListener('change', listener);
    return () => mediaQuery.removeEventListener('change', listener);
  }, []);

  // Fetch data (prod-ready avec env var)
  useEffect(() => {
    const backendUrl = process.env.REACT_APP_BACKEND_URL || 'http://localhost:8000';
    const fetchData = async () => {
      try {
        const [risksRes, incidentsRes, threatsRes, actionsRes] = await Promise.all([
          fetch(`${backendUrl}/risks`),
          fetch(`${backendUrl}/incidents`),
          fetch(`${backendUrl}/threats`),
          fetch(`${backendUrl}/actions`)
        ]);
        if (!risksRes.ok || !incidentsRes.ok || !threatsRes.ok || !actionsRes.ok) throw new Error('Fetch error');
        setRisks(await risksRes.json());
        setIncidents(await incidentsRes.json());
        setThreats(await threatsRes.json().data?.stixCoreObjects?.edges || []);
        setActions(await actionsRes.json());
        setLoading(false);
      } catch (err) {
        setError(err.message);
        setLoading(false);
      }
    };
    fetchData();
  }, []);

  // Timeline init
  useEffect(() => {
    if (timelineRef.current && incidents.length) {
      const items = incidents.map((inc, i) => ({ id: i, content: inc.title, start: new Date(inc.created_at || Date.now()) }));
      new Timeline(timelineRef.current, items, { height: '100%' });
    }
  }, [incidents]);

  // Layout personnalisable (drag/resize, localStorage)
  const defaultLayout = [
    { i: 'overview-chart', x: 0, y: 0, w: 12, h: 4, minW: 4, minH: 2 },
    { i: 'threat-map', x: 0, y: 4, w: 6, h: 6, minW: 4, minH: 4 },
    { i: 'incident-timeline', x: 6, y: 4, w: 6, h: 6, minW: 4, minH: 4 },
    { i: 'risk-list', x: 0, y: 10, w: 6, h: 4, minW: 4, minH: 2 },
    { i: 'action-list', x: 6, y: 10, w: 6, h: 4, minW: 4, minH: 2 }
  ];
  const [layout, setLayout] = useState(JSON.parse(localStorage.getItem('openriskLayout')) || defaultLayout);

  const onLayoutChange = (newLayout) => {
    setLayout(newLayout);
    localStorage.setItem('openriskLayout', JSON.stringify(newLayout));
  };

  // Haptics (vibration mobile pour feedback)
  const hapticFeedback = () => {
    if (navigator.vibrate) navigator.vibrate(50);
  };

  if (loading) return <div className="flex items-center justify-center min-h-screen bg-gray-100 dark:bg-gray-900">Loading...</div>;
  if (error) return <div className="flex items-center justify-center min-h-screen bg-gray-100 dark:bg-gray-900 text-red-500">Error: {error}</div>;

  const chartData = [
    { name: 'Risks', value: risks.length, fill: '#10B981' },
    { name: 'Incidents', value: incidents.length, fill: '#3b82f6' },
    { name: 'Threats', value: threats.length, fill: '#EF4444' },
    { name: 'Actions', value: actions.length, fill: '#F59E0B' }
  ];

  return (
    <div className={`min-h-screen ${theme === 'dark' ? 'dark bg-gray-900 text-white' : 'bg-gray-100 text-gray-900'} flex`}>
      {/* Sidebar collapsible (Slack-like) */}
      <motion.aside
        initial={{ x: -300 }}
        animate={{ x: isSidebarOpen ? 0 : -300 }}
        transition={{ type: 'spring', stiffness: 120 }}
        className="w-64 bg-gradient-to-b from-blue-800 to-blue-600 dark:from-gray-800 dark:to-gray-700 p-4 shadow-lg overflow-y-auto z-10"
      >
        <h2 className="text-xl font-bold mb-4">OpenRisk</h2>
        <nav className="space-y-2">
          {['overview', 'risks', 'incidents', 'threats', 'actions'].map((section) => (
            <motion.button
              key={section}
              onClick={() => { setActiveSection(section); hapticFeedback(); }}
              whileHover={{ scale: 1.05, boxShadow: "0px 0px 8px rgba(0,0,0,0.2)" }}
              className={`w-full text-left p-3 rounded-lg neumorphic-button ${activeSection === section ? 'bg-blue-700 dark:bg-gray-600' : ''}`}
            >
              {section.charAt(0).toUpperCase() + section.slice(1)}
            </motion.button>
          ))}
        </nav>
        <motion.button
          onClick={() => { setIsSidebarOpen(!isSidebarOpen); hapticFeedback(); }}
          whileHover={{ scale: 1.05 }}
          className="mt-auto p-2 neumorphic-button w-full"
        >
          {isSidebarOpen ? 'Close' : 'Open'} Sidebar
        </motion.button>
      </motion.aside>

      {/* Main */}
      <main className="flex-1 p-6 overflow-auto">
        <motion.h1
          initial={{ opacity: 0, y: -20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
          className="text-3xl font-bold mb-6"
        >
          OpenRisk Dashboard
        </motion.h1>
        <ResponsiveGrid
          className="layout"
          layouts={{ lg: layout, md: layout, sm: layout, xs: layout }}
          breakpoints={{ lg: 1200, md: 996, sm: 768, xs: 480 }}
          cols={{ lg: 12, md: 8, sm: 4, xs: 2 }}
          rowHeight={100}
          onLayoutChange={onLayoutChange}
          isDraggable
          isResizable
          compactType="vertical"
          margin={[16, 16]}
          containerPadding={[16, 16]}
        >
          <motion.div
            key="overview-chart"
            className="neumorphic-card p-4 cursor-grab active:cursor-grabbing"
            whileHover={{ scale: 1.02, boxShadow: "0px 10px 20px rgba(0,0,0,0.1)" }}
            onClick={hapticFeedback}
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
          >
            <h3 className="text-lg font-semibold mb-2">Overview Chart</h3>
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="name" />
                <YAxis />
                <Tooltip />
                <Legend />
                <Bar dataKey="value" fill="url(#boldGradient)" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </motion.div>

          <motion.div
            key="threat-map"
            className="neumorphic-card p-4 cursor-grab active:cursor-grabbing"
            whileHover={{ scale: 1.02 }}
            onClick={hapticFeedback}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
          >
            <h3 className="text-lg font-semibold mb-2">Threat Map</h3>
            <MapContainer center={[51.505, -0.09]} zoom={13} style={{ height: '100%', width: '100%', borderRadius: '8px' }}>
              <TileLayer url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png" />
              {threats.map((threat, i) => (
                <Marker key={i} position={[threat.node.lat || 51.505, threat.node.lng || -0.09]}>
                  <Popup>{threat.node.name}</Popup>
                </Marker>
              ))}
            </MapContainer>
          </motion.div>

          <motion.div
            key="incident-timeline"
            className="neumorphic-card p-4 cursor-grab active:cursor-grabbing"
            whileHover={{ scale: 1.02 }}
            onClick={hapticFeedback}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
          >
            <h3 className="text-lg font-semibold mb-2">Incident Timeline</h3>
            <div ref={timelineRef} style={{ height: '100%', width: '100%' }}></div>
          </motion.div>

          <motion.div
            key="risk-list"
            className="neumorphic-card p-4 overflow-auto cursor-grab active:cursor-grabbing"
            whileHover={{ scale: 1.02 }}
            onClick={hapticFeedback}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
          >
            <h3 className="text-lg font-semibold mb-2">Risks</h3>
            <ul className="space-y-2">
              {risks.map((risk, i) => (
                <li key={i} className="p-2 bg-gradient-to-r from-gray-100 to-gray-200 dark:from-gray-700 dark:to-gray-600 rounded shadow-sm">{risk.name}</li>
              ))}
            </ul>
          </motion.div>

          <motion.div
            key="action-list"
            className="neumorphic-card p-4 overflow-auto cursor-grab active:cursor-grabbing"
            whileHover={{ scale: 1.02 }}
            onClick={hapticFeedback}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
          >
            <h3 className="text-lg font-semibold mb-2">Actions</h3>
            <ul className="space-y-2">
              {actions.map((action, i) => (
                <li key={i} className="p-2 bg-gradient-to-r from-gray-100 to-gray-200 dark:from-gray-700 dark:to-gray-600 rounded shadow-sm">{action.name}</li>
              ))}
            </ul>
          </motion.div>
        </ResponsiveGrid>
      </main>
    </div>
  );
}

export default App;