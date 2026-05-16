document.addEventListener('DOMContentLoaded', function() {
  var settingsForm = document.getElementById('settings-all-form');
  var settingShortcuts = document.getElementById('setting-shortcuts');
  var basePathContainer = document.querySelector('[data-setting-code="base_path"]');

  function normalizeLower(raw) {
    raw = (raw || '').trim().toLowerCase();
    return raw;
  }

  function readFieldValue(name) {
    var field = document.querySelector('[name="' + name + '"]');
    if (!field) {
      return '';
    }
    return (field.value || '').trim();
  }

  function readChoiceValue(name) {
    var checked = document.querySelector('[name="' + name + '"]:checked');
    if (checked) {
      return (checked.value || '').trim();
    }
    return readFieldValue(name);
  }

  function setCardState(cardCode, options) {
    var card = document.querySelector('.settings-card[data-card-code="' + cardCode + '"]');
    var noteEl;
    var note;
    var manualState;
    var collapsed;

    if (!card) {
      return;
    }

    noteEl = card.querySelector('[data-card-note]');
    card.classList.toggle('is-emphasis', !!options.emphasis);
    card.classList.toggle('is-muted', !!options.muted);

    if (!noteEl) {
      return;
    }

    note = (options.note || '').trim();
    if (!note) {
      noteEl.textContent = '';
      noteEl.hidden = true;
    } else {
      noteEl.textContent = note;
      noteEl.hidden = false;
    }

    if (!options.collapsible) {
      card.removeAttribute('data-collapsible');
      card.removeAttribute('data-collapsed');
      return;
    }

    card.setAttribute('data-collapsible', '1');
    manualState = (card.getAttribute('data-card-manual') || '').trim();
    if (options.emphasis) {
      collapsed = false;
    } else if (manualState === 'expanded') {
      collapsed = false;
    } else if (manualState === 'collapsed') {
      collapsed = true;
    } else {
      collapsed = !!options.collapseWhenMuted;
    }
    if (collapsed) {
      card.setAttribute('data-collapsed', '1');
    } else {
      card.removeAttribute('data-collapsed');
    }
    syncCardToggleButton(card);
  }

  function syncCardToggleButton(card) {
    var button;
    var collapsed;

    if (!card) {
      return;
    }

    button = card.querySelector('[data-card-toggle]');
    if (!button) {
      return;
    }

    collapsed = card.getAttribute('data-collapsed') === '1';
    button.textContent = collapsed ? '展开' : '收起';
    button.setAttribute('aria-expanded', collapsed ? 'false' : 'true');
  }

  function syncAssetProviderCards() {
    var provider = normalizeLower(readChoiceValue('setting_asset_default_provider'));

    setCardState('provider', {
      emphasis: true,
      muted: false,
      note: '当前默认资源服务：' + (provider === 'imagekit' ? 'ImageKit' : 'S.EE'),
    });
    setCardState('see', {
      emphasis: provider === 'see',
      muted: provider !== 'see',
      collapsible: true,
      collapseWhenMuted: provider !== 'see',
      note: provider === 'see'
        ? '当前上传默认使用这组 S.EE 配置。'
        : '切换默认资源服务到 S.EE 后会使用这组配置。',
    });
    setCardState('imagekit', {
      emphasis: provider === 'imagekit',
      muted: false,
      collapsible: true,
      collapseWhenMuted: provider !== 'imagekit',
      note: provider === 'imagekit'
        ? '当前上传默认使用这组 ImageKit 配置。'
        : '切换默认资源服务到 ImageKit 后会使用这组配置。',
    });
  }

  function syncBackupProviderCards() {
    var enabled = normalizeLower(readChoiceValue('setting_sync_push_enabled'));
    var provider = normalizeLower(readChoiceValue('setting_sync_push_provider'));
    var providerLabel = provider === 'imagekit' ? 'ImageKit' : 'S3';
    var remoteNote = '';
    var s3Note = '';

    if (enabled === '1') {
      remoteNote = '当前远程备份服务：' + providerLabel;
      if (provider === 'imagekit') {
        remoteNote += '，凭证复用“资源与云服务”中的 ImageKit 配置。';
      }
    } else {
      remoteNote = '远程备份当前关闭，展开后可开启并配置服务。';
    }

    if (provider === 's3') {
      s3Note = enabled === '1'
        ? '当前远程备份会使用这组 S3 配置。'
        : '当前未启用远程备份，S3 配置会在开启后生效。';
    } else {
      s3Note = '当前远程备份未使用 S3。';
    }

    setCardState('remote', {
      emphasis: enabled === '1',
      muted: enabled !== '1',
      collapsible: true,
      collapseWhenMuted: enabled !== '1',
      note: remoteNote,
    });
    setCardState('s3', {
      emphasis: enabled === '1' && provider === 's3',
      muted: false,
      collapsible: true,
      collapseWhenMuted: provider !== 's3',
      note: s3Note,
    });
  }

  function syncDynamicCardStates() {
    syncAssetProviderCards();
    syncBackupProviderCards();
  }

  if (settingShortcuts) {
    settingShortcuts.addEventListener('click', function(event) {
      var button = event.target.closest('button');
      var options;

      if (!button) {
        return;
      }

      event.preventDefault();
      options = {
        setting_post_url_prefix: button.getAttribute('data-prefix') || '',
        setting_post_url_name: button.getAttribute('data-name') || '',
        setting_post_url_ext: button.getAttribute('data-ext') || '',
      };

      Object.keys(options).forEach(function(code) {
        var input = document.querySelector('[name="' + code + '"]');
        if (input) {
          input.value = options[code];
        }
      });
      syncPrefixLegends();
    });
  }

  document.querySelectorAll('input[data-range-value]').forEach(function(input) {
    var output = input.parentElement ? input.parentElement.querySelector('.range-value') : null;
    if (!output) {
      return;
    }

    function updateRangeOutput() {
      output.textContent = input.value + (input.dataset.unit || '');
    }

    input.addEventListener('input', updateRangeOutput);
    input.addEventListener('change', updateRangeOutput);
    updateRangeOutput();
  });

  function normalizePrefixLiteral(raw) {
    raw = (raw || '').trim();
    raw = raw.replace(/^\/+|\/+$/g, '');
    if (!raw) {
      return '/';
    }
    return raw;
  }

  function syncPrefixLegends() {
    var baseInput = basePathContainer ? basePathContainer.querySelector('input,textarea,select') : null;
    var basePrefix = normalizePrefixLiteral(baseInput ? baseInput.value : '');

    document.querySelectorAll('.settings-prefix-text').forEach(function(node) {
      node.textContent = basePrefix !== '/' ? '/' + basePrefix : '';
    });

    document.querySelectorAll('[data-prefix-default]').forEach(function(input) {
      var sourceCode = (input.getAttribute('data-prefix-source-code') || '').trim();
      var prefixText = (input.getAttribute('data-prefix-default') || '').trim();
      var prefixGroup;
      var prefixNode;

      if (sourceCode === 'base_path') {
        // Only URL path prefixes derive their legend from base_path; literal
        // prefixes such as ".cache/" must stay exactly as configured.
        prefixText = basePrefix === '/' ? '/' : '/' + basePrefix + '/';
      } else if (!prefixText) {
        prefixText = '/';
      }

      prefixGroup = input.closest('.ui-input-prefix-group');
      if (!prefixGroup) {
        return;
      }

      prefixNode = prefixGroup.querySelector('.ui-input-prefix');
      if (prefixNode) {
        prefixNode.textContent = prefixText;
      }
    });
  }

  if (basePathContainer) {
    basePathContainer.querySelectorAll('input,textarea,select').forEach(function(input) {
      input.addEventListener('input', syncPrefixLegends);
      input.addEventListener('change', syncPrefixLegends);
    });
  }
  syncPrefixLegends();

  if (settingsForm) {
    settingsForm.addEventListener('input', function() {
      syncDynamicCardStates();
    });
    settingsForm.addEventListener('change', function() {
      syncDynamicCardStates();
    });
  }
  syncDynamicCardStates();

  document.addEventListener('click', function(event) {
    var button = event.target.closest('[data-card-toggle]');
    var card;
    var collapsed;

    if (!button) {
      return;
    }

    card = button.closest('.settings-card');
    if (!card) {
      return;
    }

    collapsed = card.getAttribute('data-collapsed') === '1';
    if (collapsed) {
      card.setAttribute('data-card-manual', 'expanded');
      card.removeAttribute('data-collapsed');
    } else {
      card.setAttribute('data-card-manual', 'collapsed');
      card.setAttribute('data-collapsed', '1');
    }
    syncCardToggleButton(card);
  });

  document.addEventListener('click', function(event) {
    var button = event.target.closest('.settings-secret-toggle');
    var targetID;
    var input;
    var isPassword;

    if (!button) {
      return;
    }

    targetID = (button.getAttribute('data-target-id') || '').trim();
    if (!targetID) {
      return;
    }

    input = document.getElementById(targetID);
    if (!input) {
      return;
    }

    isPassword = (input.getAttribute('type') || '').toLowerCase() === 'password';
    if (isPassword) {
      input.setAttribute('type', 'text');
      button.textContent = button.getAttribute('data-hide-text') || '隐藏';
      return;
    }

    input.setAttribute('type', 'password');
    button.textContent = button.getAttribute('data-show-text') || '显示';
  });
});
