# swaves-rust-poc

这是 swaves 的**实验性全量替代迁移 POC**，明确不考虑与 Go 并存。

## 目标

- 不做双运行时兼容。
- 不保留线上 Go/Rust 运行时切换路径。
- 验证 Rust 单实现对核心能力的可覆盖性。

## 当前覆盖（最小闭环）

- 启动与进程模型：CLI + supervisor/worker + Ctrl+C 优雅退出。
- Web 与路由：最小 `site + dash + api` 路由。
- 数据层：SQLite 初始化与任务运行状态回写示例。
- 任务系统：cron 调度心跳任务并写入 `t_task_runs`。
- 模板与渲染：MiniJinja `url_for` 函数示例。
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

1. 用可验证方式复用 Go `InitialSQL` 全量语义。
2. 补齐 `site + dash + api` 的关键真实路由与业务处理。
3. 建立对等验收集（响应、错误、任务生命周期、性能基线）。
