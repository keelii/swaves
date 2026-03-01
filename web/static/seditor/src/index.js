import { EditorState } from "prosemirror-state";
import { EditorView } from "prosemirror-view";
import { keymap } from "prosemirror-keymap";
import { baseKeymap, lift, toggleMark, wrapIn } from "prosemirror-commands";
import { history, redo, undo } from "prosemirror-history";
import { InputRule, inputRules, wrappingInputRule } from "prosemirror-inputrules";
import { liftListItem, sinkListItem, splitListItem, wrapInList } from "prosemirror-schema-list";
import {
  MarkdownSerializer,
  defaultMarkdownParser,
  defaultMarkdownSerializer,
  schema as baseMarkdownSchema
} from "prosemirror-markdown";
import { Schema } from "prosemirror-model";

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

function ensureSeditorStyles() {
  if (typeof document === "undefined") {
    return;
  }

  var id = "seditor-styles";
  if (document.getElementById(id)) {
    return;
  }

  var el = document.createElement("style");
  el.id = id;
  el.textContent = `
.seditor-root .ProseMirror {
  outline: none;
  white-space: pre-wrap;
  word-break: break-word;
}

.seditor-root .ProseMirror p {
  margin: 0;
}

.seditor-root .ProseMirror ul,
.seditor-root .ProseMirror ol {
  padding-left: 1.5em;
}

.seditor-raw-block {
  white-space: pre;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  background: rgba(107, 114, 128, 0.08);
  border: 1px solid rgba(107, 114, 128, 0.2);
  border-radius: 8px;
  padding: 10px 12px;
  margin: 10px 0;
}

.seditor-raw-inline {
  white-space: pre;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  background: rgba(107, 114, 128, 0.10);
  border: 1px solid rgba(107, 114, 128, 0.22);
  border-radius: 6px;
  padding: 0 4px;
}
`;
  document.head.appendChild(el);
}

function buildSchema() {
  var rawInlineSpec = {
    inline: true,
    group: "inline",
    content: "text*",
    marks: "",
    parseDOM: [{ tag: 'span[data-seditor-raw-inline="true"]' }],
    toDOM: function() {
      return ["span", { "data-seditor-raw-inline": "true", class: "seditor-raw-inline" }, 0];
    }
  };

  var rawBlockSpec = {
    group: "block",
    content: "text*",
    marks: "",
    code: true,
    defining: true,
    parseDOM: [{ tag: 'pre[data-seditor-raw-block="true"]' }],
    toDOM: function() {
      return ["pre", { "data-seditor-raw-block": "true", class: "seditor-raw-block" }, 0];
    }
  };

  var nodes = baseMarkdownSchema.spec.nodes;
  if (!nodes.get("raw_inline")) {
    nodes = nodes.addToEnd("raw_inline", rawInlineSpec);
  }
  if (!nodes.get("raw_block")) {
    nodes = nodes.addToEnd("raw_block", rawBlockSpec);
  }

  return new Schema({
    nodes: nodes,
    marks: baseMarkdownSchema.spec.marks
  });
}

function buildMarkdownSerializer(schema) {
  var nodes = Object.assign({}, defaultMarkdownSerializer.nodes);
  nodes.raw_inline = function(state, node) {
    // Important: raw content must not be escaped.
    state.text(node.textContent, false);
  };
  nodes.raw_block = function(state, node) {
    // Important: raw content must not be escaped or normalized.
    state.write(node.textContent);
    state.closeBlock(node);
  };

  return new MarkdownSerializer(nodes, defaultMarkdownSerializer.marks);
}

function normalizeNewlines(input) {
  return String(input == null ? "" : input).replace(/\r\n?/g, "\n");
}

