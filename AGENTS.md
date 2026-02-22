# Swaves Engineering Guide

This file captures working rules from the recent development thread.
It is intended as a practical guide for future changes.

## Category 0) Governance

### 0) Rule Governance

- These rules are mandatory by default and must not be broken unless explicitly approved.
- If a task conflicts with these rules, you must confirm with the owner before execution.
- After executing any approved exception, you must explicitly confirm whether `AGENTS.md` should be updated.

## Category A) Architecture and Routing

### 1) Core Principles

- Keep behavior correct first, then optimize for simplicity and maintainability.
- Prefer one clear source of truth per concern (route, path, state, config).
- Allow small, intentional duplication when it improves readability.
- Avoid over-abstraction for basic wiring tasks (for example, route registration).
- Keep user-visible behavior consistent with actual system capability.
- If type information or static analysis can already prove safety, do not add extra defensive checks only for style.
- Keep nil/empty guards when they protect concurrency or lifecycle boundaries (for example shared mutable resolver pointers, optional dependencies, startup order).

### 2) URL and Path Semantics

- Separate route paths from content URL prefixes.
  - `ByPrefix` style values are for public content URL building.
  - Route-oriented values are for Fiber route registration and redirects.
- Do not mix prefix semantics into route construction logic.
- Use named routes and `url_for` for internal links and redirects.
- Avoid hardcoded admin paths in templates and JS.

### 3) Admin Path and Redirects

- Any redirect to admin pages must respect configurable admin base path.
- Replace string-concatenated redirects with named-route resolution where possible.
- When introducing new admin pages, always assign route names.

### 13) Template Engine and Context Contract

- Template runtime is MiniJinja only; do not introduce dual-engine switch or fallback paths.
- Keep template files as `.html` for both site and admin.
- Template context is flat: expose request/session metadata as explicit top-level fields (for example `RouteName`, `Query`, `IsLogin`, `UrlPath`, `ReqID`) when templates need them.
- Do not reintroduce compatibility wrapper namespaces such as `Req`/`Auth`/`Site`/`__root`.
- Avoid blindly injecting all request locals into templates; pass explicit, stable fields through render helpers.
- Template paths must use template-root-relative form (no leading `/`); `extends`/`include`/`import`/`from` paths must explicitly include `.html` (for example `admin/layout/layout.html`).
- In HTML attributes that contain template expressions, keep outer attribute quotes as `"` and use `'` for template string literals (for example `href="{{ url_for('admin.comments.list') }}"`).
- Internal template links must continue using `url_for`, not hardcoded admin paths.
- Development hot reload uses `SWAVES_TEMPLATE_RELOAD`; production must not clear template cache per request.

### 17) Template Layer Conventions (MiniJinja)

- `admin` and `site` are weakly isolated at folder level: keep their templates separated by directory, but code-level reuse is allowed when explicit (for example shared helpers under `internal/share`).
- Keep domain-local template structure consistent: `admin/layout|include|macro` and `site/layout|include|macro`; do not mix admin templates into site folders or vice versa.
- Use `macro` for atomic reusable HTML snippets; keep macro logic shallow and avoid embedding page-level business flow.
- Use `include` for reusable business components/modules that can be composed by pages/layouts.
- In templates, prefer explicit macro imports (`{% from "admin/macro/ui.html" import action_btn_content %}`) over alias-wide imports (`{% import ... as ui %}`).
- Macro signatures should use explicit parameters (with defaults for optional args); avoid `ctx`/`dict` bag-style parameter passing when direct arguments are possible.
- Keep template expressions minimal and readable: remove redundant nested parentheses.
- For route generation, prefer direct key/value style in `url_for` calls; avoid Go-template-style `dict(...)` wrappers when the helper supports direct args.


## Category B) Product Workflow and Data Semantics

### 4) Media Library Rules

- Runtime uses one media provider at a time.
- Provider selection UI is not shown in media upload/list pages.
- Provider switches happen only in settings via default config.
- Media list page reads from database records, not provider list API calls.

### 5) Settings and Prefix Fields

- `PrefixValue` means prefix source/value, not display description.
- Frontend linkage between prefix fields is UI-only behavior.
- UI linkage must not change storage semantics implicitly.
- Validation must match current field meaning and URL composition behavior.

### 6) Import Workflow

- Use async import with explicit states (for example `importing`).
- Keep imported items editable before final confirmation.
- Confirm action finalizes state and writes edited values.
- Cancel action must clean related records (posts, tags, categories links).
- Return specific failure reasons to user, not generic "import failed" only.

### 18) Encrypted Post Privacy Boundary

