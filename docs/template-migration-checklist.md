# Template Migration PR Checklist

- [ ] Template files use `.html` only.
- [ ] No `Req/Auth/Site/__root` reserved-key collision is introduced.
- [ ] Fragment reuse uses `import/include/macro` with explicit params.
- [ ] Recursive templates are migrated away from Go `define/template`.
- [ ] Internal links use `url_for`.
- [ ] No new hardcoded admin paths are introduced.
- [ ] New code uses `Req/Auth/Site` (and only internal compatibility key `__root`).
- [ ] Development hot reload uses `ClearTemplates()` before render.
- [ ] Production does not clear template cache per request.
- [ ] Manual regression for key pages is completed (site and admin critical routes).
