document.addEventListener("DOMContentLoaded", function() {
  var titleInput = document.getElementById("post-title");
  var textarea = document.getElementById("post-content");
  var wordCount = document.getElementById("post-editor-word-count");
  var slugInput = document.getElementById("post-slug");
  var editorRoot = document.querySelector(".content-editor");
  var slugAPIURL = editorRoot ? (editorRoot.getAttribute("data-slug-api-url") || "") : "";
  var slugSyncTimer = null;
  var slugSyncSeq = 0;

  function updateWordCount(nextMarkdown) {
    if (!wordCount) {
      return;
    }
    var text = "";
    if (titleInput) {
      text += (titleInput.value || "") + "\n";
    }
    if (typeof nextMarkdown === "string") {
      text += nextMarkdown;
    } else if (textarea) {
      text += textarea.value || "";
    }
    text = text.replace(/\s+/g, "").trim();
    wordCount.textContent = "字数 " + text.length;
  }

  function syncSlugFromTitle() {
    if (!titleInput || !slugInput || !slugAPIURL) {
      return;
    }
    var name = (titleInput.value || "").trim();
    if (!name) {
      slugInput.value = "";
      return;
    }

    var seq = ++slugSyncSeq;
    var requestURL = new URL(slugAPIURL, window.location.origin);
    requestURL.searchParams.set("name", name);
    window.sfetchJSON(requestURL.toString(), {
      method: "GET",
    }).then(function(response) {
      if (!response || !response.ok) {
        return null;
      }
      return response.body;
    }).then(function(json) {
      if (seq !== slugSyncSeq) {
        return;
      }
      if (json && json.data) {
        slugInput.value = json.data;
      }
    }).catch(function(err) {
      console.warn("slug sync failed", err);
    });
  }

  function scheduleSlugSync() {
    if (slugSyncTimer) {
      window.clearTimeout(slugSyncTimer);
    }
    slugSyncTimer = window.setTimeout(syncSlugFromTitle, 160);
  }

  if (!window.SEditor || typeof window.SEditor.init !== "function") {
    updateWordCount();
    return;
  }

  window.__seditor = window.SEditor.init({
    mount: ".content",
    textarea: "#post-content",
    placeholder: "输入正文内容（支持 Markdown）",
    onChange: function(markdown) {
      updateWordCount(markdown);
    }
  });

  if (titleInput) {
    titleInput.addEventListener("input", updateWordCount);
    titleInput.addEventListener("input", scheduleSlugSync);
  }
  if (textarea) {
    textarea.addEventListener("input", updateWordCount);
  }
  updateWordCount();
  syncSlugFromTitle();
});
