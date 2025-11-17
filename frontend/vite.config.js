import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  css: {
    postcss: {
      plugins: [require('tailwindcss'), require('autoprefixer')],
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8000' 
    }
  },
  build: {
    outDir: 'dist',
    sourcemap: false 
  }
});