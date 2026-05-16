(function() {
  var currentScript = document.currentScript;
  var systemUpdateRefreshDelay = Number(currentScript ? currentScript.getAttribute("data-refresh-delay") : "0") || 0;
  if (systemUpdateRefreshDelay > 0) {
    var refreshURL = window.location.pathname;
    window.setTimeout(function() {
      goTo(refreshURL, { replace: true });
    }, systemUpdateRefreshDelay * 1000);
  }

  var triggers = document.querySelectorAll('[data-role="system-update-manual-trigger"]');
  var input = document.getElementById("system-update-archive");
  var form = document.getElementById("system-update-manual-form");
  var fileName = document.getElementById("system-update-file-name");
  if (!triggers || triggers.length === 0 || !input || !form || !fileName) {
    return;
  }

  triggers.forEach(function(trigger) {
    trigger.addEventListener("click", function() {
      if (trigger.disabled || input.disabled) {
        return;
      }
      input.click();
    });
  });

  input.addEventListener("change", function() {
    if (!input.files || input.files.length === 0) {
      fileName.textContent = "未选择安装包";
      return;
    }
    fileName.textContent = input.files[0].name || "已选择安装包";
    form.requestSubmit();
  });
})();
