document.addEventListener('DOMContentLoaded', () => {
  AOS.init();  // Init AOS animations

  // Dark/Light Toggle
  const themeToggle = document.getElementById('theme-toggle');
  const html = document.documentElement;
  const icon = themeToggle.querySelector('i');
  if (localStorage.theme === 'light') {
    html.classList.remove('dark');
    icon.classList.replace('fa-moon', 'fa-sun');
  }
  themeToggle.addEventListener('click', () => {
    html.classList.toggle('dark');
    localStorage.theme = html.classList.contains('dark') ? 'dark' : 'light';
    icon.classList.toggle('fa-moon');
    icon.classList.toggle('fa-sun');
  });

  // Tabs
  const tabButtons = document.querySelectorAll('.tab-button');
  const tabContents = document.querySelectorAll('.tab-content');
  tabButtons.forEach(button => {
    button.addEventListener('click', () => {
      tabButtons.forEach(btn => {
        btn.classList.remove('bg-blue-700');
        btn.classList.add('bg-gray-700');
        btn.setAttribute('aria-selected', 'false');
      });
      button.classList.add('bg-blue-700');
      button.classList.remove('bg-gray-700');
      button.setAttribute('aria-selected', 'true');

      tabContents.forEach(content => content.classList.add('hidden'));
      document.getElementById(button.dataset.tab).classList.remove('hidden');
    });
  });

  // Modal
  const openModal = document.getElementById('open-demo-modal');
  const closeModal = document.getElementById('close-demo-modal');
  const modal = document.getElementById('demo-modal');
  openModal.addEventListener('click', () => modal.classList.remove('hidden'));
  closeModal.addEventListener('click', () => modal.classList.add('hidden'));
  modal.addEventListener('click', (e) => { if (e.target === modal) modal.classList.add('hidden'); });

  // Parallax Hero
  window.addEventListener('scroll', () => {
    document.documentElement.style.setProperty('--scroll-y', window.scrollY);
  });

  // Testimonials Carousel (Auto-scroll every 5s)
  const carouselInner = document.querySelector('.carousel-inner');
  let currentIndex = 0;
  const items = document.querySelectorAll('.carousel-item');
  setInterval(() => {
    currentIndex = (currentIndex + 1) % items.length;
    carouselInner.style.transform = `translateX(-${currentIndex * 100 / items.length}%)`;
  }, 5000);

  // Smooth Scroll
  document.querySelectorAll('a[href^="#"]').forEach(anchor => {
    anchor.addEventListener('click', (e) => {
      e.preventDefault();
      document.querySelector(anchor.getAttribute('href')).scrollIntoView({ behavior: 'smooth' });
    });
  });
});