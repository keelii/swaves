/**
 * Sidebar toggle handler
 * Toggles data-sidebar-open on layout when toggle button is clicked
 */
document.addEventListener('click', (e) => {
  const toggle = e.target.closest('[data-sidebar-toggle]');
  if (toggle) {
    const layout = toggle.closest('[data-sidebar-layout]');
    layout?.toggleAttribute('data-sidebar-open');
  }
});