function splitIntoRawBlockSegments(markdown) {
  var lines = normalizeNewlines(markdown).split("\n");
  var segments = [];
  var mdBuffer = [];

  function flushMarkdown() {
    if (mdBuffer.length === 0) {
      return;
    }
    segments.push({ kind: "markdown", text: mdBuffer.join("\n") });
    mdBuffer = [];
  }

  for (var i = 0; i < lines.length; ) {
    var line = lines[i];
    var trimmed = line.trim();

    if (trimmed === "$$") {
      flushMarkdown();
      var blockLines = [line];
      i += 1;
      for (; i < lines.length; i += 1) {
        blockLines.push(lines[i]);
        if (lines[i].trim() === "$$") {
          i += 1;
          break;
        }
      }
      segments.push({ kind: "raw_block", text: blockLines.join("\n") });
      continue;
    }

    if (/^\[\^[^\]]+\]:/.test(line)) {
      flushMarkdown();
      var footnoteLines = [line];
      i += 1;
      for (; i < lines.length; i += 1) {
        var next = lines[i];
        if (next.trim() === "") {
          break;
        }
        if (/^(?: {4}|\t)/.test(next)) {
          footnoteLines.push(next);
          continue;
        }
        break;
      }
      segments.push({ kind: "raw_block", text: footnoteLines.join("\n") });
      continue;
    }

    mdBuffer.push(line);
    i += 1;
  }

  flushMarkdown();
  return segments;
}

function splitRawInlineText(text) {
  var input = String(text == null ? "" : text);
  if (!input) {
    return [{ kind: "text", text: "" }];
  }

  var pattern = /\[\^[^\]]+\]|\$(?!\$)[^\n]+?\$(?!\$)/g;
  var out = [];
  var last = 0;
  var match;
  while ((match = pattern.exec(input))) {
    if (match.index > last) {
      out.push({ kind: "text", text: input.slice(last, match.index) });
    }
    out.push({ kind: "raw_inline", text: match[0] });
    last = match.index + match[0].length;
  }
  if (last < input.length) {
    out.push({ kind: "text", text: input.slice(last) });
  }
  return out.length ? out : [{ kind: "text", text: input }];
}

function replaceRawInlineInDoc(schema, doc) {
  // Keep the transformation simple and explicit: rebuild via JSON.
  var json = doc.toJSON();
  function walk(nodeJSON) {
    if (!nodeJSON || typeof nodeJSON !== "object") {
      return nodeJSON;
    }
    if (nodeJSON.type === "raw_block" || nodeJSON.type === "raw_inline" || nodeJSON.type === "code_block") {
      return nodeJSON;
    }
    if (nodeJSON.content && Array.isArray(nodeJSON.content)) {
      nodeJSON.content = nodeJSON.content.map(walk);
    }
    if (nodeJSON.type === "text" && typeof nodeJSON.text === "string") {
      if (nodeJSON.marks && Array.isArray(nodeJSON.marks)) {
        for (var mi = 0; mi < nodeJSON.marks.length; mi += 1) {
          if (nodeJSON.marks[mi] && nodeJSON.marks[mi].type === "code") {
            return nodeJSON;
          }
        }
      }
      var parts = splitRawInlineText(nodeJSON.text);
      if (parts.length === 1 && parts[0].kind === "text") {
        return nodeJSON;
      }
      var marks = nodeJSON.marks;
      var out = [];
      for (var i = 0; i < parts.length; i += 1) {
        var p = parts[i];
        if (!p.text) {
          continue;
        }
        if (p.kind === "raw_inline") {
          out.push({ type: "raw_inline", content: [{ type: "text", text: p.text }] });
          continue;
        }
        var textNode = { type: "text", text: p.text };
        if (marks) {
          textNode.marks = marks;
        }
        out.push(textNode);
      }
      return out;
    }
    return nodeJSON;
  }

  function flatten(nodeJSON) {
    if (Array.isArray(nodeJSON)) {
      var out = [];
      for (var i = 0; i < nodeJSON.length; i += 1) {
        var flat = flatten(nodeJSON[i]);
        for (var j = 0; j < flat.length; j += 1) {
          out.push(flat[j]);
        }
      }
      return out;
    }
    if (nodeJSON && nodeJSON.content && Array.isArray(nodeJSON.content)) {
      var nextContent = [];
      for (var k = 0; k < nodeJSON.content.length; k += 1) {
        var next = flatten(nodeJSON.content[k]);
        for (var m = 0; m < next.length; m += 1) {
          nextContent.push(next[m]);
        }
      }
      nodeJSON.content = nextContent;
    }
    return [nodeJSON];
  }

  var walked = walk(json);
  var flattened = flatten(walked)[0];
  return schema.nodeFromJSON(flattened);
}

