# swaves-rust-poc

这是 swaves 的**实验性全量替代迁移 POC**，明确不考虑与 Go 并存。

## 目标

- 不做双运行时兼容。
- 不保留线上 Go/Rust 运行时切换路径。
- 验证 Rust 单实现对核心能力的可覆盖性。

## 当前覆盖（最小闭环）

- 启动与进程模型：CLI + supervisor/worker + Ctrl+C 优雅退出。
- Web 与路由：最小 `site + dash + api` 路由。
- Web 与路由：补到关键 handler 子集（`/api/slug`、`/api/markdown`、`/dash/posts`、`/dash/tasks`）。
- 数据层：SQLite 初始化与任务运行状态回写示例。
- 数据层：直接复用 Go `InitialSQL` 作为 schema 真源。
- 任务系统：补齐 Go 对齐的内置任务注册、调度生命周期与 `t_task_runs` 状态回写。
- 模板与渲染：MiniJinja `url_for`、relative include、path join、filter 组合示例。
- Markdown：基础 markdown->html 渲染。
- 文件与缓存：`.cache` 与 `.cache/updater` 路径约束。

## 非目标

- 与现有 Go runtime 并存。
- 线上平滑回滚策略。
- 一次性覆盖全部业务模块。

## 运行

```bash
cd experimental/rust-poc
cargo run -- ../../data.sqlite --listen-addr 127.0.0.1:4096
```

## 下一步

1. 继续补齐 `site + dash + api` 的剩余真实路由与业务处理。
2. 建立对等验收集（响应、错误、任务生命周期、性能基线）。
3. 继续对齐 Go 路由全集与关键 handler 行为。
