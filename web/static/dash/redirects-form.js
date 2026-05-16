  (function() {
    function initRedirectTargetPicker() {
    var dialog = document.getElementById('redirect-target-picker-dialog');
    var search = document.getElementById('redirect-target-picker-search');
    var toInput = document.getElementById('redirect-to-path');
    var body = document.getElementById('redirect-target-picker-body');
    var openButton = document.getElementById('redirect-target-picker-open');
    var emptyState = document.getElementById('redirect-target-picker-empty');
    var dialogAPI = window.DashAppUI.dialog;

    if (!dialog || !search || !toInput || !body) {
      return;
    }
    if (dialog.getAttribute('data-picker-bound') === '1') {
      return;
    }
    dialog.setAttribute('data-picker-bound', '1');

    function getRows() {
      return body.querySelectorAll('[data-target-row]');
    }

    function openPickerDialog() {
      return dialogAPI.open('redirect-target-picker-dialog', openButton || null);
    }

    function closePickerDialog() {
      return dialogAPI.close('redirect-target-picker-dialog');
    }

    function updateEmptyState(visibleCount) {
      if (!emptyState) {
        return;
      }
      if (visibleCount > 0) {
        emptyState.setAttribute('hidden', '');
        return;
      }
      emptyState.removeAttribute('hidden');
    }

    function dispatchInputEvent(target, eventName) {
      if (!target || typeof target.dispatchEvent !== 'function') {
        return;
      }
      target.dispatchEvent(new Event(eventName, { bubbles: true }));
    }

    function applyTargetURL(targetURL) {
      toInput.value = targetURL;
      dispatchInputEvent(toInput, 'input');
      dispatchInputEvent(toInput, 'change');
      closePickerDialog();
      toInput.focus();
      var end = toInput.value.length;
      toInput.setSelectionRange(end, end);
    }

    function filterRows() {
      var keyword = (search.value || '').trim().toLowerCase();
      var rows = getRows();
      var visibleCount = 0;

      for (var i = 0; i < rows.length; i += 1) {
        var row = rows[i];
        if (!keyword) {
          row.hidden = false;
          visibleCount += 1;
          continue;
        }

        var text = (row.getAttribute('data-search-text') || row.textContent || '').toLowerCase();
        row.hidden = text.indexOf(keyword) < 0;
        if (!row.hidden) {
          visibleCount += 1;
        }
      }
      updateEmptyState(visibleCount);
    }

    if (openButton) {
      openButton.addEventListener('click', function(event) {
        event.preventDefault();
        if (openPickerDialog()) {
          search.value = '';
          filterRows();
          window.setTimeout(function() {
            search.focus();
          }, 0);
        }
      });
    }

    body.addEventListener('click', function(event) {
      var button = event.target.closest('[data-role="redirect-target-picker-choose"]');
      if (!button || !body.contains(button)) {
        return;
      }

      event.preventDefault();
      var targetURL = (button.getAttribute('data-target-url') || '').trim();
      if (!targetURL) {
        return;
      }
      applyTargetURL(targetURL);
    });

    search.addEventListener('input', filterRows);
    search.addEventListener('search', filterRows);
    filterRows();
    }

    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', initRedirectTargetPicker, { once: true });
      return;
    }
    initRedirectTargetPicker();
  })();
  