function parseMarkdown(schema, markdown) {
  var segments = splitIntoRawBlockSegments(markdown);
  var blocks = [];

  for (var idx = 0; idx < segments.length; idx += 1) {
    var seg = segments[idx];
    if (seg.kind === "raw_block") {
      var rawText = seg.text;
      var rawContent = rawText ? schema.text(rawText) : null;
      blocks.push(schema.nodes.raw_block.create(null, rawContent));
      continue;
    }

    var parsed = defaultMarkdownParser.parse(seg.text || "");
    var converted = schema.nodeFromJSON(parsed.toJSON());
    converted.content.forEach(function(child) {
      blocks.push(child);
    });
  }

  if (blocks.length === 0) {
    blocks.push(schema.nodes.paragraph.create());
  }

  var doc = schema.nodes.doc.create(null, blocks);
  return replaceRawInlineInDoc(schema, doc);
}

function buildPlugins(schema) {
  var listItem = schema.nodes.list_item;

  var headingRule = null;
  if (schema.nodes.heading && schema.nodes.paragraph) {
    headingRule = new InputRule(/^(#{1,6})\s$/, function(state, match, start, end) {
      var $start = state.doc.resolve(start);
      if ($start.parent.type !== schema.nodes.paragraph) {
        return null;
      }

      var level = match[1].length;
      if (level < 1 || level > 6) {
        return null;
      }

      var headingType = schema.nodes.heading;
      var depth = $start.depth;
      var parent = depth > 0 ? $start.node(depth - 1) : null;
      var index = $start.index(depth);
      if (!parent || !parent.canReplaceWith(index, index + 1, headingType)) {
        return null;
      }

      return state.tr.delete(start, end).setBlockType(start, start, headingType, { level: level });
    });
  }

  var inputRuleList = [];
  if (headingRule) {
    inputRuleList.push(headingRule);
  }
  if (schema.nodes.ordered_list) {
    inputRuleList.push(
      wrappingInputRule(
        /^(\d+)\.\s$/,
        schema.nodes.ordered_list,
        function(match) {
          return { order: Number(match[1] || 1) || 1 };
        },
        function(match, node) {
          return node.childCount + node.attrs.order === (Number(match[1] || 1) || 1);
        }
      )
    );
  }
  if (schema.nodes.bullet_list) {
    inputRuleList.push(wrappingInputRule(/^\*\s$/, schema.nodes.bullet_list));
  }
  if (schema.nodes.blockquote) {
    inputRuleList.push(wrappingInputRule(/^>\s$/, schema.nodes.blockquote));
  }

  return [
    inputRuleList.length ? inputRules({ rules: inputRuleList }) : null,
    history(),
    keymap({
      "Mod-z": undo,
      "Shift-Mod-z": redo,
      "Mod-y": redo,
      "Mod-b": toggleMark(schema.marks.strong),
      "Mod-i": toggleMark(schema.marks.em),
      Enter: listItem ? splitListItem(listItem) : undefined,
      Tab: listItem ? sinkListItem(listItem) : undefined,
      "Shift-Tab": listItem ? liftListItem(listItem) : undefined
    }),
    keymap(baseKeymap)
  ].filter(Boolean);
}

function promptForHref(currentHref) {
  if (typeof window === "undefined" || typeof window.prompt !== "function") {
    return "";
  }
  var preset = currentHref ? String(currentHref) : "https://";
  var value = window.prompt("输入链接地址", preset);
  if (value == null) {
    return "";
  }
  return String(value).trim();
}

