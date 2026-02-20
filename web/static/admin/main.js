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
