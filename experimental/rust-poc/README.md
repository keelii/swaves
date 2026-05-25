# rust-poc

Stage-0 Rust baseline for the swaves rewrite plan.

## Scope

- `axum` HTTP skeleton
- `rusqlite` SQLite baseline open path with Go-aligned PRAGMA setup
- `minijinja` loader, path join, lenient undefined behavior, and a minimal helper/filter set
- Named route registry with `UrlFor`
- Existing template render probes for:
  - `web/templates/dash/dash_login.html`
  - `web/templates/themes/tuft/post.html`

## Acceptance checks covered here

- Existing template files remain unchanged.
- Named-route URL generation is exercised from templates.
- Existing admin login and a site post page render through the Rust runtime baseline.
- SQLite initialization path is validated at startup.

## Commands

```bash
cd /home/runner/work/swaves/swaves/experimental
cargo test -p rust-poc
cargo build -p rust-poc
cargo run -p rust-poc
```

## Demo routes

- `GET /api/template-probe`
- `GET /dash/login`
- `GET /posts/demo`
