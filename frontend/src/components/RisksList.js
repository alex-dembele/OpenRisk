import React, { useEffect } from 'react';
import useRiskStore from '../stores/riskStore';
import { Link } from 'react-router-dom';

const RisksList = () => {
  const { risks, fetchRisks } = useRiskStore();
  useEffect(() => { fetchRisks(); }, []);

  return (
    <div className="space-y-4">
      <Link to="/risks/create" className="neumorphic-button p-2">Create Risk</Link>
      <input placeholder="Filter..." className="p-2 border rounded w-full" /> {/* Advanced filters */}
      <ul className="space-y-2">
        {risks.map(r => (
          <li key={r.id} className="neumorphic-card p-4">
            <Link to={`/risks/${r.id}`}>{r.name} (Score: {r.Score()})</Link>
            <p>Tags: {r.tags}</p>
            <p>Custom: {r.custom_fields}</p>
          </li>
        ))}
      </ul>
    </div>
  );
};

export default RisksList;