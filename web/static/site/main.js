function resolveElement(target) {
  if (!target) {
    return null;
  }
  if (target.nodeType === 1) {
    return target;
  }
  if (Array.isArray(target) && target.length > 0 && target[0] && target[0].nodeType === 1) {
    return target[0];
  }
  if (typeof target === "string") {
    return document.querySelector(target);
  }
  return null;
}

function setText(target, value) {
  var text = String(value == null ? "" : value);
  var el = resolveElement(target);
  if (el) {
    el.textContent = text;
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

function onReady(callback) {
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", callback, { once: true });
    return;
  }
  callback();
}

function assetLoadPromises() {
  if (!window.__swavesAssetLoadPromises) {
    window.__swavesAssetLoadPromises = {};
  }
  return window.__swavesAssetLoadPromises;
}

function loadStyle(href) {
  if (!href) {
    return Promise.resolve();
  }
  var key = "style:" + href;
  var promises = assetLoadPromises();
  if (promises[key]) {
    return promises[key];
  }
  var existingLinks = document.querySelectorAll('link[rel="stylesheet"]');
  for (var linkIndex = 0; linkIndex < existingLinks.length; linkIndex += 1) {
    if (existingLinks[linkIndex].getAttribute("href") === href) {
      promises[key] = Promise.resolve();
      return promises[key];
    }
  }
  promises[key] = new Promise(function(resolve, reject) {
    var link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    link.onload = resolve;
    link.onerror = function() {
      reject(new Error("failed to load stylesheet: " + href));
    };
    document.head.appendChild(link);
  });
  return promises[key];
}

function loadScript(src) {
  if (!src) {
    return Promise.resolve();
  }
  var key = "script:" + src;
  var promises = assetLoadPromises();
  if (promises[key]) {
    return promises[key];
  }
  var existingScripts = document.querySelectorAll("script[src]");
  for (var scriptIndex = 0; scriptIndex < existingScripts.length; scriptIndex += 1) {
    if (existingScripts[scriptIndex].getAttribute("src") === src) {
      promises[key] = Promise.resolve();
      return promises[key];
    }
  }
  promises[key] = new Promise(function(resolve, reject) {
    var script = document.createElement("script");
    script.async = true;
    script.src = src;
    script.onload = resolve;
    script.onerror = function() {
      reject(new Error("failed to load script: " + src));
    };
    document.head.appendChild(script);
  });
  return promises[key];
}

function loadResource(resource) {
  if (!resource) {
    return Promise.resolve();
  }
  if (/\.css(?:[?#]|$)/i.test(resource)) {
    return loadStyle(resource);
  }
  return loadScript(resource);
}

function loadResources(resources) {
  if (!Array.isArray(resources) || resources.length === 0) {
    return Promise.resolve();
  }
  return resources.reduce(function(chain, resource) {
    return chain.then(function() {
      return loadResource(resource);
    });
  }, Promise.resolve());
}

window.loadStyle = loadStyle;
window.loadScript = loadScript;
window.loadResources = loadResources;
window.onReady = onReady;

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

  if (heading) {
    console.warn(heading + ": " + msg);
  } else {
    console.warn(msg);
  }
  return false;
}

function getCSRFToken() {
  if (window._csrf_token_value != null) {
    var raw = String(window._csrf_token_value).trim();
    if (raw) {
      return raw;
    }
  }
  var input = document.querySelector('input[name="_csrf_token"]');
  if (!input) {
    return "";
  }
  return input.value.trim();
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
  if (input instanceof URL) {
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
    return false;
  }
}

function installSFetch() {
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
        var text = raw.trim();
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

installSFetch();
