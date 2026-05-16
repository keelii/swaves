document.addEventListener('DOMContentLoaded', function() {
  var preset = document.getElementById('schedule-preset');
  var wrap = document.getElementById('schedule-custom-wrap');
  var custom = document.getElementById('schedule-custom');
  var valueInput = document.getElementById('schedule-value');
  var form = document.getElementById('form');

  if (!preset || !wrap || !custom || !valueInput || !form) {
    return;
  }

  function syncScheduleValue() {
    valueInput.value = preset.value === '__custom__' ? custom.value : preset.value;
  }

  function toggleScheduleInput() {
    var isCustom = preset.value === '__custom__';
    wrap.hidden = !isCustom;
    syncScheduleValue();
  }

  toggleScheduleInput();

  preset.addEventListener('change', function() {
    toggleScheduleInput();
    if (preset.value === '__custom__') {
      custom.focus();
    }
  });

  custom.addEventListener('input', syncScheduleValue);
  custom.addEventListener('change', syncScheduleValue);

  form.addEventListener('submit', function(event) {
    if (preset.value === '__custom__' && !custom.value.trim()) {
      event.preventDefault();
      custom.focus();
      return;
    }
    syncScheduleValue();
  });
});
