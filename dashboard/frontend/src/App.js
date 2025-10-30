import React, { useState, useEffect } from 'react';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import 'tailwindcss/tailwind.css';

function App() {
  const [activeSection, setActiveSection] = useState('risks');
  const [risks, setRisks] = useState([]);
  const [incidents, setIncidents] = useState([]);
  const [threats, setThreats] = useState([]);
  const [actions, setActions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

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

  if (loading) return (
    <div className="flex items-center justify-center min-h-screen bg-gray-100">
      <div className="text-xl">Chargement en cours...</div>
    </div>
  );
  if (error) return (
    <div className="flex items-center justify-center min-h-screen bg-gray-100 text-red-500">
      <div>Erreur: {error}</div>
    </div>
  );

  const chartData = [
    { name: 'Risques', value: risks.length },
    { name: 'Incidents', value: incidents.length },
    { name: 'Menaces', value: threats.length },
    { name: 'Actions', value: actions.length }
  ];

  const renderSection = () => {
    switch (activeSection) {
      case 'risks':
        return (
          <div className="grid md:grid-cols-2 gap-4">
            {risks.map((risk, index) => (
              <div key={index} className="bg-white p-4 rounded-lg shadow-md">
                <h3 className="font-semibold">{risk.name || 'Risque non nommé'}</h3>
                <p>{risk.description}</p>
              </div>
            ))}
          </div>
        );
      case 'incidents':
        return (
          <ul className="space-y-2">
            {incidents.map((incident) => (
              <li key={incident.id} className="bg-white p-4 rounded-lg shadow-md">
                <h3 className="font-semibold">{incident.title}</h3>
                <p>Statut: {incident.status}</p>
              </li>
            ))}
          </ul>
        );
      case 'threats':
        return (
          <ul className="space-y-2">
            {threats.map((threat) => (
              <li key={threat.node.id} className="bg-white p-4 rounded-lg shadow-md">
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
              <li key={index} className="bg-white p-4 rounded-lg shadow-md">
                <h3 className="font-semibold">{action.name || 'Action non nommée'}</h3>
                <p>Statut: {action.status}</p>
              </li>
            ))}
          </ul>
        );
      default:
        return null;
    }
  };

  return (
    <div className="flex h-screen bg-gray-100">
      {/* Sidebar */}
      <div className="w-64 bg-blue-800 text-white flex flex-col">
        <h1 className="p-4 text-xl font-bold">OpenRiskOps</h1>
        <nav className="flex-1">
          {['risks', 'incidents', 'threats', 'actions'].map((section) => (
            <button
              key={section}
              onClick={() => setActiveSection(section)}
              className={`w-full text-left p-4 hover:bg-blue-700 ${activeSection === section ? 'bg-blue-700' : ''}`}
            >
              {section.charAt(0).toUpperCase() + section.slice(1)}
            </button>
          ))}
        </nav>
      </div>
      
      {/* Main Content */}
      <div className="flex-1 flex flex-col">
        <header className="bg-white shadow p-4">
          <h2 className="text-2xl font-semibold">Tableau de bord</h2>
        </header>
        <main className="flex-1 p-4 overflow-auto">
          {/* Overview Chart */}
          <div className="mb-6 bg-white p-4 rounded-lg shadow-md">
            <h3 className="text-lg font-semibold mb-2">Aperçu</h3>
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
          
          {/* Active Section */}
          <h3 className="text-lg font-semibold mb-2 capitalize">{activeSection}</h3>
          {renderSection()}
        </main>
      </div>
    </div>
  );
}

export default App;