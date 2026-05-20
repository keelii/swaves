# Rust POC Parity Checklist

- [x] 不做 Go/Rust 并存运行路径
- [x] CLI 入口（sqlite/listen/daemon/worker/max_failures）
- [x] supervisor -> worker 生命周期
- [x] 信号处理（Ctrl+C 优雅退出）
- [x] `site + dash + api` 最小路由
- [x] SQLite 连接与初始化 SQL
- [x] 任务调度与状态回写（`t_task_runs`）
- [x] MiniJinja 渲染探针（含 `url_for`）
- [x] Markdown 基础渲染链
- [x] `.cache` 与 `.cache/updater` 目录约束
- [x] 对齐 Go `InitialSQL` 全量语义
- [ ] 对齐 Go 路由全集与关键 handler 行为
- [ ] 对齐错误日志上下文与用户可操作报错
- [ ] 对齐任务注册全集与启动/关闭幂等性
- [x] 对齐模板 include/filter/function/path 复杂组合
- [ ] 建立性能基线并与 Go 对照
