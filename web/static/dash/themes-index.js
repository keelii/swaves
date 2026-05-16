(function() {
  var trigger = document.getElementById('theme-import-trigger');
  var input = document.getElementById('theme-import-file');
  var form = document.getElementById('theme-import-form');
  if (!trigger || !input || !form) {
    return;
  }

  trigger.addEventListener('click', function() {
    input.click();
  });

  input.addEventListener('change', function() {
    if (!input.files || input.files.length === 0) {
      return;
    }
    form.submit();
  });
})();
