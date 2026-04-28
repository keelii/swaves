# swaves 环境变量表

## 1. `swaves` 运行环境变量

| 环境变量                             | 默认值               | 说明                                                      |
|----------------------------------|-------------------|---------------------------------------------------------|
| `SWAVES_ENV`                     | `prod`            | 运行环境；支持 `prod`、`test`、`dev`                             |
| `SWAVES_SQLITE_FILE`             | 无                 | `swaves` 主程序使用的 SQLite 数据库文件路径                          |
| `SWAVES_BACKUP_DIR`              | `.cache/backups`  | 备份目录                                                    |
| `SWAVES_LISTEN_ADDR`             | `:4096`           | HTTP 监听地址                                               |
| `SWAVES_APP_NAME`                | `swaves`          | 应用名                                                     |
| `SWAVES_ENABLE_SQL_LOG`          | 由 `SWAVES_ENV` 推导 | 是否打开 SQL 日志；支持 Go `strconv.ParseBool` 可识别的布尔格式          |
| `SWAVES_DAEMON_MODE`             | `1`               | 运行模式；只接受 `0` 或 `1`                                      |
| `SWAVES_ENSURE_DEFAULT_SETTINGS` | `false`           | 仅在 `SWAVES_ENV=dev` 时生效；开启后允许执行 `EnsureDefaultSettings` |

## 2. 第三方服务与默认设置环境变量

| 环境变量                          | 默认值 | 说明                                                                |
|-------------------------------|-----|-------------------------------------------------------------------|
| `SWAVES_S3_ENDPOINT`          | 空   | S3 兼容接口地址；用于初始化默认设置值，并在部分备份流程中作为设置为空时的回退值                         |
| `SWAVES_S3_ACCESS_KEY_ID`     | 空   | S3 Access Key ID；用于初始化默认设置值，并在部分备份流程中作为设置为空时的回退值                  |
| `SWAVES_S3_SECRET_ACCESS_KEY` | 空   | S3 Secret Access Key；用于初始化默认设置值，并在部分备份流程中作为设置为空时的回退值              |
| `SWAVES_SEE_API_TOKEN`        | 空   | S.EE Bearer Token；当前作为默认设置值写入 `asset_see_api_token`               |
| `SWAVES_IMAGEKIT_PRIVATE_KEY` | 空   | ImageKit 服务端 Private Key；当前作为默认设置值写入 `asset_imagekit_private_key` |

## 3. 安装脚本环境变量

| 环境变量             | 默认值                | 说明                                                                 |
|------------------|--------------------|--------------------------------------------------------------------|
| `SWAVES_INSTALL` | `$HOME/.local/bin` | `install.sh` 安装目录；脚本会把 `swaves` 可执行文件写入 `${SWAVES_INSTALL}/swaves` |

