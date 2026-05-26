use criterion::{Criterion, black_box, criterion_group, criterion_main};
use swaves_markdown::{RenderOptions, render, render_toc, split};

const FRONT_MATTER_DOC: &str = r#"---
title: Benchmark
tags:
  - rust
  - markdown
summary: Baseline benchmark fixture
---

# Heading

Paragraph text for the benchmark fixture.
"#;

const RENDER_DOC: &str = r#"# 标题

Intro paragraph with **bold text**, _italic text_, and a [link](https://example.com).

## Checklist

- [x] one
- [ ] two
- item with `inline code`

## Table

| Name | Value |
| --- | --- |
| alpha | 1 |
| beta | 2 |

## Code

```rust
fn greet(name: &str) -> String {
    format!("hello, {name}")
}
```

## Image

![diagram](/diagram.png "Diagram")

## Notes

Here is a footnote reference.[^1]

[^1]: Footnote content.
"#;

const TOC_DOC: &str = r#"# One
## Two
### Three
## Four
# Five
"#;

fn benchmark_markdown(c: &mut Criterion) {
    let mut group = c.benchmark_group("markdown");
    let default_options = RenderOptions::default();
    let core_only_options = RenderOptions {
        render_math: false,
        render_mermaid: false,
        ..RenderOptions::default()
    };

    group.bench_function("split_front_matter", |b| {
        b.iter(|| split(black_box(FRONT_MATTER_DOC)).expect("split benchmark should succeed"))
    });

    group.bench_function("render_default", |b| {
        b.iter(|| {
            render(black_box(RENDER_DOC), black_box(&default_options))
                .expect("render benchmark should succeed")
        })
    });

    group.bench_function("render_core_only", |b| {
        b.iter(|| {
            render(black_box(RENDER_DOC), black_box(&core_only_options))
                .expect("core render benchmark should succeed")
        })
    });

    group.bench_function("render_toc", |b| {
        b.iter(|| render_toc(black_box(TOC_DOC)).expect("toc benchmark should succeed"))
    });

    group.finish();
}

criterion_group!(benches, benchmark_markdown);
criterion_main!(benches);
