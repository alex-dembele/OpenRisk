module.exports = {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: '#1E40AF',
        accent: '#10B981',
        neutral: '#F3F4F6',
        'dark-neutral': '#1F2937'
      },
      boxShadow: {
        neumorphic: '9px 9px 16px rgba(189,189,189,0.6), -9px -9px 16px rgba(255,255,255,0.5)',
        'neumorphic-dark': '9px 9px 16px rgba(0,0,0,0.6), -9px -9px 16px rgba(55,65,81,0.5)'
      },
      backgroundImage: {
        'bold-gradient': 'linear-gradient(145deg, #3b82f6, #10b981)'
      }
    }
  },
  plugins: []
};