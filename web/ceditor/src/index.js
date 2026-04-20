import { closeBrackets, autocompletion, closeBracketsKeymap, completionKeymap } from "@codemirror/autocomplete";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { html } from "@codemirror/lang-html";
import { css } from "@codemirror/lang-css";
import { javascript } from "@codemirror/lang-javascript";
import { Compartment, EditorState } from "@codemirror/state";
import {
  bracketMatching,
  defaultHighlightStyle,
  foldGutter,
  foldKeymap,
  indentOnInput,
  syntaxHighlighting
} from "@codemirror/language";
import { lintKeymap } from "@codemirror/lint";
import { highlightSelectionMatches, searchKeymap } from "@codemirror/search";
import {
  crosshairCursor,
  Decoration,
  drawSelection,
  dropCursor,
  EditorView,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  lineNumbers,
  MatchDecorator,
  rectangularSelection,
  ViewPlugin,
  keymap
} from "@codemirror/view";

function resolveElement(target) {
  if (!target) {
    return null;
  }
  if (target.nodeType === 1) {
    return target;
  }
  if (typeof target === "string") {
    return document.querySelector(target);
  }
  return null;
}

var jinjaMatchDecorator = new MatchDecorator({
  regexp: /\{\{.*?\}\}|\{%.*?%\}|\{#.*?#\}/g,
  decoration: function(match) {
    var value = match[0] || "";
    if (value.startsWith("{#")) {
      return Decoration.mark({ class: "cm-jinja-comment" });
    }
    if (value.startsWith("{%")) {
      return Decoration.mark({ class: "cm-jinja-tag" });
    }
    return Decoration.mark({ class: "cm-jinja-expression" });
  }
});

var jinjaHighlightExtension = ViewPlugin.fromClass(class {
  constructor(view) {
    this.decorations = jinjaMatchDecorator.createDeco(view);
  }

  update(update) {
    this.decorations = jinjaMatchDecorator.updateDeco(update, this.decorations);
  }
}, {
  decorations: function(instance) {
    return instance.decorations;
  }
});

function inferModeFromFilePath(filePath) {
  var normalized = typeof filePath === "string" ? filePath.trim().toLowerCase() : "";
  if (!normalized) {
    return "jinja";
  }
  if (normalized.endsWith(".css")) {
    return "css";
  }
  if (normalized.endsWith(".js") || normalized.endsWith(".mjs") || normalized.endsWith(".cjs")) {
    return "javascript";
  }
  if (
    normalized.endsWith(".jinja") ||
    normalized.endsWith(".j2") ||
    normalized.endsWith(".jinja2") ||
    normalized.endsWith(".njk") ||
    normalized.endsWith(".tmpl") ||
    normalized.endsWith(".html")
  ) {
    return "jinja";
  }
  return "html";
}

function normalizeMode(mode, filePath) {
  var next = typeof mode === "string" ? mode.trim().toLowerCase() : "";
  if (!next) {
    return inferModeFromFilePath(filePath);
  }
  if (next === "js") {
    return "javascript";
  }
  if (next === "jinja2") {
    return "jinja";
  }
  if (next === "html" || next === "jinja" || next === "css" || next === "javascript") {
    return next;
  }
  return inferModeFromFilePath(filePath);
}

function resolveModeExtensions(mode) {
  switch (mode) {
    case "css":
      return [css()];
    case "javascript":
      return [javascript()];
    case "html":
      return [html()];
    case "jinja":
    default:
      return [html(), jinjaHighlightExtension];
  }
}

function createBasicSetupExtensions() {
  return [
    lineNumbers(),
    highlightActiveLineGutter(),
    highlightSpecialChars(),
    history(),
    foldGutter({
      markerDOM: function(open) {
        var marker = document.createElement("span");
        marker.className = "cm-foldGutterMarker";
        marker.setAttribute("aria-hidden", "true");
        marker.innerHTML = open
          ? '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" focusable="false"><path d="M3 5.25L8 11L13 5.25Z" fill="currentColor"></path></svg>'
          : '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" focusable="false"><path d="M5 3L11 8L5 13Z" fill="currentColor"></path></svg>';
        return marker;
      }
    }),
    drawSelection(),
    dropCursor(),
    EditorState.allowMultipleSelections.of(true),
    indentOnInput(),
    syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
    bracketMatching(),
    closeBrackets(),
    autocompletion(),
    rectangularSelection(),
    crosshairCursor(),
    highlightActiveLine(),
    highlightSelectionMatches(),
    keymap.of([
      ...closeBracketsKeymap,
      ...defaultKeymap,
      ...searchKeymap,
      ...historyKeymap,
      ...foldKeymap,
      ...completionKeymap,
      ...lintKeymap
    ])
  ];
}

function syncTextarea(textarea, value) {
  if (!textarea) {
    return;
  }
  textarea.value = value;
}

function replaceDoc(view, value) {
  var nextValue = typeof value === "string" ? value : "";
  var current = view.state.doc.toString();
  if (current === nextValue) {
    return;
  }
  view.dispatch({
    changes: {
      from: 0,
      to: view.state.doc.length,
      insert: nextValue
    }
  });
}

function init(options) {
  var config = options && typeof options === "object" ? options : {};
  var mount = resolveElement(config.mount);
  var textarea = resolveElement(config.textarea);
  if (!mount && textarea && textarea.parentNode) {
    mount = document.createElement("div");
    textarea.parentNode.insertBefore(mount, textarea.nextSibling);
  }
  if (!mount) {
    return null;
  }

  var filePath = typeof config.filePath === "string" ? config.filePath : "";
  var mode = normalizeMode(config.mode, filePath);
  var initialValue = typeof config.value === "string" ? config.value : (textarea ? textarea.value || "" : "");
  var onChange = typeof config.onChange === "function" ? config.onChange : null;
  var onSave = typeof config.onSave === "function" ? config.onSave : null;
  var readOnly = config.readOnly === true;
  var useLineWrapping = config.lineWrapping !== false;
  var languageCompartment = new Compartment();
  var editableCompartment = new Compartment();
  var wrappingCompartment = new Compartment();
  var form = textarea ? textarea.form : null;
  var resetTimer = null;

  mount.classList.add("ceditor-root");
  mount.hidden = false;

  function getCurrentValue() {
    return view.state.doc.toString();
  }

  function triggerSave() {
    if (readOnly || !onSave) {
      return false;
    }
    var value = getCurrentValue();
    syncTextarea(textarea, value);
    onSave(value, view);
    return true;
  }

  var keybindings = [indentWithTab];
  var saveShortcutExtension = [];
  if (onSave) {
    saveShortcutExtension = [
      EditorView.domEventHandlers({
        keydown: function(event) {
          if (readOnly) {
            return false;
          }
          if ((!event.metaKey && !event.ctrlKey) || event.altKey || event.shiftKey) {
            return false;
          }
          if (String(event.key || "").toLowerCase() !== "s") {
            return false;
          }
          event.preventDefault();
          return triggerSave();
        }
      })
    ];
  }

  var state = EditorState.create({
    doc: initialValue,
    extensions: [
      createBasicSetupExtensions(),
      keymap.of(keybindings),
      saveShortcutExtension,
      languageCompartment.of(resolveModeExtensions(mode)),
      editableCompartment.of([
        EditorState.readOnly.of(readOnly),
        EditorView.editable.of(!readOnly)
      ]),
      wrappingCompartment.of(useLineWrapping ? EditorView.lineWrapping : []),
      EditorView.theme({
        "&": {
          height: "100%"
        }
      }),
      EditorView.updateListener.of(function(update) {
        if (!update.docChanged) {
          return;
        }
        var value = update.state.doc.toString();
        syncTextarea(textarea, value);
        if (onChange) {
          onChange(value, update);
        }
      })
    ]
  });

  var view = new EditorView({
    state: state,
    parent: mount
  });

  if (textarea) {
    syncTextarea(textarea, initialValue);
    textarea.classList.add("ceditor-hidden-textarea");
    textarea.hidden = true;
  }

  function handleFormReset() {
    if (!textarea) {
      return;
    }
    if (resetTimer) {
      window.clearTimeout(resetTimer);
    }
    resetTimer = window.setTimeout(function() {
      replaceDoc(view, textarea.value || "");
    }, 0);
  }

  if (form) {
    form.addEventListener("reset", handleFormReset);
  }

  return {
    getValue: function() {
      return getCurrentValue();
    },
    setValue: function(value) {
      var nextValue = typeof value === "string" ? value : "";
      replaceDoc(view, nextValue);
      syncTextarea(textarea, nextValue);
    },
    setMode: function(nextMode) {
      mode = normalizeMode(nextMode, filePath);
      view.dispatch({
        effects: languageCompartment.reconfigure(resolveModeExtensions(mode))
      });
    },
    setReadOnly: function(nextReadOnly) {
      readOnly = nextReadOnly === true;
      view.dispatch({
        effects: editableCompartment.reconfigure([
          EditorState.readOnly.of(readOnly),
          EditorView.editable.of(!readOnly)
        ])
      });
    },
    focus: function() {
      view.focus();
    },
    save: function() {
      return triggerSave();
    },
    destroy: function() {
      if (form) {
        form.removeEventListener("reset", handleFormReset);
      }
      if (resetTimer) {
        window.clearTimeout(resetTimer);
      }
      if (textarea) {
        syncTextarea(textarea, view.state.doc.toString());
        textarea.hidden = false;
        textarea.classList.remove("ceditor-hidden-textarea");
      }
      view.destroy();
      mount.innerHTML = "";
      mount.hidden = true;
    }
  };
}

export { inferModeFromFilePath, init };
