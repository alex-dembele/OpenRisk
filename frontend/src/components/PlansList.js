import React, { useEffect } from 'react';
import { motion } from 'framer-motion';
import { Progress } from 'recharts'; 
import useRiskStore from '../stores/riskStore';
import { Link } from 'react-router-dom';
import Confetti from 'react-confetti';
import toast from 'react-hot-toast';

const PlansList = () => {
  const { plans, fetchPlans, updatePlan } = useRiskStore(); 
  const [showConfetti, setShowConfetti] = React.useState(false);

  useEffect(() => {
    fetchPlans(); 
  }, [fetchPlans]);

  const handleProgressUpdate = (planId, newProgress) => {
    updatePlan(planId, { progress: newProgress });
    if (newProgress === 100) {
      setShowConfetti(true);
      toast.success('Plan completed! Badge awarded 🎉');
      setTimeout(() => setShowConfetti(false), 3000); 
    }
  };

  return (
    <div className="space-y-6 neumorphic-card p-6">
      {showConfetti && <Confetti recycle={false} numberOfPieces={200} />}
      <h2 className="text-2xl font-bold mb-4">Mitigation Plans</h2>
      <Link to="/plans/create" className="neumorphic-button p-2 inline-block">Create New Plan</Link>
      <ul className="space-y-4">
        {plans.map((plan) => (
          <motion.li
            key={plan.id}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            className="neumorphic-card p-4 rounded-lg shadow-md"
          >
            <h3 className="font-semibold">{plan.action}</h3>
            <p>Assignee: {plan.assignee_id}</p>
            <p>Deadline: {new Date(plan.deadline).toLocaleDateString()}</p>
            <div className="flex items-center space-x-4">
              <Progress
                percent={plan.progress}
                status={plan.progress === 100 ? 'success' : 'active'}
                showInfo
                strokeColor={{ '0%': '#3b82f6', '100%': '#10b981' }} // Beautiful gradient
                className="w-full"
              />
              <input
                type="number"
                min="0"
                max="100"
                value={plan.progress}
                onChange={(e) => handleProgressUpdate(plan.id, parseInt(e.target.value))}
                className="p-2 border rounded w-20"
              />
            </div>
            <p>Badges Earned: {plan.badges}</p>
            <Link to={`/plans/${plan.id}`} className="text-blue-500">Details</Link>
          </motion.li>
        ))}
      </ul>
      {plans.length === 0 && <p className="text-gray-500 text-center">No mitigation plans available. Create one!</p>}
    </div>
  );
};

export default PlansList;