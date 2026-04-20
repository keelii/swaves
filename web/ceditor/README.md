# CEditor

CEditor is the minimal CodeMirror 6 based source editor used by the Swaves theme editor.

## Scope

- HTML / Jinja templates
- CSS
- JavaScript
- textarea sync for normal HTML form submission

## Build

```bash
npm install --prefix web/ceditor
make ceditor
```

Build output:

- `web/static/ceditor/dist/ceditor.js`
- `web/static/ceditor/dist/ceditor.min.js`
- `web/static/ceditor/ceditor.css`
