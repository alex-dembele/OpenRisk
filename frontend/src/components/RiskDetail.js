import React, { useEffect, useState } from 'react';
import useRiskStore from '../stores/riskStore';
import { useParams, useNavigate } from 'react-router-dom';

const RiskDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const { risks, updateRisk, deleteRisk } = useRiskStore();
  const risk = risks.find(r => r.id === id);
  const [data, setData] = useState(risk || {});

  if (!risk) return <p>Risk not found</p>;

  const handleUpdate = () => {
    updateRisk(id, data);
  };

  const handleDelete = () => {
    deleteRisk(id);
    navigate('/risks');
  };

  return (
    <div className="neumorphic-card p-6">
      <input value={data.name} onChange={e => setData({...data, name: e.target.value})} />
      <button onClick={handleUpdate} className="neumorphic-button">Update</button>
      <button onClick={handleDelete} className="neumorphic-button bg-red-500">Delete</button>
      <Link to={`/plans?riskId=${id}`}>Plans</Link>
      <Link to={`/history/${id}`}>History</Link>
    </div>
  );
};

export default RiskDetail;