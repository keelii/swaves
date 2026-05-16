  (function () {
    function contentRoot() {
      return document.querySelector("article") || document.querySelector("main") || document.body;
    }

    function isIgnoredTextNode(node) {
      var el = node.parentElement;
      while (el) {
        var name = el.tagName ? el.tagName.toLowerCase() : "";
        if (name === "code" || name === "pre" || name === "script" || name === "style" || name === "textarea") {
          return true;
        }
        el = el.parentElement;
      }
      return false;
    }

    function collectContentText(root) {
      var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
      var chunks = [];
      var node = walker.nextNode();
      while (node) {
        if (!isIgnoredTextNode(node)) {
          chunks.push(node.nodeValue || "");
        }
        node = walker.nextNode();
      }
      return chunks.join("\n");
    }

    function isEscaped(text, index) {
      var slashCount = 0;
      var cursor = index - 1;
      while (cursor >= 0 && text[cursor] === "\\") {
        slashCount += 1;
        cursor -= 1;
      }
      return slashCount % 2 === 1;
    }

    function hasUnescapedDollarPair(text, start, delimiter) {
      var cursor = text.indexOf(delimiter, start + delimiter.length);
      while (cursor !== -1) {
        if (!isEscaped(text, cursor)) {
          return true;
        }
        cursor = text.indexOf(delimiter, cursor + delimiter.length);
      }
      return false;
    }

    function hasMathContent(root) {
      var text = collectContentText(root);
      var cursor = text.indexOf("$");
      while (cursor !== -1) {
        if (!isEscaped(text, cursor)) {
          if (text[cursor + 1] === "$") {
            if (hasUnescapedDollarPair(text, cursor, "$$")) {
              return true;
            }
            cursor += 2;
            continue;
          }
          if (hasUnescapedDollarPair(text, cursor, "$")) {
            return true;
          }
        }
        cursor = text.indexOf("$", cursor + 1);
      }
      return false;
    }

    function initKatex(root) {
      if (typeof window.renderMathInElement !== "function") {
        console.warn("katex auto-render runtime is unavailable");
        return;
      }
      window.renderMathInElement(root, {
        delimiters: [
          { left: "$$", right: "$$", display: true },
          { left: "$", right: "$", display: false }
        ],
        throwOnError: false
      });
    }

    async function loadKatex(root) {
      try {
        await window.loadResources([
          "/static/katex/katex.min.css",
          "/static/katex/katex.min.js",
          "/static/katex/contrib/auto-render.min.js"
        ]);
        initKatex(root);
      } catch (error) {
        console.warn("katex render failed", error);
      }
    }

    async function loadMermaid() {
      try {
        await window.loadResources([
          "/static/mermaid/mermaid.min.js",
          "/static/svg-pan-zoom/svg-pan-zoom.min.js",
          "/static/site/mermaid-init.js"
        ]);
        window.initMermaid();
      } catch (error) {
        console.warn("mermaid asset load failed", error);
      }
    }

    window.onReady(function () {
      var root = contentRoot();
      if (hasMathContent(root)) {
        loadKatex(root);
      }
      if (document.querySelector(".mermaid")) {
        loadMermaid();
      }
    });
  })();
