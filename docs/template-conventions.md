# Swaves Template Conventions (MiniJinja Migration)

## Scope

This document defines template conventions for the MiniJinja migration.
It focuses on template structure, context contract, and review rules.
It does not define business logic.

## MUST

### 1) File extension

- All templates MUST use `.html`.
- Mixed extensions (`.jinja`, `.j2`) are not allowed.

### 2) Root context contract

- Root reserved keys MUST be:
  - `Req`
  - `Auth`
  - `Site`
- Business payload remains at root level (no mandatory `Page` wrapper).

### 3) Reserved key protection

- Business data MUST NOT overwrite `Req`, `Auth`, or `Site`.
- Renderer MUST fail fast in development if collision is detected.

### 4) Route and link generation

- Internal links and redirects in templates MUST use `url_for`.
- Hardcoded admin paths MUST NOT be introduced.

### 5) Template roles

- `layouts/*.html`: layout skeletons only.
- `pages/*.html`: page templates only.
- `macros/*.html`: macro definitions only.
- `partials/*.html`: simple fragments only.

### 6) Extends and blocks

- Every page template MUST use `extends` (except layout and macro files).
- Shared block names MUST stay consistent:
  - `title`
  - `head`
  - `content`
  - `scripts`

### 7) Macro API discipline

- Reusable UI MUST be implemented with macros first.
- Macros MUST take explicit parameters.
- Macros MUST NOT rely on implicit ambient variables.

### 8) Import style

- Imports MUST use aliases.
  - Example: `{% import "macros/forms.html" as forms %}`
- Wildcard import style is not allowed.

### 9) Full migration policy

- Migration policy is full replacement.
- Runtime engine switch is not allowed.
- Rollback path via dual-engine mode is not allowed.

### 10) Recursive templates

- Recursive rendering MUST use MiniJinja recursive loop style (`for ... recursive` + `loop(...)`).
- Legacy recursive `define/template` patterns from Go templates MUST be replaced.

### 11) Development hot reload

- Development hot reload MUST use strategy 1:
  - call `ClearTemplates()` before rendering.
- Production MUST NOT use per-request template cache clearing.

## SHOULD

### 1) Keep templates display-oriented

- Move complex branching and aggregation into Go handlers/services.
- Keep template logic shallow and readable.

### 2) Include usage

- Use `include` for simple fragments (header, footer, empty-state).
- Prefer macros when parameters or behavior are non-trivial.

### 3) Naming

- Files SHOULD use snake_case.
- Macro names SHOULD follow `domain_action` pattern.
  - Example: `table_row_actions`, `form_field_text`

### 4) Context readability

- Prefer reading system data from:
  - `Req.RouteName`, `Req.Path`, `Req.Query`, `Req.ReqID`
  - `Auth.IsLogin`
  - `Site.Settings`

## MUST NOT

- MUST NOT introduce new hardcoded admin URL strings in templates.
- MUST NOT mix template role responsibilities in one file.
- MUST NOT silently depend on hidden context injection.
- MUST NOT add migration logic into unrelated initialization flow.

## Review gate

A template PR is mergeable only if:

- extension rule passes (`.html` only)
- reserved key rule passes
- route generation rule passes (`url_for`)
- new reusable UI follows macro-first rule
