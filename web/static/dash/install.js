  document.addEventListener('DOMContentLoaded', function() {
    var installForm = document.querySelector('.install-form');
    var basePathContainer = document.querySelector('[data-setting-code="base_path"]');
    var installPostURLPreview = document.getElementById('install-post-url-preview');

    document.querySelectorAll('[data-range-value]').forEach(function(input) {
      var output = input.parentElement && input.parentElement.querySelector('.range-value');
      var unit = input.getAttribute('data-unit') || '';

      function syncRange() {
        if (!output) {
          return;
        }
        output.textContent = (input.value || '') + unit;
      }

      input.addEventListener('input', syncRange);
      syncRange();
    });

    document.querySelectorAll('.install-secret-toggle').forEach(function(button) {
      button.addEventListener('click', function() {
        var input = document.getElementById(button.getAttribute('data-target-id'));
        if (!input) {
          return;
        }

        var isPassword = (input.getAttribute('type') || '').toLowerCase() === 'password';
        input.setAttribute('type', isPassword ? 'text' : 'password');
        button.textContent = isPassword ? button.getAttribute('data-hide-text') : button.getAttribute('data-show-text');
      });
    });

    function normalizePrefixLiteral(raw) {
      raw = (raw || '').trim();
      raw = raw.replace(/^\/+|\/+$/g, '');
      if (!raw) {
        return '/';
      }
      return raw;
    }

    function readPreviewDefault(name) {
      if (!installForm) {
        return '';
      }
      return (installForm.getAttribute('data-preview-' + name) || '').trim();
    }

    function readFieldValue(name, fallbackValue) {
      var field = document.querySelector('[name="' + name + '"]');
      if (!field) {
        return (fallbackValue || '').trim();
      }
      return (field.value || '').trim();
    }

    function splitPathSegments(raw) {
      raw = (raw || '').trim();
      if (!raw) {
        return [];
      }

      return raw
        .split('/')
        .map(function(part) {
          return (part || '').trim();
        })
        .filter(function(part) {
          return part !== '';
        });
    }

    function joinAbsoluteParts(parts) {
      var segments = [];

      parts.forEach(function(part) {
        segments = segments.concat(splitPathSegments(part));
      });

      if (!segments.length) {
        return '/';
      }

      return '/' + segments.join('/');
    }

    function buildInstallPostURLPreview() {
      var siteURL = readFieldValue('setting_site_url', readPreviewDefault('site-url')).replace(/\/+$/g, '');
      var basePath = readFieldValue('setting_base_path', readPreviewDefault('base-path'));
      var postPrefix = readFieldValue('setting_post_url_prefix', readPreviewDefault('post-prefix'));
      var postName = readFieldValue('setting_post_url_name', readPreviewDefault('post-name'));
      var postExt = readFieldValue('setting_post_url_ext', readPreviewDefault('post-ext'));
      var postPath;

      postPrefix = postPrefix.split('{datetime}').join('2024/01/02');

      if (!postName) {
        postName = '{slug}';
      }
      postName = postName.split('{slug}').join('hello-world');
      postName = postName.split('{id}').join('123');
      postName = postName.split('{title}').join('my-first-post');
      if (!(postName || '').trim()) {
        postName = 'hello-world';
      }

      postPath = joinAbsoluteParts([basePath, postPrefix, postName + postExt]);
      if (!siteURL) {
        return postPath;
      }

      return siteURL + postPath;
    }

    function syncInstallPostURLPreview() {
      var bodyNode;

      if (!installPostURLPreview) {
        return;
      }

      bodyNode = installPostURLPreview.querySelector('.ui-alert-body');
      if (!bodyNode) {
        return;
      }

      bodyNode.textContent = buildInstallPostURLPreview();
    }

    function syncPrefixLegends() {
      var baseInput = basePathContainer ? basePathContainer.querySelector('input,textarea,select') : null;
      var basePrefix = normalizePrefixLiteral(baseInput ? baseInput.value : '');

      document.querySelectorAll('[data-prefix-default]').forEach(function(input) {
        var sourceCode = (input.getAttribute('data-prefix-source-code') || '').trim();
        var prefixText = (input.getAttribute('data-prefix-default') || '').trim();
        var prefixGroup = input.closest('.ui-input-prefix-group');
        var prefixNode;

        if (sourceCode === 'base_path') {
          // Only URL path prefixes derive their legend from base_path; literal
          // prefixes such as ".cache/" must stay exactly as configured.
          prefixText = basePrefix === '/' ? '/' : '/' + basePrefix + '/';
        } else if (!prefixText) {
          prefixText = '/';
        }

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

    if (installForm) {
      installForm.addEventListener('input', syncInstallPostURLPreview);
      installForm.addEventListener('change', syncInstallPostURLPreview);
    }
    syncInstallPostURLPreview();
  });
  