- Encrypted posts are a privacy boundary for authors; any data associated with encrypted posts must stay isolated from regular post/domain tables and cross-module linkage.
- Do not embed reusable convenience features from public/normal content modules into encrypted post pages (for example media library picker or media upload entry points).
- Any new encrypted-post feature must be reviewed with privacy-first defaults: no implicit cross-reference, no auto-link to shared content assets, no leakage through shared management workflows.

## Category C) Runtime Reliability and Safety

### 7) Error Handling and Logging

- Any handled error path should include explicit logging with context.
- User-facing API errors should include actionable, specific messages.
- For external provider failures, include status code and concise response detail.
- Do not swallow errors silently in handlers or background jobs.

### 8) Database Initialization and Migrations

- Do not add migration logic into `InitDatabase`.
- Keep only `EnsureDefaultSettings` in initialization flow.
- Any schema/data migration must be implemented outside `InitDatabase`.
- Current development-phase rule: do not add runtime compatibility migrations for schema changes; directly update `InitialSQL` table definitions and restart the app/database manually.

### 10) Fiber v3 Upgrade Guidelines

- Re-check API moves between v2 and v3 before migration edits.
- Example: `DisableStartupMessage` belongs to `fiber.ListenConfig` in v3.
- Validate session encoding/decoding compatibility after upgrades.
- Upgrade middleware dependencies with matching major versions.

### 14) Background Job Lifecycle

- Task registry initialization must be idempotent and concurrency-safe.
- Start long-running scheduler initialization from app listen hooks, not during base app construction.
- Keep shutdown symmetric: stop scheduler/cron resources and wait for graceful stop completion.
- Registry and executor code must log explicit skip/failure reasons (for example nil store, duplicate init, unregistered job code).

## Category D) Code Organization and Config Hygiene

### 9) Models, Handlers, and Reuse

- Keep database operations in models layer when possible.
- Handlers should orchestrate flow, not embed complex SQL logic.
- Consolidate reusable JS utilities in `web/static/admin/main.js` and site equivalent.
- Shared JS utilities must have no dependency besides jQuery.

### 11) Env and Config Hygiene

- Keep environment variable definitions centralized in `env.go`.
- Use consistent prefixes and naming conventions.
- Remove unused env variables aggressively.
- Avoid duplicate constants for env names when direct `os.Getenv` is sufficient.

### 16) Rename and Move Hygiene

- Prefer `git mv` (or equivalent tracked rename/move operations) for file renames and directory restructuring to preserve history.

## Category E) Quality and Delivery

### 12) Testing Strategy

- Add unit tests for pure URL/path composition logic.
- Remove obsolete tests tied to removed functionality.
- Run full suite (`go test ./...`) after cross-cutting refactors.
- Fix all regressions except explicitly deferred items.

### 15) PR Self-Checklist

Before merge, verify all items below:

- [ ] No hardcoded admin URLs remain where route names exist.
- [ ] New routes have stable route names.
- [ ] Redirects and links respect configurable admin base path.
- [ ] Prefix/linkage changes do not alter persisted data semantics unexpectedly.
- [ ] Import failure responses include concrete error detail.
- [ ] Error branches log context-rich messages.
- [ ] No migration logic added to `InitDatabase` (except `EnsureDefaultSettings`).
- [ ] Development-phase schema changes update `InitialSQL` directly; no compatibility migration path added.
- [ ] Duplicate template/JS HTML blocks are synchronized or unified.
- [ ] Template context remains flat; templates do not depend on `Req/Auth/Site/__root` wrapper namespaces.
- [ ] Template `import/include` paths use template-root-relative paths (no leading `/`) with explicit `.html` suffix.
- [ ] Admin/site template directories remain weakly isolated (`admin/*` and `site/*`), with shared logic extracted explicitly.
- [ ] Macro/import style follows MiniJinja conventions (`from ... import ...` with explicit symbols, no alias-wide import drift).
- [ ] Macro signatures use explicit params/defaults; avoid `ctx` or dict-bag arguments where not necessary.
- [ ] Macro vs include responsibilities stay clear (macro=atomic snippet, include=business component).
- [ ] Template URL generation prefers direct key/value `url_for` args over `dict(...)` wrappers where supported.
- [ ] Encrypted post module remains privacy-isolated: no shared media/library embedding and no cross-module linkage regression.
- [ ] Template reload behavior follows `SWAVES_TEMPLATE_RELOAD` (dev-only per-request clearing).
- [ ] Job registry lifecycle remains safe (listen hook init + shutdown destroy).
- [ ] Added/updated tests cover new behavior and removed legacy behavior.
- [ ] Full `go test ./...` passes.
