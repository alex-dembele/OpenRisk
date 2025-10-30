document.addEventListener('DOMContentLoaded', () => {
  AOS.init({ duration: 800, once: true }); // AOS animations

  // Tabs
  const tabButtons = document.querySelectorAll('.tab-button');
  const tabContents = document.querySelectorAll('.tab-content');

  tabButtons.forEach(button => {
    button.addEventListener('click', () => {
      tabButtons.forEach(btn => btn.classList.remove('active'));
      button.classList.add('active');
      tabContents.forEach(content => content.classList.add('hidden'));
      document.getElementById(button.dataset.tab).classList.remove('hidden');
    });
  });

  // Modal
  const openModalBtn = document.getElementById('open-demo-modal');
  const closeModalBtn = document.getElementById('close-demo-modal');
  const modal = document.getElementById('demo-modal');

  openModalBtn.addEventListener('click', () => modal.classList.remove('hidden'));
  closeModalBtn.addEventListener('click', () => modal.classList.add('hidden'));

  // Accordion FAQ
  const accordionHeaders = document.querySelectorAll('.accordion-header');
  accordionHeaders.forEach(header => {
    header.addEventListener('click', () => {
      const content = header.nextElementSibling;
      content.classList.toggle('hidden');
      const icon = header.querySelector('span');
      icon.textContent = content.classList.contains('hidden') ? '+' : '-';
    });
  });

  // Dark Mode Toggle
  const toggle = document.createElement('button');
  toggle.id = 'dark-mode-toggle';
  toggle.className = 'bg-blue-600 text-white px-4 py-2 rounded-full';
  toggle.textContent = 'Dark Mode';
  document.body.appendChild(toggle);

  toggle.addEventListener('click', () => {
    const currentTheme = document.body.dataset.theme;
    document.body.dataset.theme = currentTheme === 'light' ? 'dark' : 'light';
    toggle.textContent = currentTheme === 'light' ? 'Light Mode' : 'Dark Mode';
  });

  // Smooth Scroll
  document.querySelectorAll('a[href^="#"]').forEach(anchor => {
    anchor.addEventListener('click', e => {
      e.preventDefault();
      document.querySelector(anchor.getAttribute('href')).scrollIntoView({ behavior: 'smooth' });
    });
  });
});