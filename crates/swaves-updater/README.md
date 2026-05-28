# swaves-updater

`swaves-updater` 基于 `swaves-supervisor` 的运行时协议，负责描述升级资源和生成安装计划，本身不直接执行下载或解压动作。

## 功能概览

- 描述远端发布资产
- 描述本地升级包
- 根据当前运行中的 swaves 实例生成安装计划
- 生成并写入升级重启请求

## 安装

```toml
[dependencies]
swaves-updater = { path = "../crates/swaves-updater" }
swaves-supervisor = { path = "../crates/swaves-supervisor" }
```

## 使用远端发布资产生成安装计划

```rust
use swaves_supervisor::{RuntimeInfo, RuntimeLayout};
use swaves_updater::{InstallPlan, InstallSource, ReleaseAsset};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let layout = RuntimeLayout::new("/tmp/swaves-runtime");
    let runtime = RuntimeInfo {
        pid: 42,
        executable: "/opt/swaves/swaves".into(),
        args: vec!["swaves".into()],
        working_dir: Some("/opt/swaves".into()),
        version: Some("v1.9.0".into()),
        updated_at_unix: 1_748_411_200,
    };
    let source = InstallSource::Release(ReleaseAsset::new(
        "v2.0.0",
        "swaves_v2.0.0_linux_amd64.tar.gz",
        "https://example.invalid/swaves_v2.0.0_linux_amd64.tar.gz",
        Some("https://example.invalid/swaves_v2.0.0_linux_amd64.tar.gz.sha256".into()),
    )?);

    let plan = InstallPlan::for_active_runtime(&layout, &runtime, source, 1_748_412_000)?;

    assert_eq!(plan.target_executable().to_string_lossy(), "/opt/swaves/swaves");
    assert_eq!(plan.restart_request().target_version.as_deref(), Some("v2.0.0"));
    Ok(())
}
```

## 使用本地升级包写入重启请求

```rust
use swaves_supervisor::{RuntimeInfo, RuntimeLayout};
use swaves_updater::{InstallPlan, InstallSource, LocalArchive};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let layout = RuntimeLayout::new("/tmp/swaves-runtime");
    let runtime = RuntimeInfo {
        pid: 42,
        executable: "/opt/swaves/swaves".into(),
        args: vec!["swaves".into()],
        working_dir: Some("/opt/swaves".into()),
        version: Some("v1.9.0".into()),
        updated_at_unix: 1_748_411_200,
    };
    let source = InstallSource::Local(LocalArchive::new(
        "v2.0.1",
        "swaves_v2.0.1_linux_amd64.tar.gz",
        "/tmp/swaves_v2.0.1_linux_amd64.tar.gz",
    )?);

    let plan = InstallPlan::for_active_runtime(&layout, &runtime, source, 1_748_412_001)?;
    plan.queue_restart(&layout)?;
    Ok(())
}
```

## 导出的主要 API

- `ReleaseAsset`：远端升级资源描述
- `LocalArchive`：本地升级包描述
- `InstallSource`：统一的升级来源枚举
- `InstallPlan`：针对当前运行实例生成的安装计划与重启请求
