import React from 'react';
import 'tailwindcss/tailwind.css';

function App() {
  const [risks, setRisks] = React.useState([]);

  React.useEffect(() => {
    fetch('http://localhost:8000/risks')
      .then(res => res.json())
      .then(setRisks);
  }, []);

  return (
    <div className="p-4">
      <h1>OpenRisk Dashboard</h1>
      <ul>
        {risks.map(risk => <li key={risk.id}>{risk.name}</li>)}
      </ul>
      {/* Sections for threats, incidents, actions */}
    </div>
  );
}

export default App;