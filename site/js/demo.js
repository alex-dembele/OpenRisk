// JS for interactivity: Tabs and Modal
document.addEventListener('DOMContentLoaded', () => {
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

  // Modal for Demo
  const openModalBtn = document.getElementById('open-demo-modal');
  const closeModalBtn = document.getElementById('close-demo-modal');
  const modal = document.getElementById('demo-modal');

  openModalBtn.addEventListener('click', () => {
    modal.classList.remove('hidden');
  });

  closeModalBtn.addEventListener('click', () => {
    modal.classList.add('hidden');
  });

  // Smooth Scroll (if links added)
  document.querySelectorAll('a[href^="#"]').forEach(anchor => {
    anchor.addEventListener('click', function (e) {
      e.preventDefault();
      document.querySelector(this.getAttribute('href')).scrollIntoView({
        behavior: 'smooth'
      });
    });
  });
});