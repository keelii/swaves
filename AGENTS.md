# Swaves Engineering Guide (Concise)

These rules are mandatory by default.
Any exception must be explicitly approved, and the owner must confirm whether this guide should be updated.

## 5 Engineering Values

1) Correctness first, then simplicity and maintainability.

2) Keep code direct and intuitive: avoid over-abstraction and over-encapsulation.

3) Maintain a single source of truth per concern (route, path, state, config).

4) Keep system behavior honest: user-visible behavior must match real capability.

5) Prefer explicit boundaries and readable flow over hidden magic.

## 5 Engineering Constraints

1) Route and URL discipline:
- Separate route paths from content prefixes.
- Use named routes and `UrlFor` for internal links/redirects.
- Do not hardcode admin paths.

2) Template discipline:
- MiniJinja only, `.html` only.
- Keep template context flat and explicit; do not reintroduce wrapper namespaces.
- Register helpers directly via `env.AddFilter`/`env.AddFunction`.
- Use template-root-relative paths with explicit `.html` suffix.

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
