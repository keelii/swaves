use std::sync::LazyLock;

use criterion::{Criterion, black_box, criterion_group, criterion_main};
use swaves_markdown::{RenderOptions, render, render_toc, split};

const MARKDOWN_FIXTURE: &str = include_str!("fixtures/render.md");

static FRONT_MATTER_DOC: LazyLock<String> = LazyLock::new(|| {
    format!(
        "---\ntitle: Benchmark\ntags:\n  - rust\n  - markdown\nsummary: Baseline benchmark fixture\n---\n\n{}",
        MARKDOWN_FIXTURE
    )
});

fn benchmark_markdown(c: &mut Criterion) {
    let mut group = c.benchmark_group("markdown");
    let default_options = RenderOptions::default();
    let core_only_options = RenderOptions {
        render_math: false,
        render_mermaid: false,
        ..RenderOptions::default()
    };

    group.bench_function("split_front_matter", |b| {
        b.iter(|| {
            split(black_box(FRONT_MATTER_DOC.as_str())).expect("split benchmark should succeed")
        })
    });

    group.bench_function("render_default", |b| {
        b.iter(|| {
            render(black_box(MARKDOWN_FIXTURE), black_box(&default_options))
                .expect("render benchmark should succeed")
        })
    });

    group.bench_function("render_core_only", |b| {
        b.iter(|| {
            render(black_box(MARKDOWN_FIXTURE), black_box(&core_only_options))
                .expect("core render benchmark should succeed")
        })
    });

    group.bench_function("render_toc", |b| {
        b.iter(|| render_toc(black_box(MARKDOWN_FIXTURE)).expect("toc benchmark should succeed"))
    });

    group.finish();
}

criterion_group!(benches, benchmark_markdown);
criterion_main!(benches);
