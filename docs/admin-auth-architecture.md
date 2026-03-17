# Swaves 管理端统一鉴权设计

## 1. 目标与范围

- 目标：让 Web 管理端与 Native 管理端使用同一套登录鉴权体系。
- 目标：保持 Web 公开端与管理端边界清晰，公开端不引入管理鉴权负担。
- 目标：统一身份、权限、审计模型，避免双端各自实现业务鉴权。
- 范围：本设计仅覆盖管理域，不覆盖公开站点访客互动鉴权。

## 2. 当前状态

- Web 管理端当前使用后台密码登录，密码存于 `dash_password` 设置项的 bcrypt 哈希。
- Web 管理端当前使用服务端 session + cookie 维持登录态。
- Web 管理端当前使用 CSRF token 防护写操作请求。
- Native 管理端尚未接入统一登录流。

参考实现：
- `internal/modules/dash/auth_handler.go`
- `internal/shared/types/global.go`
- `internal/platform/middleware/RequireDashAuth.go`
- `internal/platform/middleware/DashCSRF.go`
- `internal/platform/db/models.go` (`CheckPassword`)

## 3. 鉴权总方案

- 协议统一：`OAuth 2.1 / OIDC Authorization Code + PKCE`。
- 身份统一：Web 管理端与 Native 管理端共享同一身份提供方与用户目录。
- 授权统一：管理 API 使用统一的 scope/role 判定模型。
- 会话承载按终端最优实现：
  - Web 管理端：BFF session cookie。
  - Native 管理端：access token + refresh token。

说明：
- “同一种登录鉴权方式”的统一点是协议、身份与权限模型，不是 cookie/token 载体强行一致。

## 4. 终端登录流

## 4.1 Web 管理端

- Web 管理端发起登录，重定向到统一登录页。
- 登录成功后回调服务端 BFF。
- BFF 换取 token 后不下发给浏览器 JS，仅建立服务端 session。
- 浏览器保存 `HttpOnly + Secure + SameSite` cookie。
- Web 管理端后续请求由 BFF 代表用户调用管理 API。
- Web 管理端写操作继续启用 CSRF 防护。

## 4.2 Native 管理端

- Native 管理端通过系统浏览器打开统一登录页。
- Native 使用 PKCE 生成 `code_verifier` 与 `code_challenge`。
- 登录成功回调到 Native 注册的回调端点。
- Native 以 `authorization_code + code_verifier` 换取 token。
- Native 将 refresh token 存系统钥匙串，access token 放内存。
- Native 使用 bearer token 直接调用管理 API。

回调策略：
- 首选 loopback 回调（`http://127.0.0.1:<random-port>/callback`）。
- 备选自定义 scheme 回调（用于 loopback 不可用场景）。

## 5. 管理 API 鉴权契约

- 所有管理 API 必须有统一认证中间件。
- Token 至少包含：`sub`、`aud`、`iss`、`exp`、`scope`、`role`。
- 管理 API 最小 scope 建议：
  - `admin.read`
  - `admin.write`
  - `admin.task.run`
  - `admin.asset.manage`
- 不同模块由 scope 做最小权限校验，不依赖前端路由做权限控制。
- Web 与 Native 调用同一管理 API，响应结构保持一致。

## 6. 会话与令牌策略

建议默认值：

| 项目 | Web 管理端 | Native 管理端 |
|---|---|---|
| 登录协议 | Authorization Code + PKCE | Authorization Code + PKCE |
| 前端持有 access token | 否 | 是（内存） |
| refresh token | BFF 持有 | 持有并轮换 |
| 登录态载体 | 服务端 session cookie | Bearer token |
| 会话空闲超时 | 8-12 小时 | 8-12 小时 |
| 绝对过期 | 7-14 天 | 7-14 天 |
| CSRF 防护 | 必须 | 不适用（bearer） |

## 7. 安全控制要求

- Native 禁止内嵌 WebView 登录，必须使用系统浏览器。
- refresh token 必须启用轮换与重放检测。
- token 绑定 `aud`，管理 API 必须校验 `aud` 与 `iss`。
- access token 必须短时有效，建议 10-20 分钟。
- cookie 必须启用 `HttpOnly`、`Secure`、`SameSite`。
- 所有鉴权失败必须记录审计日志，含时间、终端类型、原因、请求来源。
- 统一登出需支持服务端撤销 session 与 token。
- Native 端离线缓存不得作为权限判定来源。

## 8. 权限模型建议

- 角色建议：
  - `owner`
  - `editor`
  - `operator`
  - `viewer`
- 角色与 scope 映射在服务端集中管理。
- 前端仅用于展示层禁用/隐藏，不作为最终授权判定。
- 双端功能差异由“终端能力”决定，不由“角色模型分叉”决定。

## 9. 迁移路线

## 9.1 Phase 0（保守并行）

- 保留现有 `dash_password + session` 登录。
- 新增统一身份提供方与管理 API 认证中间件。
- 引入用户、角色、授权模型数据表。

## 9.2 Phase 1（Web 管理端接入）

- Web 管理端改为统一登录入口。
- 由 BFF 接管 token，前端保持 cookie session 使用体验。
- 保持现有 CSRF 防护与写请求约束。

## 9.3 Phase 2（Native 管理端接入）

- Native 端接入 PKCE 登录流与 token 刷新。
- 接入系统钥匙串凭证存储与统一登出。
- Native 端开始走统一管理 API。

## 9.4 Phase 3（收敛旧模式）

- 下线 `dash_password` 作为主登录入口。
- 将旧登录保留为紧急恢复通道或完全移除。
- 完成双端审计、权限、会话策略一致化。

## 10. 验收标准

- Web 与 Native 使用同一登录入口与同一用户体系。
- 双端访问同一管理 API 且权限行为一致。
- Native 完成系统浏览器登录与 token 安全存储。
- Web 保持服务端 session 与 CSRF 防护。
- 关键审计事件可追踪：登录成功、登录失败、刷新、登出、撤销。
- 下线旧后台密码模式后，核心管理流程无功能回退。

## 11. 与三端需求文档关系

- 三端能力边界定义见 `docs/surface-requirements.md`。
- 本文是该文档中“管理端共享鉴权模型”的实现级设计补充。
