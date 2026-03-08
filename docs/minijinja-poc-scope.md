# MiniJinja Full Migration Scope

## In scope

- All site templates.
- All dash dash templates.
- Introduce a renderer adapter contract aligned with `Req/Auth/Site`.
- Keep existing business handlers unchanged as much as possible.
- Keep legacy layout/embed composition behavior for compatibility.
- Replace recursive template patterns with MiniJinja-compatible macro recursion.
- Keep template file extension as `.html` for all migrated files.

## Out of scope

- Cross-cutting UI redesign.
- New feature behavior changes.
- Runtime dual-engine switch.
- Rollback via old engine path.

## Success criteria

- Site and dash pages render with equivalent behavior.
- Context access contract is stable (`Req/Auth/Site`).
- Template compatibility conventions (`template(...)`, macro recursion) are validated.
- No regression in core navigation, pagination, and dash workflows.
- Development hot reload works without app restart.

## Exit criteria

- Full template set passes regression checks.
- Team accepts conventions as default standard.
- Legacy Go template engine path is fully removed.
