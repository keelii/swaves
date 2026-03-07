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

  if (window.console && typeof window.console.warn === "function") {
    if (heading) {
      window.console.warn(heading + ": " + msg);
    } else {
      window.console.warn(msg);
    }
  }
  return false;
}

function getCSRFToken() {
  if (typeof window !== "undefined" && window._csrf_token_value != null) {
    var raw = String(window._csrf_token_value || "").trim();
    if (raw) {
      return raw;
    }
  }
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
}
