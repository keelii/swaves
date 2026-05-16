  (function () {
    function askRestoreConfirm(message, title, opener) {
      var confirmAPI = window.DashAppUI.confirm;
      return confirmAPI.ask({
        dialogId: "backup-restore-confirm-dialog",
        title: title || "确认操作",
        message: message || "",
        messageSelector: "#backup-restore-confirm-message",
        okSelector: "#backup-restore-confirm-ok",
        opener: opener || null
      });
    }

    var uploadForm = document.querySelector("[data-restore-upload-form]");
    if (uploadForm) {
      var fileInput = uploadForm.querySelector('input[type="file"][name="file"]');
      if (fileInput && !fileInput.disabled) {
        fileInput.addEventListener("change", function () {
          if (!fileInput.files || fileInput.files.length === 0) {
            return;
          }
          askRestoreConfirm("确定使用这个 SQLite 文件恢复数据库吗？当前数据库会被替换，注意管理后密码也会恢复。", "确认恢复", fileInput).then(function (confirmed) {
            if (!confirmed) {
              fileInput.value = "";
              return;
            }
            uploadForm.submit();
          });
        });
      }
    }

    document.addEventListener("submit", function (event) {
      var form = event.target.closest("form[data-confirm-message]");
      if (!form || form.dataset.confirmed === "1") {
        return;
      }
      event.preventDefault();
      askRestoreConfirm(form.getAttribute("data-confirm-message"), form.getAttribute("data-confirm-title"), event.submitter || null).then(function (confirmed) {
        if (!confirmed) {
          return;
        }
        form.dataset.confirmed = "1";
        form.submit();
      });
    });

    var panel = document.querySelector("[data-restore-status-panel]");
    if (!panel) {
      return;
    }
    var refreshDelay = parseInt(panel.getAttribute("data-refresh-delay") || "0", 10);
    if (!refreshDelay || refreshDelay <= 0) {
      return;
    }

    var statusURL = panel.getAttribute("data-status-url");
    var labelEl = panel.querySelector("[data-restore-status-label]");
    var messageEl = panel.querySelector("[data-restore-status-message]");
    var updatedEl = panel.querySelector("[data-restore-status-updated]");

    function pollStatus() {
      fetch(statusURL, { headers: { "X-Requested-With": "XMLHttpRequest" } })
        .then(function (response) { return response.json(); })
        .then(function (result) {
          if (!result || !result.ok) {
            return;
          }
          if (labelEl) {
            labelEl.textContent = result.label || result.status || "";
          }
          if (messageEl) {
            messageEl.textContent = result.message || "恢复任务正在执行。";
          }
          if (updatedEl) {
            if (result.updated_at) {
              var dt = new Date(result.updated_at * 1000);
              updatedEl.textContent = " · 最后更新 " + dt.toLocaleString();
            } else {
              updatedEl.textContent = "";
            }
          }
          if (!result.active) {
            goTo('', { reload: true });
            return;
          }
          window.setTimeout(pollStatus, 1500);
        })
        .catch(function () {
          window.setTimeout(pollStatus, 1500);
        });
    }

    window.setTimeout(pollStatus, refreshDelay * 1000);
  })();
