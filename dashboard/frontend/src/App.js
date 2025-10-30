import React, { useState, useEffect } from 'react';
import 'tailwindcss/tailwind.css';

function App() {
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
          fetch('http://localhost:8000/risks'),  // Adjust to the production backend URL
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

  if (loading) return <div className="p-4">Loading...</div>;
  if (error) return <div className="p-4 text-red-500">Error: {error}</div>;

  return (
    <div className="container mx-auto p-4">
      <h1 className="text-3xl font-bold mb-4">OpenRiskOps Dashboard</h1>
      
      <section className="mb-8">
        <h2 className="text-2xl font-semibold mb-2">Risks (from OpenRMF)</h2>
        <ul className="list-disc pl-5">
          {risks.map((risk, index) => (
            <li key={index}>{risk.name || 'Unnamed Risk'} - {risk.description}</li>
          ))}
        </ul>
      </section>
      
      <section className="mb-8">
        <h2 className="text-2xl font-semibold mb-2">Incidents (from TheHive)</h2>
        <ul className="list-disc pl-5">
          {incidents.map((incident) => (
            <li key={incident.id}>{incident.title} - Status: {incident.status}</li>
          ))}
        </ul>
      </section>
      
      <section className="mb-8">
        <h2 className="text-2xl font-semibold mb-2">Threats (from OpenCTI)</h2>
        <ul className="list-disc pl-5">
          {threats.map((threat) => (
            <li key={threat.node.id}>{threat.node.name} - {threat.node.description}</li>
          ))}
        </ul>
      </section>
      
      <section className="mb-8">
        <h2 className="text-2xl font-semibold mb-2">Actions (from Cortex)</h2>
        <ul className="list-disc pl-5">
          {actions.map((action, index) => (
            <li key={index}>{action.name || 'Unnamed Action'} - Status: {action.status}</li>
          ))}
        </ul>
      </section>
    </div>
  );
}

export default App;