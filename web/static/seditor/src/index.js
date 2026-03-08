import { EditorState, Plugin, Selection } from "prosemirror-state";
import { Decoration, DecorationSet, EditorView } from "prosemirror-view";
import { keymap } from "prosemirror-keymap";
import { baseKeymap, lift, toggleMark, wrapIn } from "prosemirror-commands";
import { history, redo, undo } from "prosemirror-history";
import { InputRule, inputRules, wrappingInputRule } from "prosemirror-inputrules";
import { liftListItem, sinkListItem, splitListItem, wrapInList } from "prosemirror-schema-list";
import {
  MarkdownParser,
  MarkdownSerializer,
  MarkdownSerializerState,
  defaultMarkdownParser,
  defaultMarkdownSerializer,
  schema as baseMarkdownSchema
} from "prosemirror-markdown";
import { Schema } from "prosemirror-model";
import MarkdownIt from "markdown-it";

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

function ensurePlaceholderStyles() {
  if (typeof document === "undefined") {
    return;
  }
  var id = "seditor-placeholder-styles";
  if (document.getElementById(id)) {
    return;
  }

  var el = document.createElement("style");
  el.id = id;
  el.textContent = `
.seditor-root .ProseMirror .seditor-placeholder-block::before {
  content: attr(data-placeholder);
  color: var(--app-text-soft, #9ca3af);
  pointer-events: none;
  float: left;
  height: 0;
}
`;
  document.head.appendChild(el);
}

