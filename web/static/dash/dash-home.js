  (function() {
    function attachUVTooltip(chartEl) {
      var svg = chartEl.querySelector('svg');
      var tooltipEl = chartEl.querySelector('[data-uv-tooltip]');
      if (!svg || !tooltipEl || typeof bindSVGHitboxTooltip !== 'function') {
        return;
      }

      var tickLines = svg.querySelectorAll('line');

      function clearActiveTick() {
        tickLines.forEach(function(line) {
          line.classList.remove('is-active-tick');
        });
      }

      function getTickIndex(hitbox) {
        var index = parseInt(hitbox.getAttribute('data-index') || '', 10);
        if (isNaN(index) || index < 0) {
          return -1;
        }
        return index;
      }

      function setActiveTick(index) {
        clearActiveTick();
        if (index >= 0 && index < tickLines.length) {
          tickLines[index].classList.add('is-active-tick');
        }
      }

      bindSVGHitboxTooltip({
        container: chartEl,
        svg: svg,
        tooltip: tooltipEl,
        boundFlag: 'uvTooltipBound',
        hitboxSelector: 'rect[data-uv]',
        pointSelector: 'circle',
        getIndex: getTickIndex,
        getText: function(hitbox) {
          var label = hitbox.getAttribute('data-label') || '';
          var uv = hitbox.getAttribute('data-uv') || '0';
          var tsRaw = hitbox.getAttribute('data-ts');
          if (!label && tsRaw) {
            var ts = parseInt(tsRaw, 10);
            if (!isNaN(ts) && ts > 0) {
              var dt = new Date(ts * 1000);
              label = dt.toLocaleString();
            }
          }
          return label ? (label + ' · 访问量 ' + uv) : ('访问量 ' + uv);
        },
        onActivate: function(_hitbox, index) {
          setActiveTick(index);
        },
        onDeactivate: clearActiveTick,
      });
    }

    function initUVTooltips() {
      document.querySelectorAll('[data-uv-chart]').forEach(attachUVTooltip);
    }

    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', initUVTooltips, { once: true });
    } else {
      initUVTooltips();
    }
  })();
