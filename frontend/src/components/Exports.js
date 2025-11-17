import React from 'react';
import { motion } from 'framer-motion';
import useRiskStore from '../stores/riskStore';
import toast from 'react-hot-toast';

const Exports = () => {
  const { exportPDF, exportCSV, exportJSON } = useRiskStore();

  const handleExport = (type, func) => {
    func();
    toast.success(`${type} export started!`);
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5 }}
      className="neumorphic-card p-6 space-y-4"
    >
      <h2 className="text-2xl font-bold mb-4">Exports</h2>
      <p className="text-gray-600 dark:text-gray-400">Choose your export format:</p>
      <div className="flex flex-col md:flex-row space-y-2 md:space-y-0 md:space-x-4">
        <motion.button
          whileHover={{ scale: 1.05 }}
          whileTap={{ scale: 0.95 }}
          onClick={() => handleExport('PDF', exportPDF)}
          className="neumorphic-button p-3 bg-gradient-to-r from-blue-500 to-blue-700 text-white rounded-lg shadow-md"
        >
          Export PDF
        </motion.button>
        <motion.button
          whileHover={{ scale: 1.05 }}
          whileTap={{ scale: 0.95 }}
          onClick={() => handleExport('CSV', exportCSV)}
          className="neumorphic-button p-3 bg-gradient-to-r from-green-500 to-green-700 text-white rounded-lg shadow-md"
        >
          Export CSV
        </motion.button>
        <motion.button
          whileHover={{ scale: 1.05 }}
          whileTap={{ scale: 0.95 }}
          onClick={() => handleExport('JSON', exportJSON)}
          className="neumorphic-button p-3 bg-gradient-to-r from-purple-500 to-purple-700 text-white rounded-lg shadow-md"
        >
          Export JSON
        </motion.button>
      </div>
    </motion.div>
  );
};

export default Exports;