  (function() {
    var monitorRoot = document.querySelector('[data-monitor-root]');
    if (!monitorRoot) {
      return;
    }
    var monitorAPIURL = (monitorRoot.getAttribute('data-monitor-api-url') || '').trim();
    var monitorPageURL = (monitorRoot.getAttribute('data-monitor-page-url') || '').trim();

    var searchParams = new URLSearchParams(window.location.search || '');
    var activeGranularity = searchParams.get('granularity') || '1m';

    var availableGranularities = Array.prototype.slice.call(
      document.querySelectorAll('[data-monitor-granularity-tabs] [data-granularity]')
    ).map(function(el) {
      return el.getAttribute('data-granularity');
    });
    if (availableGranularities.length > 0 && availableGranularities.indexOf(activeGranularity) === -1) {
      activeGranularity = availableGranularities[0];
    }

    var chartsEl = document.querySelector('[data-monitor-charts]');
    var errorEl = document.querySelector('[data-monitor-error]');
    var latestTsEl = document.querySelector('[data-latest-ts]');

    function formatPercent(value) {
      var num = Number(value || 0);
      return num.toFixed(2) + '%';
    }

    function formatBytes(value) {
      var num = Number(value || 0);
      if (!Number.isFinite(num) || num <= 0) {
        return '0 B';
      }
      var units = ['B', 'KB', 'MB', 'GB', 'TB'];
      var index = 0;
      while (num >= 1024 && index < units.length - 1) {
        num = num / 1024;
        index += 1;
      }
      if (index === 0) {
        return Math.round(num) + ' ' + units[index];
      }

      var rounded = Number(num.toPrecision(2));
      var text = '';
      if (rounded >= 10) {
        text = rounded.toFixed(0);
      } else {
        text = rounded.toFixed(1);
      }
      return text + ' ' + units[index];
    }

    function formatNumber(value) {
      var num = Number(value || 0);
      if (!Number.isFinite(num)) {
        return '-';
      }
      return String(Math.round(num));
    }

    function formatDateTime(ts) {
      var unix = Number(ts || 0);
      if (!Number.isFinite(unix) || unix <= 0) {
        return '-';
      }
      return new Date(unix * 1000).toLocaleString();
    }

    function updateActiveTab() {
      document.querySelectorAll('[data-monitor-granularity-tabs] [data-granularity]').forEach(function(tab) {
        var key = tab.getAttribute('data-granularity');
        var selected = key === activeGranularity;
        tab.classList.toggle('active', selected);
        if (selected) {
          tab.setAttribute('aria-current', 'page');
        } else {
          tab.removeAttribute('aria-current');
        }
        var url = new URL(monitorPageURL, window.location.origin);
        url.searchParams.set('granularity', key);
        tab.setAttribute('href', url.pathname + url.search);
      });
    }

    function setLatestValues(latest) {
      if (!latest) {
        return;
      }
      var pid = latest.pid || {};
      var os = latest.os || {};

      var latestMap = {
        pid_cpu: formatPercent(pid.cpu),
        pid_ram: formatBytes(pid.ram),
        pid_conns: formatNumber(pid.conns),
        os_cpu: formatPercent(os.cpu),
        os_ram: formatBytes(os.ram) + ' / ' + formatBytes(os.total_ram),
        os_conns: formatNumber(os.conns),
      };

      Object.keys(latestMap).forEach(function(key) {
        var el = document.querySelector('[data-latest="' + key + '"]');
        if (el) {
          el.textContent = latestMap[key];
        }
      });

      if (latestTsEl) {
        latestTsEl.textContent = formatDateTime(latest.ts);
      }
    }

    function renderCharts(charts) {
      if (!chartsEl) {
        return;
      }

      if (!Array.isArray(charts) || charts.length === 0) {
        chartsEl.innerHTML = '<div class="monitor-card">暂无图表数据</div>';
        return;
      }

      chartsEl.innerHTML = charts.map(function(chart) {
        var metric = chart && chart.metric ? chart.metric : {};
        var title = metric.label || metric.Label || chart.title || '指标';
        var unit = metric.unit || metric.Unit || chart.unit || '';
        var svg = chart && chart.svg ? chart.svg : '<div class="monitor-card-sub">暂无图表</div>';
        return '<div class="monitor-chart-card">' +
          '<div class="monitor-chart-head">' +
            '<div class="monitor-chart-title">' + title + '</div>' +
            '<div class="monitor-chart-unit">' + unit + '</div>' +
          '</div>' +
          '<div class="monitor-chart-body">' + svg + '</div>' +
        '</div>';
      }).join('');

      bindChartTooltips();
    }

    function bindChartTooltips() {
      if (!chartsEl) {
        return;
      }
      chartsEl.querySelectorAll('.monitor-chart-body').forEach(function(chartBody) {
        attachChartTooltip(chartBody);
      });
    }

    function attachChartTooltip(chartBody) {
      var svg = chartBody.querySelector('svg');
      if (!svg || typeof bindSVGHitboxTooltip !== 'function') {
        return;
      }

      var points = svg.querySelectorAll('circle');
      var tooltip = chartBody.querySelector('[data-monitor-tooltip]');
      if (!tooltip) {
        tooltip = document.createElement('div');
        tooltip.className = 'monitor-chart-tooltip';
        tooltip.setAttribute('data-monitor-tooltip', '1');
        tooltip.hidden = true;
        chartBody.appendChild(tooltip);
      }

      bindSVGHitboxTooltip({
        container: chartBody,
        svg: svg,
        tooltip: tooltip,
        boundFlag: 'monitorTooltipBound',
        hitboxSelector: 'rect[data-uv]',
        pointSelector: 'circle',
        getText: function(hitbox) {
          var titleEl = hitbox.querySelector('title');
          if (titleEl && titleEl.textContent) {
            var text = titleEl.textContent.trim();
            if (text) {
              return text;
            }
          }
          var label = (hitbox.getAttribute('data-label') || '').trim();
          var uv = hitbox.getAttribute('data-uv') || '0';
          return label ? (label + ' - ' + uv) : uv;
        },
        getIndex: function(hitbox) {
          var index = parseInt(hitbox.getAttribute('data-index') || '', 10);
          if (isNaN(index) || index < 0 || index >= points.length) {
            return -1;
          }
          return index;
        },
      });
    }

    function showError(message) {
      if (!errorEl) {
        return;
      }
      errorEl.hidden = false;
      errorEl.textContent = message || '加载监控数据失败';
    }

    function hideError() {
      if (!errorEl) {
        return;
      }
      errorEl.hidden = true;
      errorEl.textContent = '';
    }

	    function load() {
	      hideError();

	      var url = new URL(monitorAPIURL, window.location.origin);
	      url.searchParams.set('granularity', activeGranularity);

	      window.sfetchJSON(url.toString(), {
	        method: 'GET'
	      })
	        .then(function(resp) {
	          var data = resp && resp.body;
	          if (!resp || !resp.ok || !data || data.ok !== true) {
	            var msg = data && data.error ? data.error : ('请求失败，状态码 ' + (resp ? resp.status : 0));
	            throw new Error(msg);
	          }
	          return data;
	        })
	        .then(function(data) {
	          setLatestValues(data.latest || {});
          renderCharts(data.charts || []);
        })
        .catch(function(err) {
          showError(err && err.message ? err.message : '加载监控数据失败');
        });
    }

    updateActiveTab();
    load();

    window.setInterval(load, 5000);
  })();
