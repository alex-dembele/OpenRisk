import React from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import Dashboard from './components/Dashboard';
import RisksList from './components/RisksList';
import CreateRisk from './components/CreateRisk';
import RiskDetail from './components/RiskDetail';
import PlansList from './components/PlansList';
import History from './components/History';
import Exports from './components/Exports';
import Sidebar from './components/Sidebar';
import { useTheme } from './stores/themeStore'; 

function App() {
  const { theme } = useTheme();
  return (
    <div className={`min-h-screen ${theme === 'dark' ? 'dark' : ''}`}>
      <Router>
        <Sidebar />
        <main className="ml-64 p-6">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/risks" element={<RisksList />} />
            <Route path="/risks/create" element={<CreateRisk />} />
            <Route path="/risks/:id" element={<RiskDetail />} />
            <Route path="/plans" element={<PlansList />} />
            <Route path="/history/:riskId" element={<History />} />
            <Route path="/exports" element={<Exports />} />
          </Routes>
        </main>
      </Router>
    </div>
  );
}

export default App;