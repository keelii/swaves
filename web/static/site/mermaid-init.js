  document.addEventListener("DOMContentLoaded", function() {
    var activeMermaidViewer = null;

    function buildMermaidFullscreenButton() {
      var button = document.createElement("button");
      button.type = "button";
      button.className = "mermaid-fullscreen-button";
      button.title = "Fullscreen";
      button.setAttribute("aria-label", "Fullscreen diagram");
      button.innerHTML = '<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16"><path d="M2 2h4v1.5H3.5V6H2V2Zm8 0h4v4h-1.5V3.5H10V2ZM3.5 10v2.5H6V14H2v-4h1.5Zm9 2.5V10H14v4h-4v-1.5h2.5Z"></path></svg>';
      return button;
    }

    function resetMermaidPanZoom(diagram) {
      if (!diagram || !diagram.mermaidPanZoom) {
        return;
      }
      diagram.mermaidPanZoom.resize();
      diagram.mermaidPanZoom.resetZoom();
      diagram.mermaidPanZoom.resetPan();
      diagram.mermaidPanZoom.fit();
      diagram.mermaidPanZoom.center();
    }

    function resizeMermaidPanZoom(diagram) {
      if (!diagram || !diagram.mermaidPanZoom) {
        return;
      }
      diagram.mermaidPanZoom.resize();
      diagram.mermaidPanZoom.fit();
      diagram.mermaidPanZoom.center();
    }

    function exitMermaidPageFullscreen(viewer) {
      if (!viewer || !viewer.classList.contains("mermaid-page-fullscreen")) {
        return;
      }
      viewer.classList.remove("mermaid-page-fullscreen");
      document.documentElement.classList.remove("mermaid-page-fullscreen-active");
      activeMermaidViewer = null;
      var diagram = viewer.querySelector(".mermaid");
      resetMermaidPanZoom(diagram);
    }

    function enterMermaidPageFullscreen(viewer) {
      if (!viewer || viewer.classList.contains("mermaid-page-fullscreen")) {
        return;
      }
      if (activeMermaidViewer) {
        exitMermaidPageFullscreen(activeMermaidViewer);
      }
      viewer.classList.add("mermaid-page-fullscreen");
      document.documentElement.classList.add("mermaid-page-fullscreen-active");
      activeMermaidViewer = viewer;
      resizeMermaidPanZoom(viewer.querySelector(".mermaid"));
    }

    function enhanceMermaidContainer(diagram) {
      if (!diagram || diagram.dataset.mermaidFullscreenReady === "true" || !diagram.parentNode) {
        return;
      }
      diagram.dataset.mermaidFullscreenReady = "true";

      var viewer = document.createElement("div");
      viewer.className = "mermaid-viewer";
      diagram.parentNode.insertBefore(viewer, diagram);
      viewer.appendChild(diagram);

      var button = buildMermaidFullscreenButton();
      viewer.appendChild(button);
      button.addEventListener("click", function() {
        if (viewer.classList.contains("mermaid-page-fullscreen")) {
          exitMermaidPageFullscreen(viewer);
          return;
        }
        enterMermaidPageFullscreen(viewer);
      });
    }

    function enableMermaidPanZoom(diagram) {
      if (!diagram || diagram.dataset.mermaidPanZoomReady === "true") {
        return;
      }
      var svg = diagram.querySelector("svg");
      if (!svg || typeof window.svgPanZoom !== "function") {
        return;
      }

      enhanceMermaidContainer(diagram);
      diagram.dataset.mermaidPanZoomReady = "true";
      diagram.classList.add("mermaid-pan-zoom");
      svg.style.maxWidth = "none";
      svg.style.width = "100%";
      svg.style.height = "100%";
      diagram.mermaidPanZoom = window.svgPanZoom(svg, {
        controlIconsEnabled: false,
        fit: true,
        center: true,
        minZoom: 0.25,
        maxZoom: 8,
        zoomScaleSensitivity: 0.3
      });
    }

    function enableMermaidPanZoomAll() {
      if (typeof window.svgPanZoom !== "function") {
        console.warn("svg-pan-zoom runtime is unavailable");
        return;
      }
      document.querySelectorAll(".mermaid").forEach(enableMermaidPanZoom);
    }

    document.addEventListener("keydown", function(event) {
      if (event.key === "Escape" && activeMermaidViewer) {
        exitMermaidPageFullscreen(activeMermaidViewer);
      }
    });

    window.addEventListener("resize", function() {
      document.querySelectorAll(".mermaid").forEach(resizeMermaidPanZoom);
    });

    if (!window.mermaid || typeof window.mermaid.initialize !== "function") {
      console.warn("mermaid runtime is unavailable");
      return;
    }
    window.mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict"
    });
    if (typeof window.mermaid.run === "function") {
      window.mermaid.run({
        querySelector: ".mermaid",
        suppressErrors: true
      }).then(function() {
        enableMermaidPanZoomAll();
      }).catch(function(error) {
        console.warn("mermaid render failed", error);
        enableMermaidPanZoomAll();
      });
    }
  });
