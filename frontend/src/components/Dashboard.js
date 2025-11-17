import React from 'react';
import { motion } from 'framer-motion';
import { ResponsiveContainer, HeatMap, LineChart, Line } from 'recharts'; 
import useRiskStore from '../stores/riskStore';

const Dashboard = () => {
  const { risks, plans } = useRiskStore();
  const score = risks.reduce((acc, r) => acc + r.probability * r.impact * r.criticality, 0) / risks.length || 0; // Global score

  const heatmapData = risks.map(r => ({ x: r.name, y: r.status, value: r.Score() })); // heatmap
  const trendsData = [{ day: '30d', value: 80 }, { day: '60d', value: 70 }, { day: '90d', value: 90 }]; // Mock trends

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="grid grid-cols-1 md:grid-cols-2 gap-6">
      <div className="neumorphic-card p-4">
        <h2>Global Score: {score.toFixed(2)}</h2>
      </div>
      <div className="neumorphic-card p-4">
        <ResponsiveContainer height={300}>
          <HeatMap data={heatmapData} xAxisKey="x" yAxisKey="y" colorKey="value" />
        </ResponsiveContainer>
      </div>
      <div className="neumorphic-card p-4">
        <ResponsiveContainer height={300}>
          <LineChart data={trendsData}>
            <Line type="monotone" dataKey="value" stroke="#10B981" />
          </LineChart>
        </ResponsiveContainer>
      </div>
      {/* Add risks by asset,*/}
    </motion.div>
  );
};

export default Dashboard;