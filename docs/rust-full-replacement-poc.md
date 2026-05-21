# Rust Full-Replacement POC (Experimental)

## 决策边界

- 本 POC 明确为**全量替代实验**，不考虑与 Go 并存。
- 不引入双引擎、双运行时、灰度切换路径。
- 目标是验证 Rust 单实现对核心链路的可达性与语义一致性。

## 范围

### P0（当前已落地骨架）

- CLI + supervisor/worker + 信号退出。
- 最小 `site + dash + api` 路由与关键 handler 子集。
- 已补手动任务触发与 task runs 查看，验证 Go 任务页关键操作闭环。
- SQLite 接入与任务状态写回。
- Go `InitialSQL` 复用，避免 Rust 侧重复维护 schema。
- MiniJinja 渲染探针与 `url_for` helper。
- 模板 loader + include/path join/filter/function 组合探针。
- API/页面错误返回已带基础可操作提示，并记录路由上下文日志。
- Go 对齐的内置任务注册已写入 SQLite，并补了启动/关闭幂等调度生命周期。
- `.cache` / `updater` 路径约束实现。

### P1（下一阶段）

- Markdown 扩展链与 goldmark 语义对齐（TOC/mermaid/unsafe HTML）。
- 模板 include/filter/function/path 组合行为对齐。
- 资产 provider 抽象与设置切换一致性。

### P2（后续）

- 监控指标与后台低频专业模块。
- 前端 SUI 相关开发态联调闭环。

## 验收标准

- 功能等价：核心 API/任务/管理操作可跑通。
- 数据等价：同 SQLite 库读写不破坏语义。
- 行为等价：错误处理、日志、任务生命周期一致。
- 性能基线：启动时间、常见接口延迟、内存占用在可接受区间。

## 当前实现位置

- `experimental/rust-poc/`
