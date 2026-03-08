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
- Renderer internally injects `__root` for MiniJinja compatibility.
- Business payload remains at root level (no mandatory `Page` wrapper).

### 3) Reserved key protection

- Business data MUST NOT overwrite `Req`, `Auth`, or `Site`.
- Business data MUST NOT overwrite `__root`.
- Renderer MUST fail fast in development if collision is detected.

### 4) Route and link generation

- Internal links and redirects in templates MUST use `url_for`.
- Hardcoded dash paths MUST NOT be introduced.

### 5) Composition style

- Composition MUST use MiniJinja-native patterns:
  - cross-file fragments via `import` + macro call
  - macro internals may use `with` + `include` to pass explicit context
  - layout composition via `embed`
- Custom `template("...", ctx)` compatibility calls are not allowed.
- Recursive fragments MUST use MiniJinja macros/recursive loops.

### 9) Full migration policy

- Migration policy is full replacement.
- Runtime engine switch is not allowed.
- Rollback path via dual-engine mode is not allowed.

### 10) Recursive templates

- Legacy recursive `define/template` patterns from Go templates MUST be replaced.
- Recursive rendering MAY use macro recursion or MiniJinja recursive loops.

### 11) Development hot reload

- Development hot reload MUST use strategy 1:
  - call `ClearTemplates()` before rendering.
- Runtime toggle uses env var `SWAVES_TEMPLATE_RELOAD`:
  - `1/true/yes/on` => enabled
  - unset/other => disabled
- Production MUST NOT use per-request template cache clearing.

## SHOULD

### 1) Keep templates display-oriented

- Move complex branching and aggregation into Go handlers/services.
- Keep template logic shallow and readable.

### 2) Reuse style

- Prefer `import` + macro as the default cross-file reuse style.
- Prefer macro wrappers with explicit `ctx` fields for predictable context flow.

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

- MUST NOT introduce new hardcoded dash URL strings in templates.
- MUST NOT mix template role responsibilities in one file.
- MUST NOT silently depend on hidden context injection.
- MUST NOT add migration logic into unrelated initialization flow.

## Review gate

A template PR is mergeable only if:

- extension rule passes (`.html` only)
- reserved key rule passes
- route generation rule passes (`url_for`)
- reusable fragment APIs keep explicit parameters