function buildSchema() {
  var tableSpec = {
    group: "block",
    content: "(table_head | table_body)+",
    isolating: true,
    parseDOM: [{ tag: "table" }],
    toDOM: function() {
      return ["table", 0];
    }
  };

  var tableHeadSpec = {
    content: "table_row+",
    parseDOM: [{ tag: "thead" }],
    toDOM: function() {
      return ["thead", 0];
    }
  };

  var tableBodySpec = {
    content: "table_row+",
    parseDOM: [{ tag: "tbody" }],
    toDOM: function() {
      return ["tbody", 0];
    }
  };

  var tableRowSpec = {
    content: "(table_header | table_cell)+",
    parseDOM: [{ tag: "tr" }],
    toDOM: function() {
      return ["tr", 0];
    }
  };

  var tableHeaderSpec = {
    content: "inline*",
    parseDOM: [{ tag: "th" }],
    toDOM: function() {
      return ["th", 0];
    }
  };

  var tableCellSpec = {
    content: "inline*",
    parseDOM: [{ tag: "td" }],
    toDOM: function() {
      return ["td", 0];
    }
  };

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
  if (!nodes.get("table")) {
    nodes = nodes.addToEnd("table", tableSpec);
  }
  if (!nodes.get("table_head")) {
    nodes = nodes.addToEnd("table_head", tableHeadSpec);
  }
  if (!nodes.get("table_body")) {
    nodes = nodes.addToEnd("table_body", tableBodySpec);
  }
  if (!nodes.get("table_row")) {
    nodes = nodes.addToEnd("table_row", tableRowSpec);
  }
  if (!nodes.get("table_header")) {
    nodes = nodes.addToEnd("table_header", tableHeaderSpec);
  }
  if (!nodes.get("table_cell")) {
    nodes = nodes.addToEnd("table_cell", tableCellSpec);
  }
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

function buildMarkdownParser(schema) {
  var tableTokens = {
    table: { block: "table" },
    thead: { block: "table_head" },
    tbody: { block: "table_body" },
    tfoot: { block: "table_body" },
    tr: { block: "table_row" },
    th: { block: "table_header" },
    td: { block: "table_cell" }
  };
  var tokens = Object.assign({}, defaultMarkdownParser.tokens, tableTokens);
  var tokenizer = new MarkdownIt("default", {
    html: false,
    linkify: false,
    typographer: false
  });
  var parser = new MarkdownParser(schema, tokenizer, tokens);
  parser.tokenHandlers.softbreak = function(state) {
    if (schema.nodes.hard_break) {
      state.addNode(schema.nodes.hard_break);
      return;
    }
    state.addText("\n");
  };
  return parser;
}

function escapeTableCellContent(raw) {
  return String(raw == null ? "" : raw)
    .replace(/\r\n?/g, "\n")
    .replace(/\n+/g, "<br>")
    .replace(/\|/g, "\\|")
    .trim();
}

function escapeMarkdownLabelText(raw) {
  return String(raw == null ? "" : raw)
    .replace(/\r\n?/g, " ")
    .replace(/\\/g, "\\\\")
    .replace(/\[/g, "\\[")
    .replace(/\]/g, "\\]");
}

function serializeTableCell(state, cell) {
  var tempState = new MarkdownSerializerState(state.nodes, state.marks, state.options);
  tempState.renderInline(cell, true);
  return escapeTableCellContent(tempState.out);
}

function buildMarkdownSerializer(schema) {
  var nodes = Object.assign({}, defaultMarkdownSerializer.nodes);
  nodes.table = function(state, node) {
    state.ensureNewLine();

    var rows = [];
    node.forEach(function(child) {
      var childName = child && child.type ? child.type.name : "";
      if (childName === "table_head" || childName === "table_body") {
        child.forEach(function(row) {
          rows.push(row);
        });
        return;
      }
      if (childName === "table_row") {
        rows.push(child);
      }
    });
    if (rows.length === 0) {
      state.write("|  |");
      state.write("\n| --- |");
      state.closeBlock(node);
      return;
    }

    var rowValues = rows.map(function(row) {
      var values = [];
      row.forEach(function(cell) {
        values.push(serializeTableCell(state, cell));
      });
      return values;
    });

    var colCount = 0;
    for (var i = 0; i < rowValues.length; i += 1) {
      if (rowValues[i].length > colCount) {
        colCount = rowValues[i].length;
      }
    }
    if (colCount < 1) {
      colCount = 1;
    }

    for (var rowIndex = 0; rowIndex < rowValues.length; rowIndex += 1) {
      while (rowValues[rowIndex].length < colCount) {
        rowValues[rowIndex].push("");
      }
    }

    var headerRow = rowValues[0];
    var delimiterRow = [];
    for (var colIndex = 0; colIndex < colCount; colIndex += 1) {
      delimiterRow.push("---");
    }

    state.write("| " + headerRow.join(" | ") + " |");
    state.write("\n| " + delimiterRow.join(" | ") + " |");

    for (var bodyIndex = 1; bodyIndex < rowValues.length; bodyIndex += 1) {
      state.write("\n| " + rowValues[bodyIndex].join(" | ") + " |");
    }
    state.closeBlock(node);
  };
  nodes.image = function(state, node) {
    var alt = escapeMarkdownLabelText(node.attrs && node.attrs.alt ? node.attrs.alt : "");
    var src = String(node.attrs && node.attrs.src ? node.attrs.src : "").replace(/[\(\)]/g, "\\$&");
    var title = node.attrs && node.attrs.title ? ' "' + String(node.attrs.title).replace(/"/g, '\\"') + '"' : "";
    state.write("![" + alt + "](" + src + title + ")");
  };
  nodes.hard_break = function(state) {
    // Keep single-line markdown breaks as plain "\n" to avoid adding trailing backslashes.
    state.write("\n");
  };
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

  var pattern = /\[\^[^\]]+\]|\$\$[^\n]+?\$\$|\$(?!\$)[^\n]+?\$(?!\$)/g;
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

function protectRawInlinePlaceholders(text) {
  var input = String(text == null ? "" : text);
  if (!input) {
    return { text: "", placeholders: [] };
  }

  var pattern = /\[\^[^\]]+\]|\$\$[^\n]+?\$\$|\$(?!\$)[^\n]+?\$(?!\$)/g;
  var placeholders = [];
  var out = "";
  var last = 0;
  var match;

  while ((match = pattern.exec(input))) {
    out += input.slice(last, match.index);
    var token = "\uE000SEDITOR_RAW_INLINE_" + placeholders.length + "\uE001";
    placeholders.push({ token: token, raw: match[0] });
    out += token;
    last = match.index + match[0].length;
  }

  out += input.slice(last);
  return { text: out, placeholders: placeholders };
}

function escapeRegExp(text) {
  return String(text == null ? "" : text).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function replaceRawInlinePlaceholdersInDoc(schema, doc, placeholders) {
  if (!placeholders || placeholders.length < 1) {
    return doc;
  }

  var byToken = {};
  var parts = [];
  for (var i = 0; i < placeholders.length; i += 1) {
    var item = placeholders[i];
    if (!item || !item.token) {
      continue;
    }
    byToken[item.token] = item.raw || "";
    parts.push(escapeRegExp(item.token));
  }
  if (parts.length < 1) {
    return doc;
  }

  var tokenPattern = new RegExp(parts.join("|"), "g");

  function splitTextByToken(text, marks) {
    tokenPattern.lastIndex = 0;
    var out = [];
    var last = 0;
    var match;

    while ((match = tokenPattern.exec(text))) {
      if (match.index > last) {
        var beforeText = { type: "text", text: text.slice(last, match.index) };
        if (marks) {
          beforeText.marks = marks;
        }
        out.push(beforeText);
      }

      var raw = byToken[match[0]];
      if (raw) {
        out.push({ type: "raw_inline", content: [{ type: "text", text: raw }] });
      } else {
        var tokenText = { type: "text", text: match[0] };
        if (marks) {
          tokenText.marks = marks;
        }
        out.push(tokenText);
      }
      last = match.index + match[0].length;
    }

    if (last < text.length) {
      var tailText = { type: "text", text: text.slice(last) };
      if (marks) {
        tailText.marks = marks;
      }
      out.push(tailText);
    }

    if (out.length < 1) {
      var original = { type: "text", text: text };
      if (marks) {
        original.marks = marks;
      }
      out.push(original);
    }
    return out;
  }

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
      return splitTextByToken(nodeJSON.text, nodeJSON.marks);
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

  var walked = walk(doc.toJSON());
  var flattened = flatten(walked)[0];
  return schema.nodeFromJSON(flattened);
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

function parseMarkdown(schema, parser, markdown) {
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

    var protectedMarkdown = protectRawInlinePlaceholders(seg.text || "");
    var parsed = parser.parse(protectedMarkdown.text || "");
    var converted = schema.nodeFromJSON(parsed.toJSON());
    converted = replaceRawInlinePlaceholdersInDoc(schema, converted, protectedMarkdown.placeholders);
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

function createPlaceholderPlugin(schema, placeholder) {
  var text = String(placeholder == null ? "" : placeholder).trim();
  if (!text) {
    return null;
  }

  return new Plugin({
    props: {
      decorations: function(state) {
        var doc = state.doc;
        if (doc.childCount !== 1) {
          return null;
        }
        var first = doc.firstChild;
        if (!first || !first.isTextblock || first.content.size !== 0) {
          return null;
        }
        var deco = Decoration.node(0, first.nodeSize, {
          "data-placeholder": text,
          class: "seditor-placeholder-block"
        });
        return DecorationSet.create(doc, [deco]);
      }
    }
  });
}

function buildHeadingIDRunePattern() {
  try {
    return new RegExp("^[\\p{L}\\p{N}]$", "u");
  } catch (error) {
    return null;
  }
}

function isHeadingIDRune(char, headingIDRunePattern) {
  if (!char) {
    return false;
  }
  if (char === "-") {
    return true;
  }
  if ((char >= "0" && char <= "9") || (char >= "A" && char <= "Z") || (char >= "a" && char <= "z")) {
    return true;
  }
  return !!(headingIDRunePattern && headingIDRunePattern.test(char));
}

function buildHeadingAnchorID(text, headingIDRunePattern) {
  var source = String(text == null ? "" : text).trim().replace(/ /g, "-");
  var out = "";
  for (var i = 0; i < source.length; i += 1) {
    var char = source.charAt(i);
    if (isHeadingIDRune(char, headingIDRunePattern)) {
      out += char;
    }
  }
  return out || "heading";
}

function createHeadingIDSyncPlugin(schema) {
  if (!schema.nodes.heading) {
    return null;
  }

  var headingIDRunePattern = buildHeadingIDRunePattern();
  return new Plugin({
    props: {
      decorations: function(state) {
        var headingType = schema.nodes.heading;
        var decorations = [];
        state.doc.descendants(function(node, pos) {
          if (node.type === headingType) {
            decorations.push(
              Decoration.node(pos, pos + node.nodeSize, {
                id: buildHeadingAnchorID(node.textContent || "", headingIDRunePattern)
              })
            );
          }
          return true;
        });
        if (!decorations.length) {
          return null;
        }
        return DecorationSet.create(state.doc, decorations);
      }
    }
  });
}

function buildPlugins(schema, options) {
  var opts = options || {};
  var listItem = schema.nodes.list_item;
  var placeholderPlugin = createPlaceholderPlugin(schema, opts.placeholder);
  var headingIDSyncPlugin = createHeadingIDSyncPlugin(schema);

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
    placeholderPlugin,
    headingIDSyncPlugin,
    inputRuleList.length ? inputRules({ rules: inputRuleList }) : null,
    history(),
    keymap({
      "Mod-z": undo,
      "Shift-Mod-z": redo,
      "Mod-y": redo,
      "Mod-b": toggleMark(schema.marks.strong),
      "Mod-i": toggleMark(schema.marks.em),
      Backspace: deleteSingleCellTableOnBackspace,
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

function createEmptyTableCellNode(cellType) {
  if (!cellType) {
    return null;
  }
  if (typeof cellType.createAndFill === "function") {
    var filled = cellType.createAndFill();
    if (filled) {
      return filled;
    }
  }
  return cellType.create();
}

function createTableRowNode(schema, colCount, isHeader) {
  var rowType = schema.nodes.table_row;
  var headerCellType = schema.nodes.table_header;
  var bodyCellType = schema.nodes.table_cell;
  if (!rowType || !headerCellType || !bodyCellType) {
    return null;
  }

  var cells = [];
  var cellType = isHeader ? headerCellType : bodyCellType;
  for (var col = 0; col < colCount; col += 1) {
    var cell = createEmptyTableCellNode(cellType);
    if (!cell) {
      return null;
    }
    cells.push(cell);
  }

  return rowType.create(null, cells);
}

function insertDefaultTableNode(view, schema) {
  var tableType = schema.nodes.table;
  var tableHeadType = schema.nodes.table_head;
  var tableBodyType = schema.nodes.table_body;
  if (!tableType) {
    return false;
  }

  var headerRow = createTableRowNode(schema, 3, true);
  var bodyRow = createTableRowNode(schema, 3, false);
  if (!headerRow || !bodyRow) {
    return false;
  }

  var tableChildren = [];
  if (tableHeadType && tableBodyType) {
    tableChildren.push(tableHeadType.create(null, [headerRow]));
    tableChildren.push(tableBodyType.create(null, [bodyRow]));
  } else {
    tableChildren.push(headerRow);
    tableChildren.push(bodyRow);
  }

  var tableNode;
  try {
    tableNode = tableType.create(null, tableChildren);
  } catch (error) {
    return false;
  }

  var insertPos = view.state.selection.from;
  var tr = view.state.tr.replaceSelectionWith(tableNode);
  var mappedInsertPos = tr.mapping.map(insertPos, -1);
  var target = normalizeTargetCell(tableNode, 0, 0);
  var cursorPos = mappedInsertPos + getTableCellContentOffset(tableNode, target.row, target.col);
  tr = tr.setSelection(Selection.near(tr.doc.resolve(cursorPos))).scrollIntoView();
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

function getTableRowsFromNode(tableNode) {
  var rows = [];
  if (!tableNode || typeof tableNode.childCount !== "number") {
    return rows;
  }

  var offset = 1;
  var headCount = 0;
  var bodyCount = 0;

  for (var childIndex = 0; childIndex < tableNode.childCount; childIndex += 1) {
    var child = tableNode.child(childIndex);
    if (!child || !child.type) {
      continue;
    }

    var childName = child.type.name;
    if (childName === "table_head" || childName === "table_body") {
      var sectionType = childName;
      var sectionOffset = offset + 1;
      for (var sectionRowIndex = 0; sectionRowIndex < child.childCount; sectionRowIndex += 1) {
        var rowNode = child.child(sectionRowIndex);
        rows.push({
          rowNode: rowNode,
          sectionType: sectionType,
          rowIndexInSection: sectionRowIndex,
          startOffset: sectionOffset
        });
        sectionOffset += rowNode.nodeSize;
      }
      if (sectionType === "table_head") {
        headCount += child.childCount;
      } else {
        bodyCount += child.childCount;
      }
      offset += child.nodeSize;
      continue;
    }

    if (childName === "table_row") {
      var fallbackSectionType = headCount === 0 ? "table_head" : "table_body";
      var fallbackRowIndex = fallbackSectionType === "table_head" ? headCount : bodyCount;
      rows.push({
        rowNode: child,
        sectionType: fallbackSectionType,
        rowIndexInSection: fallbackRowIndex,
        startOffset: offset
      });
      if (fallbackSectionType === "table_head") {
        headCount += 1;
      } else {
        bodyCount += 1;
      }
      offset += child.nodeSize;
      continue;
    }

    offset += child.nodeSize;
  }

  return rows;
}

function getTableRowsFromJSON(tableJSON) {
  var rows = [];
  var content = tableJSON && Array.isArray(tableJSON.content) ? tableJSON.content : [];
  var headCount = 0;
  var bodyCount = 0;

  for (var index = 0; index < content.length; index += 1) {
    var child = content[index];
    if (!child || typeof child !== "object") {
      continue;
    }

    if (child.type === "table_head" || child.type === "table_body") {
      var sectionType = child.type;
      var sectionRows = Array.isArray(child.content) ? child.content : [];
      for (var rowIndex = 0; rowIndex < sectionRows.length; rowIndex += 1) {
        var row = sectionRows[rowIndex];
        if (!row || typeof row !== "object" || row.type !== "table_row") {
          continue;
        }
        rows.push({
          row: row,
          sectionType: sectionType,
          rowIndexInSection: rowIndex
        });
      }
      if (sectionType === "table_head") {
        headCount += sectionRows.length;
      } else {
        bodyCount += sectionRows.length;
      }
      continue;
    }

    if (child.type === "table_row") {
      var fallbackSectionType = headCount === 0 ? "table_head" : "table_body";
      var fallbackRowIndex = fallbackSectionType === "table_head" ? headCount : bodyCount;
      rows.push({
        row: child,
        sectionType: fallbackSectionType,
        rowIndexInSection: fallbackRowIndex
      });
      if (fallbackSectionType === "table_head") {
        headCount += 1;
      } else {
        bodyCount += 1;
      }
    }
  }

  return rows;
}

function getTableContext(state) {
  if (!state || !state.selection || !state.selection.$from) {
    return null;
  }

  var $from = state.selection.$from;
  var tableDepth = -1;
  var rowDepth = -1;
  var cellDepth = -1;

  for (var depth = $from.depth; depth >= 0; depth -= 1) {
    var node = $from.node(depth);
    var nodeName = node && node.type ? node.type.name : "";
    if (cellDepth < 0 && (nodeName === "table_cell" || nodeName === "table_header")) {
      cellDepth = depth;
    }
    if (rowDepth < 0 && nodeName === "table_row") {
      rowDepth = depth;
    }
    if (nodeName === "table") {
      tableDepth = depth;
      break;
    }
  }

  if (tableDepth < 0 || rowDepth < 0 || cellDepth < 0) {
    return null;
  }

  var tableNode = $from.node(tableDepth);
  var tablePos = $from.before(tableDepth);
  var rowStartOffset = $from.before(rowDepth) - tablePos;
  var rows = getTableRowsFromNode(tableNode);
  if (rows.length < 1) {
    return null;
  }

  var rowIndex = -1;
  for (var i = 0; i < rows.length; i += 1) {
    if (rows[i].startOffset === rowStartOffset) {
      rowIndex = i;
      break;
    }
  }
  if (rowIndex < 0) {
    var relativePos = $from.pos - tablePos;
    for (var j = 0; j < rows.length; j += 1) {
      var start = rows[j].startOffset;
      var end = start + rows[j].rowNode.nodeSize;
      if (relativePos >= start && relativePos < end) {
        rowIndex = j;
        break;
      }
    }
  }
  if (rowIndex < 0) {
    return null;
  }

  var rowMeta = rows[rowIndex];
  var colIndex = $from.index(rowDepth);
  if (!rowMeta.rowNode || rowMeta.rowNode.childCount < 1) {
    colIndex = 0;
  } else if (colIndex < 0) {
    colIndex = 0;
  } else if (colIndex >= rowMeta.rowNode.childCount) {
    colIndex = rowMeta.rowNode.childCount - 1;
  }

  return {
    tableNode: tableNode,
    tablePos: tablePos,
    cellDepth: cellDepth,
    rowIndex: rowIndex,
    rowIndexInSection: rowMeta.rowIndexInSection,
    sectionType: rowMeta.sectionType,
    colIndex: colIndex
  };
}

function deleteSingleCellTableOnBackspace(state, dispatch) {
  if (!state || !state.selection || !state.selection.empty) {
    return false;
  }

  var context = getTableContext(state);
  if (!context || !context.tableNode || !Number.isInteger(context.cellDepth)) {
    return false;
  }

  var $from = state.selection.$from;
  var cellNode = $from.node(context.cellDepth);
  if (!cellNode || cellNode.content.size !== 0 || $from.parentOffset !== 0) {
    return false;
  }

  var rows = getTableRowsFromNode(context.tableNode);
  if (rows.length !== 1) {
    return false;
  }
  var onlyRow = rows[0].rowNode;
  if (!onlyRow || onlyRow.childCount !== 1) {
    return false;
  }

  if (!dispatch) {
    return true;
  }

  var tr = state.tr.delete(context.tablePos, context.tablePos + context.tableNode.nodeSize);
  var selectionPos = Math.min(context.tablePos, tr.doc.content.size);
  tr = tr.setSelection(Selection.near(tr.doc.resolve(selectionPos), -1)).scrollIntoView();
  dispatch(tr);
  return true;
}

function cloneNodeJSON(node) {
  return JSON.parse(JSON.stringify(node.toJSON()));
}

function getNormalizedTableRows(sections) {
  var rows = [];
  var headRows = sections && Array.isArray(sections.headRows) ? sections.headRows : [];
  var bodyRows = sections && Array.isArray(sections.bodyRows) ? sections.bodyRows : [];

  for (var i = 0; i < headRows.length; i += 1) {
    rows.push({
      row: headRows[i],
      sectionType: "table_head",
      rowIndexInSection: i
    });
  }
  for (var j = 0; j < bodyRows.length; j += 1) {
    rows.push({
      row: bodyRows[j],
      sectionType: "table_body",
      rowIndexInSection: j
    });
  }
  return rows;
}

function getMaxTableColumns(tableJSON) {
  var rows = getTableRowsFromJSON(tableJSON);
  var maxCount = 0;
  for (var i = 0; i < rows.length; i += 1) {
    var row = rows[i].row;
    var cells = row && Array.isArray(row.content) ? row.content : [];
    if (cells.length > maxCount) {
      maxCount = cells.length;
    }
  }
  return maxCount;
}

function createEmptyCellJSON(typeName) {
  return {
    type: typeName || "table_cell",
    content: []
  };
}

function ensureRowCells(rowJSON, cellType) {
  if (!rowJSON || typeof rowJSON !== "object") {
    return;
  }
  if (!Array.isArray(rowJSON.content)) {
    rowJSON.content = [];
  }
  if (rowJSON.content.length < 1) {
    rowJSON.content.push(createEmptyCellJSON(cellType));
  }

  for (var cellIndex = 0; cellIndex < rowJSON.content.length; cellIndex += 1) {
    var cell = rowJSON.content[cellIndex];
    if (!cell || typeof cell !== "object") {
      rowJSON.content[cellIndex] = createEmptyCellJSON(cellType);
      continue;
    }
    cell.type = cellType;
    if (!Array.isArray(cell.content)) {
      cell.content = [];
    }
  }
}

function normalizeTableJSONSections(tableJSON) {
  var rows = getTableRowsFromJSON(tableJSON);
  var headRows = [];
  var bodyRows = [];

  for (var i = 0; i < rows.length; i += 1) {
    var rowMeta = rows[i];
    if (!rowMeta.row) {
      continue;
    }
    if (rowMeta.sectionType === "table_head") {
      ensureRowCells(rowMeta.row, "table_header");
      headRows.push(rowMeta.row);
    } else {
      ensureRowCells(rowMeta.row, "table_cell");
      bodyRows.push(rowMeta.row);
    }
  }

  if (headRows.length < 1) {
    if (bodyRows.length > 0) {
      var promoted = bodyRows.shift();
      ensureRowCells(promoted, "table_header");
      headRows.push(promoted);
    } else {
      headRows.push({
        type: "table_row",
        content: [createEmptyCellJSON("table_header")]
      });
    }
  }

  return {
    headRows: headRows,
    bodyRows: bodyRows
  };
}

function setTableJSONSections(tableJSON, sections) {
  var nextContent = [{
    type: "table_head",
    content: sections.headRows
  }];
  if (sections.bodyRows.length > 0) {
    nextContent.push({
      type: "table_body",
      content: sections.bodyRows
    });
  }
  tableJSON.content = nextContent;
}

function getTableCellContentOffset(tableNode, rowIndex, colIndex) {
  var rows = getTableRowsFromNode(tableNode);
  if (rows.length < 1) {
    return 1;
  }

  var targetRow = rowIndex;
  if (targetRow < 0) {
    targetRow = 0;
  }
  if (targetRow >= rows.length) {
    targetRow = rows.length - 1;
  }

  var rowMeta = rows[targetRow];
  var rowNode = rowMeta.rowNode;
  if (!rowNode || rowNode.childCount < 1) {
    return rowMeta.startOffset + 1;
  }

  var targetCol = colIndex;
  if (targetCol < 0) {
    targetCol = 0;
  }
  if (targetCol >= rowNode.childCount) {
    targetCol = rowNode.childCount - 1;
  }

  var offset = rowMeta.startOffset + 1;
  for (var col = 0; col < targetCol; col += 1) {
    offset += rowNode.child(col).nodeSize;
  }
  offset += 1;
  return offset;
}

function normalizeTargetCell(newTableNode, rowIndex, colIndex) {
  var rows = getTableRowsFromNode(newTableNode);
  if (rows.length < 1) {
    return { row: 0, col: 0 };
  }

  var targetRow = rowIndex;
  if (targetRow < 0) {
    targetRow = 0;
  }
  if (targetRow >= rows.length) {
    targetRow = rows.length - 1;
  }

  var rowNode = rows[targetRow].rowNode;
  var targetCol = colIndex;
  if (!rowNode || rowNode.childCount < 1) {
    targetCol = 0;
  } else {
    if (targetCol < 0) {
      targetCol = 0;
    }
    if (targetCol >= rowNode.childCount) {
      targetCol = rowNode.childCount - 1;
    }
  }

  return { row: targetRow, col: targetCol };
}

function applyTableMutation(state, dispatch, mutate) {
  var context = getTableContext(state);
  if (!context || typeof mutate !== "function") {
    return false;
  }

  var tableJSON = cloneNodeJSON(context.tableNode);
  var result = mutate(tableJSON, context);
  if (!result || result.ok !== true) {
    return false;
  }

  var newTableNode;
  try {
    newTableNode = state.schema.nodeFromJSON(tableJSON);
  } catch (error) {
    return false;
  }
  if (!newTableNode || newTableNode.childCount < 1) {
    return false;
  }

  var target = normalizeTargetCell(
    newTableNode,
    Number.isInteger(result.targetRow) ? result.targetRow : context.rowIndex,
    Number.isInteger(result.targetCol) ? result.targetCol : context.colIndex
  );

  if (!dispatch) {
    return true;
  }

  var tr = state.tr.replaceWith(
    context.tablePos,
    context.tablePos + context.tableNode.nodeSize,
    newTableNode
  );
  var cursorPos = context.tablePos + getTableCellContentOffset(newTableNode, target.row, target.col);
  tr = tr.setSelection(Selection.near(tr.doc.resolve(cursorPos))).scrollIntoView();
  dispatch(tr);
  return true;
}

function mutateTableAddRow(tableJSON, context) {
  var sections = normalizeTableJSONSections(tableJSON);
  var rows = getNormalizedTableRows(sections);
  if (rows.length < 1 || context.rowIndex < 0 || context.rowIndex >= rows.length) {
    return { ok: false };
  }

  var currentRow = rows[context.rowIndex];
  var baseRow = currentRow.row || {};
  var baseCells = Array.isArray(baseRow.content) ? baseRow.content : [];
  var colCount = baseCells.length || getMaxTableColumns(tableJSON);
  if (colCount < 1) {
    colCount = 1;
  }

  var newRowCells = [];
  for (var col = 0; col < colCount; col += 1) {
    newRowCells.push(createEmptyCellJSON("table_cell"));
  }
  var newRow = {
    type: "table_row",
    content: newRowCells
  };

  var targetRow = context.rowIndex + 1;
  if (currentRow.sectionType === "table_head") {
    sections.bodyRows.splice(0, 0, newRow);
    targetRow = sections.headRows.length;
  } else {
    var bodyInsertIndex = currentRow.rowIndexInSection + 1;
    sections.bodyRows.splice(bodyInsertIndex, 0, newRow);
    targetRow = sections.headRows.length + bodyInsertIndex;
  }

  setTableJSONSections(tableJSON, sections);
  return {
    ok: true,
    targetRow: targetRow,
    targetCol: Math.min(context.colIndex, colCount - 1)
  };
}

function mutateTableAddColumn(tableJSON, context) {
  var sections = normalizeTableJSONSections(tableJSON);
  var rows = getNormalizedTableRows(sections);
  if (rows.length < 1) {
    return { ok: false };
  }

  var targetCol = context.colIndex + 1;
  for (var rowIndex = 0; rowIndex < rows.length; rowIndex += 1) {
    var rowMeta = rows[rowIndex];
    var rowJSON = rowMeta.row;
    if (!rowJSON || !Array.isArray(rowJSON.content)) {
      rowJSON.content = [];
    }
    var cells = rowJSON.content;
    var sampleCell = cells[Math.min(context.colIndex, Math.max(0, cells.length - 1))];
    var fallbackType = rowMeta.sectionType === "table_head" ? "table_header" : "table_cell";
    var cellType = sampleCell && sampleCell.type ? sampleCell.type : fallbackType;
    var insertAt = Math.min(targetCol, cells.length);
    cells.splice(insertAt, 0, createEmptyCellJSON(cellType));
  }

  setTableJSONSections(tableJSON, sections);
  return {
    ok: true,
    targetRow: context.rowIndex,
    targetCol: targetCol
  };
}

function mutateTableDeleteRow(tableJSON, context) {
  var sections = normalizeTableJSONSections(tableJSON);
  var rows = getNormalizedTableRows(sections);
  if (rows.length <= 1 || context.rowIndex < 0 || context.rowIndex >= rows.length) {
    return { ok: false };
  }

  var rowMeta = rows[context.rowIndex];
  if (!rowMeta) {
    return { ok: false };
  }

  if (rowMeta.sectionType === "table_head") {
    if (sections.headRows.length > 1) {
      sections.headRows.splice(rowMeta.rowIndexInSection, 1);
    } else if (sections.bodyRows.length > 0) {
      var promoted = sections.bodyRows.shift();
      ensureRowCells(promoted, "table_header");
      sections.headRows[0] = promoted;
    } else {
      return { ok: false };
    }
  } else {
    sections.bodyRows.splice(rowMeta.rowIndexInSection, 1);
  }

  if (sections.headRows.length < 1) {
    if (sections.bodyRows.length < 1) {
      return { ok: false };
    }
    var fallbackHead = sections.bodyRows.shift();
    ensureRowCells(fallbackHead, "table_header");
    sections.headRows.push(fallbackHead);
  }

  setTableJSONSections(tableJSON, sections);
  var nextRows = getNormalizedTableRows(sections);
  var targetRow = context.rowIndex;
  if (targetRow >= nextRows.length) {
    targetRow = nextRows.length - 1;
  }
  var targetRowNode = nextRows[targetRow] ? nextRows[targetRow].row : null;
  var targetCells = targetRowNode && Array.isArray(targetRowNode.content) ? targetRowNode.content : [];
  var targetCol = Math.min(context.colIndex, Math.max(0, targetCells.length - 1));

  return {
    ok: true,
    targetRow: targetRow,
    targetCol: targetCol
  };
}

function mutateTableDeleteColumn(tableJSON, context) {
  var sections = normalizeTableJSONSections(tableJSON);
  var rows = getNormalizedTableRows(sections);
  if (rows.length < 1) {
    return { ok: false };
  }

  var maxCols = 0;
  for (var row = 0; row < rows.length; row += 1) {
    var cells = rows[row].row && Array.isArray(rows[row].row.content) ? rows[row].row.content : [];
    if (cells.length > maxCols) {
      maxCols = cells.length;
    }
  }
  if (maxCols <= 1) {
    return { ok: false };
  }

  for (var rowIndex = 0; rowIndex < rows.length; rowIndex += 1) {
    var rowMeta = rows[rowIndex];
    var rowCells = rowMeta.row && Array.isArray(rowMeta.row.content) ? rowMeta.row.content : [];
    if (rowCells.length < 1) {
      rowMeta.row.content = [createEmptyCellJSON(rowMeta.sectionType === "table_head" ? "table_header" : "table_cell")];
      continue;
    }
    var removeAt = Math.min(context.colIndex, rowCells.length - 1);
    rowCells.splice(removeAt, 1);
    if (rowCells.length < 1) {
      rowCells.push(createEmptyCellJSON(rowMeta.sectionType === "table_head" ? "table_header" : "table_cell"));
    }
  }

  setTableJSONSections(tableJSON, sections);
  var targetCol = context.colIndex;
  if (targetCol >= maxCols - 1) {
    targetCol = maxCols - 2;
  }
  if (targetCol < 0) {
    targetCol = 0;
  }

  return {
    ok: true,
    targetRow: context.rowIndex,
    targetCol: targetCol
  };
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
    case "table_insert":
      return function(state, dispatch, view) {
        if (!schema.nodes.table || !schema.nodes.table_row || !schema.nodes.table_header || !schema.nodes.table_cell) {
          return false;
        }
        if (!dispatch) {
          return true;
        }
        if (!view) {
          return false;
        }
        return insertDefaultTableNode(view, schema);
      };
    case "table_add_row":
      return function(state, dispatch) {
        return applyTableMutation(state, dispatch, mutateTableAddRow);
      };
    case "table_add_column":
      return function(state, dispatch) {
        return applyTableMutation(state, dispatch, mutateTableAddColumn);
      };
    case "table_delete_row":
      return function(state, dispatch) {
        return applyTableMutation(state, dispatch, mutateTableDeleteRow);
      };
    case "table_delete_column":
      return function(state, dispatch) {
        return applyTableMutation(state, dispatch, mutateTableDeleteColumn);
      };
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
  var tableCommands = {
    table_add_row: true,
    table_add_column: true,
    table_delete_row: true,
    table_delete_column: true
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
      var visibleWhen = String(el.getAttribute("data-seditor-visible-when") || "").trim();
      bindings.push({ el: el, name: name, command: command, visibleWhen: visibleWhen });
      if (toggleCommands[String(name || "").trim()]) {
        el.setAttribute("aria-pressed", "false");
      }
      el.addEventListener("click", function(e) {
        e.preventDefault();
        if (el.disabled || el.hidden) {
          return;
        }
        view.focus();
        command(view.state, view.dispatch, view);
      });
    })();
  }

  function refresh() {
    var inTable = !!getTableContext(view.state);
    for (var bi = 0; bi < bindings.length; bi += 1) {
      var binding = bindings[bi];
      var commandName = String(binding.name || "").trim();
      if (binding.visibleWhen === "table") {
        binding.el.hidden = !inTable;
        if (!inTable) {
          binding.el.disabled = true;
          continue;
        }
      }
      if (!toggleCommands[commandName]) {
        if (tableCommands[commandName]) {
          var enabled = binding.command(view.state, null, view);
          binding.el.disabled = !enabled;
        }
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

  mount.classList.add("seditor-root");
  if (typeof opts.placeholder === "string" && opts.placeholder.trim()) {
    ensurePlaceholderStyles();
  }

  var schema = buildSchema();
  var markdownParser = buildMarkdownParser(schema);
  var serializer = buildMarkdownSerializer(schema);
  var plugins = buildPlugins(schema, opts);

  var textarea = resolveElement(opts.textarea);
  var initialMarkdown = "";
  if (typeof opts.initialMarkdown === "string") {
    initialMarkdown = opts.initialMarkdown;
  } else if (textarea && typeof textarea.value === "string") {
    initialMarkdown = textarea.value;
  }

  var doc = parseMarkdown(schema, markdownParser, initialMarkdown);
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
      var nextDoc = parseMarkdown(schema, markdownParser, String(markdown || ""));
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
