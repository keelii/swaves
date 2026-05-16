(function() {
  var optionsInput = document.getElementById('setting-options');
  var defaultContainer = document.getElementById('default-option-value-container');
  var typeSelect = document.getElementById('setting-type');
  var newValueInput = document.getElementById('setting-new-value');
  var newValueToggle = document.getElementById('setting-new-value-toggle');
  var rangeInput = document.querySelector('[data-role="setting-range-input"]');
  var rangeOutput = document.getElementById('setting-range-output');

  function bindMaskToggle(buttonID, inputID) {
    var button = document.getElementById(buttonID);
    var input = document.getElementById(inputID);
    if (!button || !input) {
      return;
    }
    button.addEventListener('click', function() {
      var isPassword = (input.getAttribute('type') || '').toLowerCase() === 'password';
      input.setAttribute('type', isPassword ? 'text' : 'password');
      button.textContent = isPassword ? '隐藏' : '显示';
    });
  }

  function parseOptionItems(raw) {
    var text = (raw || '').trim();
    if (!text) {
      return [];
    }

    var parsed;
    try {
      parsed = JSON.parse(text);
    } catch (error) {
      return [];
    }

    if (!Array.isArray(parsed)) {
      return [];
    }

    var seen = {};
    var result = [];
    parsed.forEach(function(item) {
      if (!item || typeof item !== 'object') {
        return;
      }
      var value = String(item.value == null ? '' : item.value).trim();
      if (!value || seen[value]) {
        return;
      }
      seen[value] = true;
      var label = String(item.label == null ? value : item.label).trim();
      if (!label) {
        label = value;
      }
      result.push({
        value: value,
        label: label,
      });
    });
    return result;
  }

  function readDefaultOptionCurrent() {
    if (!defaultContainer) {
      return '';
    }
    var field = defaultContainer.querySelector('[name="default_option_value"]');
    if (field) {
      return field.value || '';
    }
    return defaultContainer.getAttribute('data-current-value') || '';
  }

  function renderDefaultOptionControl() {
    if (!optionsInput || !defaultContainer) {
      return;
    }

    var current = readDefaultOptionCurrent();
    defaultContainer.setAttribute('data-current-value', current);

    var options = parseOptionItems(optionsInput.value);
    defaultContainer.innerHTML = '';

    if (options.length === 0) {
      var input = document.createElement('input');
      input.type = 'text';
      input.className = 'ui-input';
      input.id = 'default_option_value';
      input.name = 'default_option_value';
      input.placeholder = '默认值（可选）';
      input.value = current;
      defaultContainer.appendChild(input);
      return;
    }

    var select = document.createElement('select');
    select.className = 'ui-select';
    select.id = 'default_option_value';
    select.name = 'default_option_value';

    var emptyOption = document.createElement('option');
    emptyOption.value = '';
    emptyOption.textContent = '不设置';
    select.appendChild(emptyOption);

    options.forEach(function(item) {
      var option = document.createElement('option');
      option.value = item.value;
      option.textContent = item.label;
      if (item.value === current) {
        option.selected = true;
      }
      select.appendChild(option);
    });

    defaultContainer.appendChild(select);
  }

  function resolveValueInputType(settingType) {
    switch (settingType) {
      case 'number':
        return 'number';
      case 'date':
        return 'date';
      case 'time':
        return 'time';
      case 'datetime':
        return 'datetime-local';
      case 'url':
        return 'url';
      default:
        return 'text';
    }
  }

  function syncNewValueInputByType() {
    if (!typeSelect || !newValueInput) {
      return;
    }

    var currentType = (typeSelect.value || '').toLowerCase();
    var shouldMask = currentType === 'password' || currentType === 'secret';
    var inputType = shouldMask ? 'password' : resolveValueInputType(currentType);
    newValueInput.setAttribute('type', inputType);

    if (!newValueToggle) {
      return;
    }

    if (shouldMask) {
      newValueToggle.hidden = false;
    } else {
      newValueToggle.hidden = true;
      newValueToggle.textContent = '显示';
    }
  }

  if (optionsInput && defaultContainer) {
    optionsInput.addEventListener('input', renderDefaultOptionControl);
    optionsInput.addEventListener('change', renderDefaultOptionControl);
    renderDefaultOptionControl();
  }

  if (typeSelect && newValueInput) {
    typeSelect.addEventListener('change', syncNewValueInputByType);
    syncNewValueInputByType();
  }

  if (rangeInput && rangeOutput) {
    var syncRangeOutput = function() {
      rangeOutput.textContent = rangeInput.value || '';
    };
    rangeInput.addEventListener('input', syncRangeOutput);
    syncRangeOutput();
  }

  bindMaskToggle('setting-secret-toggle', 'setting-secret-value');
  bindMaskToggle('setting-new-value-toggle', 'setting-new-value');
})();
