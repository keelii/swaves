# seditor (swaves editor)

`seditor` is a small, self-contained ProseMirror bundle that exports a single
global init API.

## Build

From the repository root:

```bash
cd web/static/seditor
npm install
npm run build
```

This produces:

- `web/static/seditor/dist/seditor.js`
- `web/static/seditor/dist/seditor.js.map`

## Use (minimal)

Include the bundle:

```html
<script src="/static/seditor/dist/seditor.js"></script>
```

Mount an editor:

```html
<div class="content-editor"></div>
<textarea id="post-content" hidden></textarea>

<script>
  window.SEditor.init({
    mount: ".content-editor",
    textarea: "#post-content"
  });
</script>
```

## Toolbar bindings

Any element with `data-seditor-command="..."` will be bound as a button:

- `bold`
- `italic`
- `blockquote`
- `bullet_list`
- `ordered_list`
- `undo`
- `redo`

## Headings

Typing `# ` / `## ` / `### ` ... at the beginning of a paragraph turns it into a
heading (`h1`..`h6`).

## Lists

Typing `1. ` at the beginning of a paragraph turns it into an ordered list.

Typing `* ` at the beginning of a paragraph turns it into a bullet list.

Typing `> ` at the beginning of a paragraph turns it into a blockquote.

Example:

```html
<button type="button" data-seditor-command="bold">Bold</button>
<button type="button" data-seditor-command="italic">Italic</button>
```

## Raw blocks

This v1 bundle recognizes these patterns and preserves them as raw blocks:

- display math blocks (`$$ ... $$`)
- footnote definition blocks (`[^id]: ...`)

They are editable and serialized back to Markdown byte-for-byte **inside the raw
block** (other non-raw parts may be normalized by the Markdown serializer).