function readFileAsDataURL(file) {
  return new Promise(function(resolve, reject) {
    var reader = new FileReader();
    reader.onload = function() {
      resolve(typeof reader.result === "string" ? reader.result : "");
    };
    reader.onerror = function() {
      reject(reader.error || new Error("read file failed"));
    };
    reader.readAsDataURL(file);
  });
}

function getImageUploadInput() {
  if (typeof document === "undefined") {
    return null;
  }
  var id = "seditor-image-upload-input";
  var input = document.getElementById(id);
  if (input) {
    return input;
  }

  input = document.createElement("input");
  input.id = id;
  input.type = "file";
  input.accept = "image/*";
  input.multiple = true;
  input.style.display = "none";
  document.body.appendChild(input);
  return input;
}

function resolveImageAttrs(file, opts) {
  if (opts && typeof opts.onImageUpload === "function") {
    return Promise.resolve(opts.onImageUpload(file)).then(function(result) {
      if (typeof result === "string") {
        return { src: result, alt: file.name || "", title: null };
      }
      if (result && typeof result === "object") {
        var src = result.src || result.url || "";
        var alt = result.alt == null ? (file.name || "") : String(result.alt);
        var title = result.title == null ? null : String(result.title);
        return { src: String(src), alt: alt, title: title };
      }
      return null;
    });
  }

  return readFileAsDataURL(file).then(function(dataURL) {
    return { src: dataURL, alt: file.name || "", title: null };
  });
}

function insertImageNode(view, schema, attrs) {
  var image = schema.nodes.image;
  if (!image || !attrs || !attrs.src) {
    return false;
  }
  var node = image.create({
    src: attrs.src,
    alt: attrs.alt || "",
    title: attrs.title == null ? null : attrs.title
  });
  var tr = view.state.tr.replaceSelectionWith(node).scrollIntoView();
  view.dispatch(tr);
  return true;
}

function isMarkActive(state, markType) {
  if (!markType || !state || !state.selection) {
    return false;
  }
  var selection = state.selection;
  if (selection.empty) {
    var stored = state.storedMarks;
    if (stored && markType.isInSet(stored)) {
      return true;
    }
    return markType.isInSet(selection.$from.marks()) != null;
  }
  return state.doc.rangeHasMark(selection.from, selection.to, markType);
}

function hasAncestorNodeType($pos, nodeType) {
  if (!$pos || !nodeType) {
    return false;
  }
  for (var depth = $pos.depth; depth > 0; depth -= 1) {
    if ($pos.node(depth).type === nodeType) {
      return true;
    }
  }
  return false;
}

function isNodeTypeActive(state, nodeType) {
  if (!state || !state.selection || !nodeType) {
    return false;
  }
  var selection = state.selection;
  return hasAncestorNodeType(selection.$from, nodeType) || hasAncestorNodeType(selection.$to, nodeType);
}

function getActiveLinkMark(state, linkMarkType) {
  if (!state || !state.selection || !linkMarkType) {
    return null;
  }
  var selection = state.selection;
  if (!selection.empty) {
    var found = null;
    state.doc.nodesBetween(selection.from, selection.to, function(node) {
      if (found || !node || !node.marks) {
        return;
      }
      found = linkMarkType.isInSet(node.marks);
    });
    return found;
  }
  return linkMarkType.isInSet(selection.$from.marks()) || null;
}

function getLinkMarkRangeAtCursor(state, linkMarkType) {
  if (!state || !state.selection || !state.selection.empty || !linkMarkType) {
    return null;
  }
  var $from = state.selection.$from;
  var parent = $from.parent;
  var offset = $from.parentOffset;
  var start = offset;
  var end = offset;

  var index = $from.index();
  while (index > 0) {
    var prev = parent.child(index - 1);
    if (!linkMarkType.isInSet(prev.marks)) {
      break;
    }
    start -= prev.nodeSize;
    index -= 1;
  }

  index = $from.indexAfter();
  while (index < parent.childCount) {
    var next = parent.child(index);
    if (!linkMarkType.isInSet(next.marks)) {
      break;
    }
    end += next.nodeSize;
    index += 1;
  }

  if (start === end) {
    return null;
  }
  var base = $from.start();
  return { from: base + start, to: base + end };
}

