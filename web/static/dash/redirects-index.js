(function() {
  var trigger = document.getElementById('redirect-import-trigger');
  var input = document.getElementById('redirect-import-file');
  var form = document.getElementById('redirect-import-form');
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
