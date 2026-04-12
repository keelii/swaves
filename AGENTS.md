# Swaves Engineering Guide (Concise)

These rules are mandatory by default.
Any exception must be explicitly approved, and the owner must confirm whether this guide should be updated.

## 6 Engineering Values

1) Correctness first, then simplicity and maintainability.

2) Keep code direct and intuitive: avoid over-abstraction and over-encapsulation.

3) Maintain a single source of truth per concern (route, path, state, config).

4) Keep system behavior honest: user-visible behavior must match real capability.

5) Prefer explicit boundaries and readable flow over hidden magic.

6) Start with the smallest feature that solves the real user need; avoid introducing extra entities, options, edge-case flows, or speculative capabilities without clear demand.

## 5 Engineering Constraints

1) Route and URL discipline:
- Separate route paths from content prefixes.
- Use named routes and `UrlFor` for internal links/redirects.
- Do not hardcode dash paths.

2) Template discipline:
- MiniJinja only, `.html` only.
- Keep template context flat and explicit; do not reintroduce wrapper namespaces.
- Register helpers directly via `env.AddFilter`/`env.AddFunction`.
- Use template-root-relative paths with explicit `.html` suffix.
- Prefer `include` without `with`; included templates should read needed values directly from current context when feasible.
- Frontend templates should trust the shared admin runtime baseline: do not add repetitive existence guards around required APIs such as `console.warn`, `notify`, or `window.DashAppUI.{toast,dialog,confirm}` at call sites.
- Avoid meaningless defensive coercion in template scripts (for example `String(message || '')`) when the value is already known text or comes from the current UI flow; keep normalization only where unknown external input is actually being sanitized.

3) Data and workflow discipline:
- Keep settings/prefix semantics stable.
- Import flow must be async, editable-before-confirm, and cancel-cleanup capable.
- Encrypted post features must remain privacy-isolated.
- Runtime uses one asset provider at a time, with provider switching only in settings.

4) Runtime safety discipline:
- Never swallow errors; log handled error paths with context and return actionable messages.
- Keep migration logic out of `InitDatabase` (except `EnsureDefaultSettings`).
- In current development phase, update `InitialSQL` directly for schema changes; do not add compatibility migrations.
- Background job lifecycle must be idempotent, concurrency-safe, and symmetric on startup/shutdown.

5) Delivery quality discipline:
- Keep DB logic in models, keep handlers orchestration-focused.
- Centralize shared frontend navigation/redirect helpers; avoid ad-hoc `window.location.*` usage.
- Keep env definitions centralized and clean.
- Preserve file history when moving/renaming (prefer tracked move).
- Treat tests and PR checklist as a merge gate; run `go test ./...`.

6) Go CLI flag discipline:
- Declare all CLI flags at package top level via `var` with `flag.String`/`flag.Bool`/etc.
- Call `flag.Parse()` (or `flag.CommandLine.Parse(...)`) only in `main()`.
- Dereference flag pointers before use and validate required inputs explicitly in `main()` flow.
- Keep `flag` as input boundary only; pass plain values into business/runtime code.
- Keep CLI simple for single-command tools; avoid cross-package flag access and unnecessary wrappers.

## Current Project Baselines

### SUI admin baseline
- SUI migration is complete; treat SUI as the default admin UI baseline.
- New admin pages/features should be implemented in SUI first, while keeping backend semantics and route discipline unchanged.
- For modules/pages, prioritize component-layer consistency and reuse existing generic components (for example data list/table list pages) instead of rebuilding per-module list pages.

### SEditor (ProseMirror) v1 scope
- `web/seditor/` is a standalone editor workspace and only exports one public init API.
- Bundle editor with esbuild into a single JS artifact for integration.
- Current integration route is `/sui/post_edit` (`sui.post_edit`).
- v1 supports minimal markdown WYSIWYG: bold, italic, heading (`###` style), blockquote, ordered/unordered list.
- v1 explicitly does not add rendered support for footnote/formula; these stay raw text behavior.
- `raw_block` is editable in WYSIWYG mode using `<pre contenteditable>` style behavior, without preview rendering.
- Markdown fidelity requirement ("原文输出，一字不差") is guaranteed only for `raw_*` nodes (for example footnote/formula/raw blocks), not for full-document normalization-sensitive content.

### Editor UX requirements
- Post editor should autofocus to editable area on load.
- Markdown common toolbar includes blockquote action, with `quote` icon.
- Auto list input rules are required:
  - `1. ` -> ordered list
  - `* ` -> unordered list
- Heading input rule is required:
  - `### ` (and `#`..`######`) -> `h1`..`h6`

### SUI CSS conventions
- Keep stylesheet organized in this order:
  1) Global styles
  2) Component styles (`ui-*`)
  3) Framework layout styles (`app-*`, `status-*`, `nav-*`, `toolbar-*`)
  4) Page-level styles
- Prefer scoped component selectors over global element selectors for controls.
  - Do not style checkbox/radio via global `input[type=...]`; use component classes (for example `.ui-checkbox`, `.ui-radio`).
- Keep global link (`a`) styling as a deliberate project-level choice.
- For custom dropdown UI (`ui-dropdown`), keep native-select fallback for browsers without `showPicker` support.