function isCommandActive(schema, name, state) {
  var cmd = String(name || "").trim();
  switch (cmd) {
    case "bold":
      return isMarkActive(state, schema.marks.strong);
    case "italic":
      return isMarkActive(state, schema.marks.em);
    case "link":
      return isMarkActive(state, schema.marks.link);
    case "blockquote":
      return isNodeTypeActive(state, schema.nodes.blockquote);
    case "bullet_list":
      return isNodeTypeActive(state, schema.nodes.bullet_list);
    case "ordered_list":
      return isNodeTypeActive(state, schema.nodes.ordered_list);
    default:
      return false;
  }
}

function commandByName(schema, name, opts) {
  var strong = schema.marks.strong;
  var em = schema.marks.em;
  var link = schema.marks.link;
  var blockquote = schema.nodes.blockquote;

  switch (String(name || "").trim()) {
    case "bold":
      return strong ? toggleMark(strong) : null;
    case "italic":
      return em ? toggleMark(em) : null;
    case "blockquote":
      if (!blockquote) {
        return null;
      }
      return function(state, dispatch, view) {
        var sel = state.selection;
        if (!sel || !sel.$from || !sel.$to) {
          return false;
        }
        var fromHasBlockquote = false;
        var toHasBlockquote = false;
        for (var d = sel.$from.depth; d > 0; d -= 1) {
          if (sel.$from.node(d).type === blockquote) {
            fromHasBlockquote = true;
            break;
          }
        }
        for (var d2 = sel.$to.depth; d2 > 0; d2 -= 1) {
          if (sel.$to.node(d2).type === blockquote) {
            toHasBlockquote = true;
            break;
          }
        }
        if (fromHasBlockquote && toHasBlockquote) {
          return lift(state, dispatch, view);
        }
        return wrapIn(blockquote)(state, dispatch, view);
      };
    case "bullet_list":
      return schema.nodes.bullet_list ? wrapInList(schema.nodes.bullet_list) : null;
    case "ordered_list":
      return schema.nodes.ordered_list ? wrapInList(schema.nodes.ordered_list) : null;
    case "link":
      if (!link) {
        return null;
      }
      return function(state, dispatch, view) {
        if (!view) {
          return false;
        }
        var sel = state.selection;
        var activeLink = getActiveLinkMark(state, link);
        if (activeLink) {
          if (!sel.empty) {
            dispatch(state.tr.removeMark(sel.from, sel.to, link).scrollIntoView());
            return true;
          }
          var range = getLinkMarkRangeAtCursor(state, link);
          if (!range) {
            return false;
          }
          dispatch(state.tr.removeMark(range.from, range.to, link).scrollIntoView());
          return true;
        }

        var href = promptForHref(activeLink && activeLink.attrs ? activeLink.attrs.href : "");
        if (!href) {
          return false;
        }
        var mark = link.create({ href: href, title: null });
        if (!sel.empty) {
          dispatch(state.tr.addMark(sel.from, sel.to, mark).scrollIntoView());
          return true;
        }
        var text = href;
        var tr = state.tr.insertText(text, sel.from, sel.to);
        tr.addMark(sel.from, sel.from + text.length, mark);
        dispatch(tr.scrollIntoView());
        return true;
      };
    case "image_upload":
      return function(state, dispatch, view) {
        if (!view) {
          return false;
        }
        var input = getImageUploadInput();
        if (!input) {
          return false;
        }

        input.value = "";
        input.onchange = function() {
          var files = input.files ? Array.prototype.slice.call(input.files) : [];
          if (files.length === 0) {
            return;
          }

          var sequence = Promise.resolve();
          files.forEach(function(file) {
            sequence = sequence.then(function() {
              return resolveImageAttrs(file, opts);
            }).then(function(attrs) {
              if (!attrs || !attrs.src) {
                return;
              }
              insertImageNode(view, schema, attrs);
            }).catch(function() {
              // Keep image insertion best-effort and continue with remaining files.
            });
          });

          sequence.then(function() {
            view.focus();
          });
        };

        input.click();
        return true;
      };
    case "undo":
      return undo;
    case "redo":
      return redo;
    default:
      return null;
  }
}

