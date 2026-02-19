/**
 * oat - Tooltip Enhancement
 * Converts title attributes to data-tooltip for custom styling.
 * Progressive enhancement: native title works without JS.
 */

document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('[data-title]').forEach(el => {
    const text = el.getAttribute('data-title');
    if (text) {
      el.setAttribute('data-tooltip', text);
      if (!el.hasAttribute('aria-label')) {
        el.setAttribute('aria-label', text);
      }
      el.removeAttribute('data-title');
    }
  });
});
