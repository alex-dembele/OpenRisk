import { create } from 'zustand';
import toast from 'react-hot-toast';
import Confetti from 'react-confetti';
import axios from 'axios';

const useRiskStore = create((set) => ({
  risks: [],
  plans: [],
  history: [],
  loading: false,
  error: null,
  fetchRisks: async () => {
    set({ loading: true });
    try {
      const res = await axios.get('/api/risks');
      set({ risks: res.data, loading: false });
    } catch (err) {
      set({ error: err.message, loading: false });
      toast.error(err.message);
    }
  },
  createRisk: async (data) => {
    try {
      const res = await axios.post('/api/risks', data);
      set((state) => ({ risks: [...state.risks, res.data] }));
      toast.success('Risk created!');
    } catch (err) {
      toast.error(err.message);
    }
  },
  updateRisk: async (id, data) => {
    try {
      const res = await axios.put(`/api/risks/${id}`, data);
      set((state) => ({
        risks: state.risks.map(r => r.id === id ? res.data : r)
      }));
      if (data.status === 'Mitigated') {
        
        Confetti({ recycle: false });
        toast.success('Risk mitigated! Badge awarded 🎉');
      }
    } catch (err) {
      toast.error(err.message);
    }
  },
  
  exportPDF: async () => {
    try {
      const res = await axios.post('/api/exports/pdf', { risks: useRiskStore.getState().risks }, { responseType: 'blob' });
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', 'openrisk-report.pdf');
      document.body.appendChild(link);
      link.click();
      toast.success('PDF exported!');
    } catch (err) {
      toast.error(err.message);
    }
  },
  
}));

export default useRiskStore;