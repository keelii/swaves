# Swaves Template Conventions (MiniJinja Migration)

## Scope

This document defines template conventions for the MiniJinja migration.
It focuses on template structure and review rules.
It does not define business logic.

## MUST

### 1) File extension

- All templates MUST use `.html`.
- Mixed extensions (`.jinja`, `.j2`) are not allowed.

### 2) Route and link generation

- Internal links and redirects in templates MUST use `UrlFor`.
- Hardcoded dash paths MUST NOT be introduced.

### 3) Composition style

- Composition MUST use MiniJinja-native patterns:
  - cross-file fragments via `import` + macro call
  - macro internals may use `with` + `include` to pass explicit context
  - layout composition via `embed`
- Custom `template("...", ctx)` compatibility calls are not allowed.
- Recursive fragments MUST use MiniJinja macros/recursive loops.

### 4) View behavior boundary

- Templates MUST remain DOM-oriented.
- Page behavior code MUST live in shared/static JS, not inline template scripts, unless the code is a small bootstrapping snippet that only passes server-rendered config into an existing JS module.
- Reusable behavior MUST be extracted to `web/static/dash/main.js` or the site entry for that runtime; page-specific behavior MAY live in a dedicated page script under `web/static/...`.
- Inline `<style>` blocks in templates SHOULD be limited to small page-local adjustments; reusable styles MUST move into static stylesheet entries.

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

### 4) Frontend behavior style

- Dash frontend behavior SHOULD use native DOM APIs consistently; do not reintroduce `jQuery` without explicit approval.
- Do not mix multiple frontend libraries inside one page/module without explicit approval.
- Prefer shared helpers in `web/static/dash/main.js` before adding new page-local helpers.
- Inline bootstrapping code SHOULD stay short: collect DOM refs, read server config, call shared/page-local helpers.

## MUST NOT

- MUST NOT introduce new hardcoded dash URL strings in templates.
- MUST NOT mix template role responsibilities in one file.
- MUST NOT implement large page controllers directly inside template files.
- MUST NOT silently depend on hidden context injection.
- MUST NOT add migration logic into unrelated initialization flow.

## Review gate

A template PR is mergeable only if:

- extension rule passes (`.html` only)
- route generation rule passes (`UrlFor`)
- reusable fragment APIs keep explicit parameters
