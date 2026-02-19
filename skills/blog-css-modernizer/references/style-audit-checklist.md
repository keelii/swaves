# Style Audit Checklist

## A) File map for this repository

- Frontend shell and embedded page styles: `web/templates/ui/layout.html`
- Frontend pages: `web/templates/ui/home.html`, `web/templates/ui/list.html`, `web/templates/ui/post.html`, `web/templates/ui/detail.html`
- Global/admin style entry: `web/static/style.css`
- Frontend typography baseline: `web/static/tufte-css/tufte.css`
- Oat UI foundation (mainly admin/components): `web/static/oat/css/00-base.css`, `web/static/oat/css/01-theme.css`

## B) Baseline extraction checklist

Before implementation, extract and summarize:

1. Color tokens and semantic usage (`--primary`, neutral tones, borders, muted text)
2. Typography stack and scale (body, heading, small text, metadata text)
3. Spacing rhythm (margin/padding cadence and vertical gaps)
4. Component states (normal/hover/focus/active/disabled)
5. Content density (line length, list density, card density)
6. Breakpoint behavior and overflow handling

## C) Modernization recipe (incremental)

Prefer this order:

1. Normalize tokens (colors, radius, spacing, motion duration)
2. Fix hierarchy (visual contrast between title/body/meta/actions)
3. Improve interaction feedback (hover/focus/pressed consistency)
4. Improve responsive layout (navigation, table/list wrapping, comment form usability)
5. Refine micro-visuals (subtle borders, restrained shadows, cleaner separators)

## D) Guardrails

- Do not replace the whole design language unless explicitly requested
- Do not introduce a heavy CSS framework for small styling tasks
- Do not remove readable focus indicators
- Do not break long-form reading comfort for article content

## E) Acceptance checklist

- User requirements are directly reflected in selectors/components
- Existing style language is still recognizable
- Main user path works on desktop and small screens
- No obvious specificity conflicts or duplicated hard-coded tokens
- New styles are grouped logically and easy to maintain
