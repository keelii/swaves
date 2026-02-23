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

function getCSRFMetaValue(name) {
  var el = document.querySelector('meta[name="' + name + '"]');
  if (!el) {
    return "";
  }
  return String(el.getAttribute("content") || "").trim();
}

function isCSRFProtectedMethod(method) {
  var verb = String(method || "GET").toUpperCase();
  return !(verb === "GET" || verb === "HEAD" || verb === "OPTIONS" || verb === "TRACE");
}

function ensureFormCSRFToken(form) {
  if (!form || !form.getAttribute) {
    return;
  }
  var method = form.getAttribute("method") || "GET";
  if (!isCSRFProtectedMethod(method)) {
    return;
  }

  var token = getCSRFMetaValue("csrf-token");
  if (!token) {
    return;
  }
  var fieldName = getCSRFMetaValue("csrf-field-name") || "_csrf_token";
  var input = form.querySelector('input[name="' + fieldName + '"]');
  if (!input) {
    input = document.createElement("input");
    input.type = "hidden";
    input.name = fieldName;
    form.appendChild(input);
  }
  input.value = token;
}

function installCSRFFormProtection() {
  var forms = document.querySelectorAll("form");
  for (var i = 0; i < forms.length; i += 1) {
    ensureFormCSRFToken(forms[i]);
  }
  document.addEventListener("submit", function(event) {
    ensureFormCSRFToken(event.target);
  });
}

function installCSRFFetchProtection() {
  if (typeof window.fetch !== "function") {
    return;
  }

  var originalFetch = window.fetch;
  window.fetch = function(input, init) {
    var requestInit = init ? Object.assign({}, init) : {};
    var token = getCSRFMetaValue("csrf-token");
    var method = requestInit.method;
    if (!method && input && typeof input === "object" && typeof input.method === "string") {
      method = input.method;
    }
    if (!isCSRFProtectedMethod(method) || !token) {
      return originalFetch(input, init);
    }

    var baseHeaders = requestInit.headers;
    if (!baseHeaders && input && typeof input === "object" && input.headers) {
      baseHeaders = input.headers;
    }
    var headers = new Headers(baseHeaders || undefined);
    if (!headers.has("X-CSRF-Token")) {
      headers.set("X-CSRF-Token", token);
    }
    requestInit.headers = headers;
    return originalFetch(input, requestInit);
  };
}

if (typeof document !== "undefined") {
  document.addEventListener("DOMContentLoaded", function() {
    installCSRFFormProtection();
    installCSRFFetchProtection();
  });
}
