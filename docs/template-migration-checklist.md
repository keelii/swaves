# Template Migration PR Checklist

- [ ] Template files use `.html` only.
- [ ] Updated templates match directory role (`layouts/pages/macros/partials`).
- [ ] No `Req/Auth/Site` reserved-key collision is introduced.
- [ ] Page templates use `extends` (except layout or macro files).
- [ ] Reusable UI uses macro-first, not copy-paste.
- [ ] `import` uses aliases and no wildcard import.
- [ ] `include` is used only for lightweight fragments.
- [ ] Internal links use `url_for`.
- [ ] No new hardcoded admin paths are introduced.
- [ ] New code uses `Req/Auth/Site` and does not spread legacy keys.
- [ ] Manual regression for key pages is completed (home, post, admin list).

