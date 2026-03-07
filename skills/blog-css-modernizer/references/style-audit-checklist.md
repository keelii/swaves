# Style Audit Checklist

## A) File map for this repository

- Site shell and embedded page styles: `web/templates/site/layout.html`
- Site pages: `web/templates/site/home.html`, `web/templates/site/list.html`, `web/templates/site/post.html`, `web/templates/site/detail.html`
- Shared icon component: `web/templates/lucide_icon.html`
- Site custom style entry: `web/static/site/style.css`
- Site typography baseline: `web/static/site/tufte-css/tufte.css`
- Admin custom style entry: `web/static/dash/style.css`
- Oat UI foundation: `web/static/dash/oat/css/00-base.css`, `web/static/dash/oat/css/01-theme.css`

## B) Site baseline extraction checklist

Before site implementation, extract and summarize:

1. Color tokens and semantic usage (`--primary`, neutral tones, borders, muted text)
2. Typography stack and scale (body, heading, small text, metadata text)
3. Spacing rhythm (margin/padding cadence and vertical gaps)
4. Component states (normal/hover/focus/active/disabled)
5. Content density (line length, list density, card density)
6. Breakpoint behavior and overflow handling

## C) Site modernization recipe (incremental)

Prefer this order:

1. Normalize tokens (colors, radius, spacing, motion duration)
2. Fix hierarchy (visual contrast between title/body/meta/actions)
3. Improve interaction feedback (hover/focus/pressed consistency)
4. Improve responsive layout (navigation, table/list wrapping, comment form usability)
5. Refine micro-visuals (subtle borders, restrained shadows, cleaner separators)

## D) Site guardrails

- Do not replace the whole design language unless explicitly requested
- Do not introduce a heavy CSS framework for small styling tasks
- Do not replace semantic HTML structure with div-only wrappers
- Do not remove readable focus indicators
- Do not break long-form reading comfort for article content

## E) Admin alignment checklist

1. Reuse Oat classes/components before creating custom class names
2. Keep spacing, radius, border, and color decisions aligned to Oat theme tokens
3. Put custom admin overrides in `web/static/dash/style.css` only
4. Avoid broad resets or selectors that can break Oat defaults
5. Keep hover/focus/active feedback aligned with Oat behavior

## F) Final acceptance checklist

- User requirements are directly reflected in selectors/components
- Existing style language is still recognizable for both site and admin
- Main user path works on desktop and small screens
- No obvious specificity conflicts or duplicated hard-coded tokens
- New styles are grouped logically and easy to maintain
