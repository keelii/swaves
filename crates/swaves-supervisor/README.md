# swaves-supervisor

`swaves-supervisor` 提供 swaves 主控进程与工作进程之间使用的运行时协议，以及围绕协议文件的最小 supervisor 控制 API。

## 功能概览

- 约定运行时缓存目录结构
- 读写运行时信息文件
- 读写重启请求文件
- 封装 supervisor 基础配置校验
- 提供轻量级 `SupervisorRuntime` 入口

## 安装

```toml
[dependencies]
swaves-supervisor = { path = "../crates/swaves-supervisor" }
```

## 基本使用

```rust
use std::path::PathBuf;

use swaves_supervisor::{
    RestartRequest, RuntimeInfo, RuntimeLayout, SupervisorConfig, SupervisorRuntime,
};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let layout = RuntimeLayout::new("/tmp/swaves-runtime");
    let runtime = SupervisorRuntime::new(SupervisorConfig {
        listen_addr: "127.0.0.1:8080".into(),
        worker_command: PathBuf::from("/opt/swaves/swaves"),
        max_failures: 3,
        ready_timeout_secs: 10,
        shutdown_timeout_secs: 10,
        drain_timeout_secs: 10,
        runtime_layout: layout.clone(),
    })?;

    runtime.publish_runtime_info(&RuntimeInfo {
        pid: 42,
        executable: "/opt/swaves/swaves".into(),
        args: vec!["swaves".into(), "--worker".into()],
        working_dir: Some("/opt/swaves".into()),
        version: Some("v1.0.0".into()),
        updated_at_unix: 1_748_411_200,
    })?;

    runtime.queue_restart(&RestartRequest::upgrade(
        1_748_412_000,
        "v1.0.1",
        "swaves_v1.0.1_linux_amd64.tar.gz",
    ))?;

    Ok(())
}
```

## 运行时目录结构

`RuntimeLayout::new("/tmp/swaves-runtime")` 会约定如下路径：

- `master-runtime.json`：运行中主进程的当前信息
- `restart-request.json`：待处理的重启/升级请求
- `updater/`：升级流程使用的暂存目录

## 导出的主要 API

- `RuntimeLayout`：协议文件路径与读写入口
- `RuntimeInfo`：主进程运行时信息
- `RestartRequest` / `RestartReason`：重启请求模型
- `SupervisorConfig`：supervisor 配置与校验
- `SupervisorRuntime`：对外暴露的 supervisor 运行时封装
