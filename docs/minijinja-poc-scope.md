# MiniJinja PoC Scope (Site First)

## In scope

- Site templates only:
  - layout
  - home
  - post
  - list and pagination partials
- Introduce a renderer adapter contract aligned with `Req/Auth/Site`.
- Keep existing business handlers unchanged as much as possible.

## Out of scope

- Admin templates.
- Cross-cutting UI redesign.
- New feature behavior changes.
- Global full migration.

## Success criteria

- Site pages render with equivalent behavior.
- Context access contract is stable (`Req/Auth/Site`).
- Template `extends/import/macro/include` conventions are validated.
- No regression in core navigation and pagination paths.

## Exit criteria

- PoC pages pass manual regression.
- Team agrees the conventions are practical.
- Migration estimate for the admin side becomes predictable.

