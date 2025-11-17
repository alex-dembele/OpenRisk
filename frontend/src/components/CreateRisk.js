import React, { useState } from 'react';
import useRiskStore from '../stores/riskStore';
import { useNavigate } from 'react-router-dom';

const CreateRisk = () => {
  const { createRisk } = useRiskStore();
  const navigate = useNavigate();
  const [data, setData] = useState({ name: '', probability: 1, impact: 1, criticality: 1, tags: '', custom_fields: '' });

  const handleSubmit = (e) => {
    e.preventDefault();
    createRisk(data);
    navigate('/risks');
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4 neumorphic-card p-6">
      <input value={data.name} onChange={e => setData({...data, name: e.target.value})} placeholder="Name" className="p-2 border rounded w-full" />
      <select value={data.probability} onChange={e => setData({...data, probability: parseInt(e.target.value)})} className="p-2 border rounded w-full">
        <option value={1}>1 (Low)</option> 
        {/* Up to 5 */}
      </select>
      <input value={data.tags} onChange={e => setData({...data, tags: e.target.value})} placeholder="Tags (JSON)" />
      <input value={data.custom_fields} onChange={e => setData({...data, custom_fields: e.target.value})} placeholder="Custom (JSON)" />
      <button type="submit" className="neumorphic-button p-2">Create</button>
    </form>
  );
};

export default CreateRisk;