function bindCommandButtons(view, schema, root, opts) {
  if (!root || !root.querySelectorAll) {
    return { refresh: function() {} };
  }

  var toggleCommands = {
    bold: true,
    italic: true,
    link: true,
    blockquote: true,
    bullet_list: true,
    ordered_list: true
  };
  var bindings = [];
  var els = root.querySelectorAll("[data-seditor-command]");
  for (var i = 0; i < els.length; i += 1) {
    (function() {
      var el = els[i];
      var name = el.getAttribute("data-seditor-command");
      var command = commandByName(schema, name, opts);
      if (!command) {
        return;
      }
      bindings.push({ el: el, name: name });
      if (toggleCommands[String(name || "").trim()]) {
        el.setAttribute("aria-pressed", "false");
      }
      el.addEventListener("click", function(e) {
        e.preventDefault();
        view.focus();
        command(view.state, view.dispatch, view);
      });
    })();
  }

  function refresh() {
    for (var bi = 0; bi < bindings.length; bi += 1) {
      var binding = bindings[bi];
      var commandName = String(binding.name || "").trim();
      if (!toggleCommands[commandName]) {
        continue;
      }
      var active = isCommandActive(schema, commandName, view.state);
      binding.el.classList.toggle("selected", active);
      binding.el.setAttribute("aria-pressed", active ? "true" : "false");
    }
  }

  refresh();
  return { refresh: refresh };
}

export function init(options) {
  var opts = options || {};
  var mount = resolveElement(opts.mount || opts.el || opts.element);
  if (!mount) {
    throw new Error("SEditor.init: mount element is required");
  }

  // ensureSeditorStyles();

  mount.classList.add("seditor-root");

  var schema = buildSchema();
  var serializer = buildMarkdownSerializer(schema);
  var plugins = buildPlugins(schema);

  var textarea = resolveElement(opts.textarea);
  var initialMarkdown = "";
  if (typeof opts.initialMarkdown === "string") {
    initialMarkdown = opts.initialMarkdown;
  } else if (textarea && typeof textarea.value === "string") {
    initialMarkdown = textarea.value;
  }

  var doc = parseMarkdown(schema, initialMarkdown);
  var state = EditorState.create({ schema: schema, doc: doc, plugins: plugins });

  var view = null;
  function sync(nextState) {
    var markdown = serializer.serialize(nextState.doc);
    if (textarea && typeof textarea.value === "string") {
      textarea.value = markdown;
    }
    if (typeof opts.onChange === "function") {
      opts.onChange(markdown);
    }
    return markdown;
  }

  var commandControls = null;

  view = new EditorView(mount, {
    state: state,
    dispatchTransaction: function(tr) {
      var nextState = view.state.apply(tr);
      view.updateState(nextState);
      sync(nextState);
      if (commandControls && typeof commandControls.refresh === "function") {
        commandControls.refresh();
      }
    }
  });

  if (opts.bindCommands !== false) {
    commandControls = bindCommandButtons(view, schema, opts.commandsRoot ? resolveElement(opts.commandsRoot) : document, opts);
  }

  sync(state);
  if (commandControls && typeof commandControls.refresh === "function") {
    commandControls.refresh();
  }

  return {
    getMarkdown: function() {
      return serializer.serialize(view.state.doc);
    },
    setMarkdown: function(markdown) {
      var nextDoc = parseMarkdown(schema, String(markdown || ""));
      var nextState = EditorState.create({ schema: schema, doc: nextDoc, plugins: plugins });
      view.updateState(nextState);
      sync(nextState);
      if (commandControls && typeof commandControls.refresh === "function") {
        commandControls.refresh();
      }
    },
    focus: function() {
      view.focus();
    },
    destroy: function() {
      view.destroy();
    }
  };
}
