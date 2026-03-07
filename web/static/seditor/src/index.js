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

function buildPlugins(schema, options) {
  var opts = options || {};
  var listItem = schema.nodes.list_item;
  var placeholderPlugin = createPlaceholderPlugin(schema, opts.placeholder);

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
  var rowNode = $from.node(rowDepth);
  var rowIndex = $from.index(tableDepth);
  var colIndex = $from.index(rowDepth);

  if (!tableNode || rowIndex < 0 || rowIndex >= tableNode.childCount) {
    return null;
  }
  if (!rowNode || colIndex < 0 || colIndex >= rowNode.childCount) {
    return null;
  }

  return {
    tableNode: tableNode,
    tablePos: $from.before(tableDepth),
    rowIndex: rowIndex,
    colIndex: colIndex
  };
}

function cloneNodeJSON(node) {
  return JSON.parse(JSON.stringify(node.toJSON()));
}

function getMaxTableColumns(tableJSON) {
  var rows = Array.isArray(tableJSON.content) ? tableJSON.content : [];
  var maxCount = 0;
  for (var i = 0; i < rows.length; i += 1) {
    var cells = rows[i] && Array.isArray(rows[i].content) ? rows[i].content : [];
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

function getTableCellContentOffset(tableNode, rowIndex, colIndex) {
  var offset = 1;
  for (var row = 0; row < rowIndex; row += 1) {
    offset += tableNode.child(row).nodeSize;
  }

  var rowNode = tableNode.child(rowIndex);
  offset += 1;
  for (var col = 0; col < colIndex; col += 1) {
    offset += rowNode.child(col).nodeSize;
  }
  offset += 1;
  return offset;
}

function normalizeTargetCell(newTableNode, rowIndex, colIndex) {
  var targetRow = rowIndex;
  if (targetRow < 0) {
    targetRow = 0;
  }
  if (targetRow >= newTableNode.childCount) {
    targetRow = newTableNode.childCount - 1;
  }

  var targetRowNode = newTableNode.child(targetRow);
  var targetCol = colIndex;
  if (targetCol < 0) {
    targetCol = 0;
  }
  if (targetCol >= targetRowNode.childCount) {
    targetCol = targetRowNode.childCount - 1;
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
  var rows = Array.isArray(tableJSON.content) ? tableJSON.content : [];
  if (rows.length < 1 || context.rowIndex < 0 || context.rowIndex >= rows.length) {
    return { ok: false };
  }

  var baseRow = rows[context.rowIndex] || {};
  var baseCells = Array.isArray(baseRow.content) ? baseRow.content : [];
  var colCount = baseCells.length || getMaxTableColumns(tableJSON);
  if (colCount < 1) {
    colCount = 1;
  }

  var newRowCells = [];
  for (var col = 0; col < colCount; col += 1) {
    newRowCells.push(createEmptyCellJSON("table_cell"));
  }

  var insertIndex = context.rowIndex + 1;
  rows.splice(insertIndex, 0, {
    type: "table_row",
    content: newRowCells
  });

  return {
    ok: true,
    targetRow: insertIndex,
    targetCol: Math.min(context.colIndex, colCount - 1)
  };
}

function mutateTableAddColumn(tableJSON, context) {
  var rows = Array.isArray(tableJSON.content) ? tableJSON.content : [];
  if (rows.length < 1) {
    return { ok: false };
  }

  var targetCol = context.colIndex + 1;
  for (var row = 0; row < rows.length; row += 1) {
    var rowJSON = rows[row];
    if (!rowJSON || !Array.isArray(rowJSON.content)) {
      rowJSON.content = [];
    }
    var cells = rowJSON.content;
    var sampleCell = cells[Math.min(context.colIndex, Math.max(0, cells.length - 1))];
    var cellType = sampleCell && sampleCell.type ? sampleCell.type : (row === 0 ? "table_header" : "table_cell");
    var insertAt = Math.min(targetCol, cells.length);
    cells.splice(insertAt, 0, createEmptyCellJSON(cellType));
  }

  return {
    ok: true,
    targetRow: context.rowIndex,
    targetCol: targetCol
  };
}

function mutateTableDeleteRow(tableJSON, context) {
  var rows = Array.isArray(tableJSON.content) ? tableJSON.content : [];
  if (rows.length <= 1 || context.rowIndex < 0 || context.rowIndex >= rows.length) {
    return { ok: false };
  }

  rows.splice(context.rowIndex, 1);
  var targetRow = context.rowIndex;
  if (targetRow >= rows.length) {
    targetRow = rows.length - 1;
  }
  var targetCells = rows[targetRow] && Array.isArray(rows[targetRow].content) ? rows[targetRow].content : [];
  var targetCol = Math.min(context.colIndex, Math.max(0, targetCells.length - 1));

  return {
    ok: true,
    targetRow: targetRow,
    targetCol: targetCol
  };
}

function mutateTableDeleteColumn(tableJSON, context) {
  var rows = Array.isArray(tableJSON.content) ? tableJSON.content : [];
  if (rows.length < 1) {
    return { ok: false };
  }

  var minCols = Infinity;
  for (var row = 0; row < rows.length; row += 1) {
    var cells = rows[row] && Array.isArray(rows[row].content) ? rows[row].content : [];
    if (cells.length < minCols) {
      minCols = cells.length;
    }
  }
  if (!Number.isFinite(minCols) || minCols <= 1) {
    return { ok: false };
  }

  for (var rowIndex = 0; rowIndex < rows.length; rowIndex += 1) {
    var rowCells = rows[rowIndex] && Array.isArray(rows[rowIndex].content) ? rows[rowIndex].content : [];
    var removeAt = Math.min(context.colIndex, rowCells.length - 1);
    rowCells.splice(removeAt, 1);
  }

  return {
    ok: true,
    targetRow: context.rowIndex,
    targetCol: Math.min(context.colIndex, minCols - 2)
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
