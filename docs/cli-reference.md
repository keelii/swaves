# swaves CLI 参数表

## 1. 子命令与入口参数

| 范围     | 参数 / 用法                                                  | 说明                                                           |
|--------|----------------------------------------------------------|--------------------------------------------------------------|
| 启动     | `swaves <sqlite-file>`                                   | SQLite 数据库文件路径；未提供时可改用环境变量配置。                                |
| 升级     | `swaves upgrade`                                         | 下载并安装当前平台的最新稳定版。                                             |
| 密码哈希   | `swaves hash-password <raw-password>`                    | 生成 bcrypt 密码哈希。                                              |
| 设置后台密码 | `swaves set-admin-password <sqlite-file> <raw-password>` | 直接修改指定 SQLite 中的 `settings.dash_password`，并输出写入后的 bcrypt 哈希。 |

## 2. 运行参数

| 参数                 | 类型     | 默认值              | 说明                                                                   |
|--------------------|--------|------------------|----------------------------------------------------------------------|
| `--help`/`-h`      | string | -                | 帮助信息                                                                 |
| `--version`/`-v`   | string | -                | 版本信息                                                                 |
| `--backup-dir`     | string | `.cache/backups` | 备份目录。                                                                |
| `--listen-addr`    | string | `:4096`          | HTTP 监听地址。                                                           |
| `--app-name`       | string | `swaves`         | 应用名，用于运行日志与 Fiber `AppName`。                                         |
| `--enable-sql-log` | bool   | 由运行环境推导          | 是否打开 SQL 日志；默认仅在 `dev` 环境开启。                                         |
| `--daemon-mode`    | int    | `1`              | `1` 表示启用 master + worker 模式；`0` 表示直接以前台 worker 方式运行。Windows 不支持 `1`。 |
| `--max-failures`   | int    | `5`              | master 允许 worker 连续失败的最大次数；`<= 0` 表示不限次数。                            |
| `--worker-process` | bool   | `false`          | 内部使用。master 拉起 worker 时自动注入，普通用户不应手动传入。                              |
