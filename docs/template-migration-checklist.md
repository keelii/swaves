# Template Migration PR Checklist

- [ ] Template files use `.html` only.
- [ ] Updated templates match directory role (`layouts/pages/macros/partials`).
- [ ] No `Req/Auth/Site` reserved-key collision is introduced.
- [ ] Page templates use `extends` (except layout or macro files).
- [ ] Reusable UI uses macro-first, not copy-paste.
- [ ] `import` uses aliases and no wildcard import.
- [ ] `include` is used only for lightweight fragments.
- [ ] Recursive templates are migrated to `for ... recursive` style.
- [ ] Internal links use `url_for`.
- [ ] No new hardcoded admin paths are introduced.
- [ ] New code uses `Req/Auth/Site` only.
- [ ] Development hot reload uses `ClearTemplates()` before render.
- [ ] Production does not clear template cache per request.
- [ ] Manual regression for key pages is completed (site and admin critical routes).
