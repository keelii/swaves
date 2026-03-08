function isJQueryObject(value) {
  return !!(value && typeof value === "object" && value.jquery);
}

function resolveElement(target) {
  if (!target) {
    return null;
  }
  if (target.nodeType === 1) {
    return target;
  }
  if (isJQueryObject(target)) {
    return target.length > 0 ? target.get(0) : null;
  }
  if (Array.isArray(target) && target.length > 0 && target[0] && target[0].nodeType === 1) {
    return target[0];
  }
  if (typeof target === "string") {
    return document.querySelector(target);
  }
  return null;
}

function resolveJQuery(target) {
  if (!window.jQuery) {
    return null;
  }
  if (isJQueryObject(target)) {
    return target;
  }
  if (typeof target === "string") {
    return window.jQuery(target);
  }
  if (target && target.nodeType === 1) {
    return window.jQuery(target);
  }
  return null;
}

function setText(target, value) {
  var text = String(value == null ? "" : value);
  var el = resolveElement(target);
  if (el) {
    el.textContent = text;
    return;
  }

  var $target = resolveJQuery(target);
  if ($target && $target.length) {
    $target.text(text);
  }
}

function escapeHTML(raw) {
  return String(raw == null ? "" : raw)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function openDialog(target) {
  var dialogEl = resolveElement(target);
  if (!dialogEl || typeof dialogEl.showModal !== "function") {
    return false;
  }
  if (dialogEl.open) {
    dialogEl.close();
  }
  dialogEl.showModal();
  return true;
}

function closeDialog(target) {
  var dialogEl = resolveElement(target);
  if (!dialogEl || !dialogEl.open) {
    return false;
  }
  dialogEl.close();
  return true;
}

function notify(message, title, options) {
  var opts = options || {};
  var msg = String(message == null ? "" : message);
  var heading = String(title == null ? "" : title);
  var variant = String(opts.variant || "info");

  if (opts.dialog) {
    if (opts.titleTarget) {
      setText(opts.titleTarget, heading || "提示");
    }
    if (opts.messageTarget) {
      setText(opts.messageTarget, msg);
    }
    if (openDialog(opts.dialog)) {
      return true;
    }
  }

  if (!opts.disableToast && window.ot && typeof window.ot.toast === "function") {
    window.ot.toast(msg, heading, { variant: variant });
    return true;
  }

  if (opts.alertFallback && typeof window.alert === "function") {
    if (heading) {
      window.alert(heading + "\n" + msg);
    } else {
      window.alert(msg);
    }
    return true;
  }

  if (window.console && typeof window.console.warn === "function") {
    if (heading) {
      window.console.warn(heading + ": " + msg);
    } else {
      window.console.warn(msg);
    }
  }
  return false;
}

function goTo(target, options) {
  if (typeof window === "undefined" || !window.location) {
    return false;
  }

  var opts = options || {};
  if (opts.reload === true) {
    window.location.reload();
    return true;
  }

  var href = String(target == null ? "" : target).trim();
  if (!href) {
    return false;
  }

  if (opts.replace === true && typeof window.location.replace === "function") {
    window.location.replace(href);
    return true;
  }
  if (typeof window.location.assign === "function") {
    window.location.assign(href);
    return true;
  }

  window.location.href = href;
  return true;
}

function bindPageSizeSelect(target, options) {
  var selectEl = resolveElement(target);
  if (!selectEl || typeof selectEl.addEventListener !== "function") {
    return false;
  }

  if (selectEl.getAttribute("data-page-size-bound") === "1") {
    return true;
  }
  selectEl.setAttribute("data-page-size-bound", "1");

  var opts = options || {};
  var pageSizeParam = String(opts.pageSizeParam || selectEl.getAttribute("data-page-size-param") || "pageSize").trim() || "pageSize";
  var pageParam = String(opts.pageParam || selectEl.getAttribute("data-page-param") || "page").trim();
  var resetPage = opts.resetPage;
  if (resetPage == null) {
    var resetRaw = selectEl.getAttribute("data-reset-page");
    resetPage = resetRaw == null ? "1" : resetRaw;
  }
  var baseURL = String(opts.baseURL || selectEl.getAttribute("data-base-url") || "").trim();

  selectEl.addEventListener("change", function() {
    var url = null;
    try {
      url = new URL(baseURL || window.location.href, window.location.origin);
    } catch (err) {
      url = new URL(window.location.href, window.location.origin);
    }

    var value = String(selectEl.value || "").trim();
    if (value) {
      url.searchParams.set(pageSizeParam, value);
    } else {
      url.searchParams.delete(pageSizeParam);
    }

    if (pageParam) {
      if (resetPage == null || String(resetPage) === "") {
        url.searchParams.delete(pageParam);
      } else {
        url.searchParams.set(pageParam, String(resetPage));
      }
    }

    goTo(url.toString());
  });

  return true;
}

function initPageSizeSelects() {
  if (typeof document === "undefined" || typeof document.querySelectorAll !== "function") {
    return;
  }
  var selects = document.querySelectorAll('[data-role="page-size-select"]');
  if (!selects || selects.length === 0) {
    return;
  }
  for (var i = 0; i < selects.length; i += 1) {
    bindPageSizeSelect(selects[i]);
  }
}

function bindSVGHitboxTooltip(options) {
  var opts = options || {};
  var container = resolveElement(opts.container);
  if (!container || typeof container.querySelector !== "function") {
    return false;
  }

  var svgEl = resolveElement(opts.svg);
  if (!svgEl) {
    var svgSelector = String(opts.svgSelector || "svg").trim() || "svg";
    svgEl = container.querySelector(svgSelector);
  }
  if (!svgEl || typeof svgEl.querySelectorAll !== "function") {
    return false;
  }

  var boundFlag = String(opts.boundFlag || "tooltipBound").trim();
  if (svgEl.dataset && boundFlag) {
    if (svgEl.dataset[boundFlag] === "1") {
      return false;
    }
    svgEl.dataset[boundFlag] = "1";
  }

  var hitboxSelector = String(opts.hitboxSelector || "rect[data-uv]").trim() || "rect[data-uv]";
  var hitboxes = svgEl.querySelectorAll(hitboxSelector);
  if (!hitboxes || hitboxes.length === 0) {
    return false;
  }

  var tooltipEl = resolveElement(opts.tooltip);
  if (!tooltipEl && opts.createTooltip === true) {
    tooltipEl = document.createElement("div");
    tooltipEl.className = String(opts.tooltipClassName || "").trim() || "monitor-chart-tooltip";
    if (opts.tooltipDataAttr) {
      tooltipEl.setAttribute(String(opts.tooltipDataAttr), "1");
    }
    tooltipEl.hidden = true;
    container.appendChild(tooltipEl);
  }
  if (!tooltipEl) {
    return false;
  }

  var pointSelector = String(opts.pointSelector || "").trim();
  var points = pointSelector ? svgEl.querySelectorAll(pointSelector) : [];
  var getIndex = typeof opts.getIndex === "function"
    ? opts.getIndex
    : function(hitbox) {
        var index = parseInt(hitbox && hitbox.getAttribute ? hitbox.getAttribute("data-index") || "" : "", 10);
        if (isNaN(index) || index < 0) {
          return -1;
        }
        return index;
      };
  var getText = typeof opts.getText === "function"
    ? opts.getText
    : function(hitbox) {
        return hitbox && hitbox.getAttribute ? String(hitbox.getAttribute("data-label") || hitbox.getAttribute("data-uv") || "").trim() : "";
      };
  var onActivate = typeof opts.onActivate === "function" ? opts.onActivate : null;
  var onDeactivate = typeof opts.onDeactivate === "function" ? opts.onDeactivate : null;
  var activeHitbox = null;

  function hideTooltip() {
    tooltipEl.hidden = true;
    activeHitbox = null;
    if (onDeactivate) {
      onDeactivate();
    }
  }

  function anchorPosition(hitbox) {
    var index = getIndex(hitbox);
    if (index >= 0 && points && index < points.length) {
      var pointRect = points[index].getBoundingClientRect();
      return {
        left: pointRect.left + pointRect.width / 2,
        top: pointRect.top + pointRect.height / 2,
      };
    }

    var hitboxRect = hitbox.getBoundingClientRect();
    return {
      left: hitboxRect.left + hitboxRect.width / 2,
      top: hitboxRect.top + hitboxRect.height / 2,
    };
  }

  function showTooltip(hitbox) {
    if (!hitbox) {
      return;
    }
    activeHitbox = hitbox;

    var text = String(getText(hitbox) || "").trim();
    if (!text) {
      hideTooltip();
      return;
    }

    tooltipEl.textContent = text;
    tooltipEl.hidden = false;

    var containerRect = container.getBoundingClientRect();
    var anchor = anchorPosition(hitbox);
    var tooltipHalf = tooltipEl.offsetWidth / 2;
    var left = anchor.left - containerRect.left;
    var minLeft = tooltipHalf + 4;
    var maxLeft = containerRect.width - tooltipHalf - 4;
    if (left < minLeft) {
      left = minLeft;
    }
    if (left > maxLeft) {
      left = maxLeft;
    }

    var anchorTop = anchor.top - containerRect.top;
    var top = anchorTop - tooltipEl.offsetHeight - 8;
    if (top < 0) {
      top = anchorTop + 8;
    }
    var maxTop = containerRect.height - tooltipEl.offsetHeight;
    if (top > maxTop) {
      top = maxTop;
    }

    tooltipEl.style.left = left + "px";
    tooltipEl.style.top = top + "px";

    if (onActivate) {
      onActivate(hitbox, getIndex(hitbox));
    }
  }

  function isSameHitboxTarget(nextTarget) {
    if (typeof SVGElement === "undefined" || !nextTarget || !(nextTarget instanceof SVGElement)) {
      return false;
    }
    if (nextTarget.matches && nextTarget.matches(hitboxSelector)) {
      return true;
    }
    return false;
  }

  hitboxes.forEach(function(hitbox) {
    hitbox.setAttribute("tabindex", "0");
    hitbox.addEventListener("mouseenter", function() {
      showTooltip(hitbox);
    });
    hitbox.addEventListener("mousemove", function() {
      showTooltip(hitbox);
    });
    hitbox.addEventListener("mouseleave", function(event) {
      if (isSameHitboxTarget(event.relatedTarget)) {
        return;
      }
      hideTooltip();
    });
    hitbox.addEventListener("focus", function() {
      showTooltip(hitbox);
    });
    hitbox.addEventListener("blur", function() {
      hideTooltip();
    });
  });

  svgEl.addEventListener("mouseleave", function() {
    hideTooltip();
  });

  return true;
}

function initDashMainBehaviors() {
  initPageSizeSelects();
}

function getCSRFToken() {
  var input = document.querySelector('input[name="_csrf_token"]');
  if (!input) {
    return "";
  }
  return String(input.value || "").trim();
}

function shouldCSRF(method) {
  var verb = String(method || "GET").toUpperCase();
  return verb === "POST" || verb === "PUT" || verb === "PATCH" || verb === "DELETE";
}

function resolveRequestMethod(input, init) {
  if (init && typeof init.method === "string") {
    return init.method;
  }
  if (input && typeof input === "object" && typeof input.method === "string") {
    return input.method;
  }
  return "GET";
}

function resolveRequestURL(input) {
  if (typeof input === "string") {
    return input;
  }
  if (typeof URL !== "undefined" && input instanceof URL) {
    return input.toString();
  }
  if (input && typeof input === "object" && typeof input.url === "string") {
    return input.url;
  }
  return window.location.href;
}

function appendQueryToURL(rawURL, query) {
  if (!query) {
    return rawURL;
  }

  var url = new URL(String(rawURL), window.location.origin);
  if (query instanceof URLSearchParams) {
    query.forEach(function(value, key) {
      url.searchParams.set(key, value);
    });
    return url.toString();
  }

  if (typeof query === "object") {
    Object.keys(query).forEach(function(key) {
      var value = query[key];
      if (value == null) {
        return;
      }
      if (Array.isArray(value)) {
        url.searchParams.delete(key);
        for (var idx = 0; idx < value.length; idx += 1) {
          if (value[idx] == null) {
            continue;
          }
          url.searchParams.append(key, String(value[idx]));
        }
        return;
      }
      url.searchParams.set(key, String(value));
    });
  }

  return url.toString();
}

function isSameOriginRequest(input) {
  try {
    var url = new URL(resolveRequestURL(input), window.location.origin);
    return url.origin === window.location.origin;
  } catch (err) {
    return true;
  }
}

function installSFetch() {
  if (typeof window.fetch !== "function") {
    return;
  }
  window.sfetch = function(input, init, opts) {
    var requestInit = init ? Object.assign({}, init) : {};
    var extra = opts || {};
    var disableCSRF = extra.disableCSRF === true;
    var requestInput = input;
    if (extra.query) {
      var queryURL = appendQueryToURL(resolveRequestURL(input), extra.query);
      if (typeof Request !== "undefined" && input instanceof Request) {
        requestInput = new Request(queryURL, input);
      } else {
        requestInput = queryURL;
      }
    }
    var method = resolveRequestMethod(input, requestInit);
    var sameOrigin = isSameOriginRequest(requestInput);

    var baseHeaders = requestInit.headers;
    if (!baseHeaders && input && typeof input === "object" && input.headers) {
      baseHeaders = input.headers;
    }
    var headers = null;
    var ensureHeaders = function() {
      if (!headers) {
        headers = new Headers(baseHeaders || undefined);
      }
      return headers;
    };

    if (!disableCSRF && shouldCSRF(method) && sameOrigin) {
      var token = getCSRFToken();
      if (token) {
        var csrfHeaders = ensureHeaders();
        if (!csrfHeaders.has("X-CSRF-Token")) {
          csrfHeaders.set("X-CSRF-Token", token);
        }
      }
    }

    if (extra.ajax !== false && sameOrigin) {
      var ajaxHeaders = ensureHeaders();
      if (!ajaxHeaders.has("X-Requested-With")) {
        ajaxHeaders.set("X-Requested-With", "XMLHttpRequest");
      }
    }

    if (headers) {
      requestInit.headers = headers;
    }

    return window.fetch(requestInput, requestInit);
  };

  window.sfetchJSON = function(input, init, opts) {
    var requestInit = init ? Object.assign({}, init) : {};
    var baseHeaders = requestInit.headers;
    if (!baseHeaders && input && typeof input === "object" && input.headers) {
      baseHeaders = input.headers;
    }
    var headers = new Headers(baseHeaders || undefined);
    if (!headers.has("Accept")) {
      headers.set("Accept", "application/json");
    }
    requestInit.headers = headers;

    return window.sfetch(input, requestInit, opts).then(function(response) {
      return response.text().then(function(raw) {
        var text = String(raw || "").trim();
        var body = null;
        if (text) {
          try {
            body = JSON.parse(text);
          } catch (err) {
            body = null;
          }
        }
        return {
          status: response.status,
          ok: response.ok,
          body: body,
          raw: text,
          response: response
        };
      });
    });
  };
}

if (typeof document !== "undefined") {
  installSFetch();
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initDashMainBehaviors, { once: true });
  } else {
    initDashMainBehaviors();
  }
}
