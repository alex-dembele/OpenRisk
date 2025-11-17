import React, { useEffect } from 'react';
import { motion } from 'framer-motion';
import DiffViewer from 'react-diff-viewer-continued';
import useRiskStore from '../stores/riskStore';
import { useParams } from 'react-router-dom';

const History = () => {
  const { riskId } = useParams();
  const { history, fetchHistory, loading, error } = useRiskStore(); 

  useEffect(() => {
    if (riskId) {
      fetchHistory(riskId);
    }
  }, [riskId, fetchHistory]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <motion.div
          animate={{ rotate: 360 }}
          transition={{ duration: 1, repeat: Infinity, ease: "linear" }}
          className="w-8 h-8 border-4 border-t-blue-500 border-gray-200 rounded-full"
        />
        <p className="ml-4 text-xl text-gray-700 dark:text-gray-300">Loading history...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-64 text-red-500">
        <p>{error}</p>
      </div>
    );
  }

  if (history.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-500">
        <p>No history available for this risk.</p>
      </div>
    );
  }

  return (
    <motion.div 
      initial={{ opacity: 0 }} 
      animate={{ opacity: 1 }} 
      className="space-y-6"
    >
      <h2 className="text-2xl font-bold mb-4">Risk History</h2>
      {history.map((h, index) => (
        <motion.div 
          key={h.id || index} 
          className="neumorphic-card p-4"
          initial={{ y: 20, opacity: 0 }}
          animate={{ y: 0, opacity: 1 }}
          transition={{ delay: index * 0.1 }}
        >
          <h3 className="text-lg font-semibold mb-2">
            Change: {h.changeType} at {new Date(h.createdAt).toLocaleString()}
          </h3>
          <DiffViewer 
            oldValue={JSON.stringify(h.diff.before || {}, null, 2)} 
            newValue={JSON.stringify(h.diff.after || {}, null, 2)} 
            splitView={true} 
            showDiffOnly={true} 
            leftTitle="Before" 
            rightTitle="After" 
          />
        </motion.div>
      ))}
    </motion.div>
  );
};

export default History;