  (function() {
    function toDatetimeLocal(unixSeconds) {
      var parsed = Number(unixSeconds || 0);
      if (!isFinite(parsed) || parsed <= 0) {
        return "";
      }
      var date = new Date(parsed * 1000);
      if (!isFinite(date.getTime())) {
        return "";
      }
      function pad(num) {
        return String(num).padStart(2, "0");
      }
      return [
        date.getFullYear(),
        "-",
        pad(date.getMonth() + 1),
        "-",
        pad(date.getDate()),
        "T",
        pad(date.getHours()),
        ":",
        pad(date.getMinutes())
      ].join("");
    }

    var optionInput = document.getElementById("encrypted-expiry-option");
    var customInput = document.getElementById("encrypted-expiry-custom");
    if (!optionInput || !customInput) {
      return;
    }

    function syncCustomVisibility() {
      if ((optionInput.value || "") === "custom") {
        if (!customInput.value) {
          var initialUnix = customInput.getAttribute("data-initial-unix");
          customInput.value = toDatetimeLocal(initialUnix);
        }
        customInput.hidden = false;
        return;
      }
      customInput.hidden = true;
      customInput.value = "";
    }

    optionInput.addEventListener("change", syncCustomVisibility);
    syncCustomVisibility();
  })();
