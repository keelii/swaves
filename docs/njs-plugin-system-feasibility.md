# Swaves 插件系统可行性研究：基于 nginx/njs

> 参考项目：https://github.com/nginx/njs
> 研究分支：`copilot/research-plugin-system-feasibility`

## 1. 背景与问题定义

### 1.1 研究动机

Swaves 当前是一个功能完整的 Go/Fiber 博客平台，架构完全内聚。
用户无法在不修改源码的情况下扩展系统行为（如自定义鉴权逻辑、请求拦截、响应后处理、第三方 Webhook 集成等）。

本研究评估以 **nginx/njs**（NGINX JavaScript 动态模块）作为 Swaves 插件运行时的技术可行性。

### 1.2 njs 简介

njs 是 NGINX 官方提供的 JavaScript 动态模块，允许在 nginx 的请求处理管道中运行 ECMAScript 5.1+（部分 ES6）脚本。
它以 `ngx_http_js_module`（HTTP）和 `ngx_stream_js_module`（TCP/UDP）两个模块形式提供，无需重新编译 nginx。

njs 的核心能力：
- 访问和修改请求头、URI、Body、变量
- 发起子请求（subrequest）到后端
- 访问响应内容并进行改写
- 异步 I/O、定时器、SharedDict（与 workers 共享 KV）
- 不是 Node.js，不支持 npm 生态，没有 `require` 文件系统模块
- 每个请求独立执行上下文，无持久化内存（SharedDict 除外）

---

## 2. Swaves 当前可扩展性分析

### 2.1 当前架构边界

```
访客 / 管理员
     │
     ▼
[Fiber v3 HTTP Server]  ← cmd/swaves/worker.go
     │
     ├── site 模块   (公开站点路由)
     ├── dash 模块   (管理后台路由)
     ├── sui 模块    (新管理后台路由)
     ├── api 模块    (内部 API)
     └── 中间件层    (CSRF, Auth, RateLimit, Context)
          │
          ▼
     [SQLite DB]
     [MiniJinja Templates]
     [Job Registry]
```

### 2.2 现有扩展点（Go 层）

| 扩展类型 | 当前状态 | 可插拔程度 |
|---|---|---|
| HTTP 中间件 | `internal/platform/middleware/` | 需改源码 |
| 后台任务 | `job.RegisterJob()` | 需改源码 |
| 路由注册 | `*Module*/router.go` | 需改源码 |
| 模板过滤器/函数 | `env.AddFilter/AddFunction` | 需改源码 |
| 数据库模型 | `internal/platform/db/` | 需改源码 |

结论：**当前无外部可注入扩展点**，所有扩展均需修改 Go 源码。

---

## 3. njs 作为插件运行时：集成方案

### 方案 A：njs 作为前置代理插件层

```
[用户请求]
    │
    ▼
[nginx + njs 插件脚本]  ← 插件在此层执行
    │
    ▼ (反向代理)
[Swaves Go 服务]  :8080
    │
    ▼
[SQLite / 模板 / 任务]
```

**机制说明**：
- nginx 以反向代理方式接收所有流量
- 用户将插件写为 `.js` 文件，在 nginx `js_import` 中加载
- 插件脚本可在 `access_by_js`、`content_by_js`、`header_filter_by_js`、`body_filter_by_js` 等阶段介入
- swaves 本身不做任何改动

**示例插件能力（njs 侧）**：

```javascript
// plugins/my_auth.js
import qs from 'querystring';

// 请求鉴权 hook：在请求到达 swaves 前检查自定义 API Key
export default { authenticate };

function authenticate(r) {
    const token = r.headersIn['X-Api-Token'];
    if (!validToken(token)) {
        r.return(401, 'Unauthorized');
        return;
    }
    r.internalRedirect('@swaves_backend');
}
```

```nginx
# nginx.conf
js_import plugins/my_auth.js;

server {
    location / {
        js_content plugins/my_auth.authenticate;
    }
    location @swaves_backend {
        proxy_pass http://127.0.0.1:8080;
    }
}
```

### 方案 B：njs 插件通过 Swaves Plugin API 调用后端数据

在方案 A 基础上，swaves 暴露专用的内部插件 API（仅对 loopback 可达）：

```
njs 插件脚本
    │
    ├── 调用 swaves /api/plugin/* (subrequest)
    │       ↓
    │   获取文章数据、设置项、用户 session 状态
    │
    └── 根据数据决定响应行为
```

**Swaves 需新增**：
- `GET /api/plugin/settings` — 返回部分公开设置
- `GET /api/plugin/post/:slug` — 返回文章元数据（供插件决策）
- 内部鉴权（仅允许 127.0.0.1 访问）

### 方案 C：Go 内嵌 JS 运行时（非 nginx/njs，但参考 njs 设计）

放弃 nginx 依赖，在 swaves Go 进程内嵌入 JS 引擎（如 [goja](https://github.com/dop251/goja) — Go 实现的 ECMAScript 5.1+ 运行时），提供类 njs 的插件接口。

**对比 njs**：
- goja 是纯 Go 实现，无 C 依赖，可直接调用 Go 函数
- 插件脚本与 swaves 进程共生，不需要 nginx
- 支持的 JS 特性与 njs 相近（ES5+部分ES6）
- 可访问 DB、Config、任务注册等所有 Go 内部资源

```go
// 伪代码：Go 侧插件 Host
type PluginContext struct {
    Request  *PluginRequest
    Response *PluginResponse
    DB       *db.DB
    Settings map[string]string
}

func RunPlugin(script string, ctx PluginContext) error {
    vm := goja.New()
    vm.Set("ctx", ctx)
    _, err := vm.RunString(script)
    return err
}
```

---

## 4. 插件用例评估

以下用例按博客平台实际需求优先级排序：

| 用例 | 方案 A (njs 代理层) | 方案 B (njs+Plugin API) | 方案 C (Go 内嵌 JS) |
|---|---|---|---|
| 自定义请求鉴权（IP 白名单、API Key） | ✅ 完全可行 | ✅ 完全可行 | ✅ 完全可行 |
| 请求限流/防刷 | ✅ SharedDict 实现 | ✅ | ✅ |
| 响应头注入（CSP、HSTS、自定义 Header） | ✅ | ✅ | ✅ |
| 内容后处理（HTML 注入追踪代码） | ✅ body_filter | ✅ | ✅ |
| Webhook 发送（文章发布通知） | ⚠️ 异步子请求，有限 | ⚠️ | ✅ 可直接调用 |
| 访问文章数据库（用于插件决策） | ❌ 无直接访问 | ✅ 通过 API | ✅ 直接 DB 访问 |
| 注册自定义后台任务 | ❌ | ❌ | ✅ job.RegisterJob |
| 扩展后台管理页面 | ❌ | ❌ | ⚠️ 需模板支持 |
| 修改 RSS 输出 | ⚠️ body_filter 改写 | ⚠️ | ✅ |
| 热加载（无重启加载插件） | ✅ nginx reload | ✅ | ⚠️ 需文件监听 |
| 插件调试友好性 | ❌ nginx 错误日志 | ⚠️ | ✅ Go 测试体系 |

---

## 5. 可行性分析

### 5.1 方案 A：njs 代理层

**优势**：
- swaves 本身零改动
- nginx 已是常见生产部署前置（TLS、静态文件、缓存）
- njs 生命周期独立，插件崩溃不影响 swaves 主进程
- nginx reload 即可热更新插件

**劣势**：
- **强制 nginx 作为部署依赖**：当前 swaves 设计为独立运行（见 `cmd/swaves/worker.go` 直接 Listen），引入 nginx 改变了最小部署模型
- **插件能力受限**：njs 只能操作 HTTP 请求/响应层，无法访问 swaves 内部状态（DB、设置、任务系统）
- **调试困难**：njs 错误只出现在 nginx error.log，没有结构化上下文
- **njs JS 兼容性受限**：ES5.1 strict + 部分 ES6，无 npm，插件开发门槛高
- **SharedDict 容量有限**：仅适合轻量 KV，不适合复杂插件状态

**可行性结论**：✅ **技术上完全可行，但适用场景窄（仅网络层扩展），不适合作为通用插件系统基础**。

### 5.2 方案 B：njs + Swaves Plugin API

**优势**：
- 相对方案 A，插件可访问 swaves 数据（通过 API subrequest）
- swaves 改动最小（仅增加内部 Plugin API 端点）

**劣势**：
- 仍然强依赖 nginx
- Plugin API 设计需要仔细界定边界，避免暴露敏感数据
- subrequest 引入额外延迟（每次插件决策 = 一次 loopback HTTP 调用）
- 插件能力仍被 HTTP 请求/响应语义所限制

**可行性结论**：✅ **技术可行，但架构代价偏高（nginx 强依赖 + API 设计复杂度），适合特定场景而非通用插件系统**。

### 5.3 方案 C：Go 内嵌 JS 运行时（goja）

**优势**：
- **无额外部署依赖**：插件与 swaves 共进程，保持"单二进制"部署模型
- **插件能力最强**：可直接调用 Go 函数访问 DB、Config、Job、模板等
- **调试友好**：可在 Go 测试框架内测试插件脚本
- **热加载可行**：文件监听 + goja VM 重建即可
- njs 设计可作为 API 设计参考（request/response 对象模型）

**劣势**：
- 需要实现插件沙箱（防止插件调用危险函数、死循环等）
- goja 性能低于原生 Go，复杂插件有 CPU 开销
- 增加 swaves 本身的复杂度（插件加载、错误隔离、生命周期管理）
- 需要设计并维护插件 API（JS 侧接口契约）

**可行性结论**：✅ **技术完全可行，能力最全，是三方案中最符合 swaves 架构风格的选择**（单二进制、无外部依赖、可测试）。

---

## 6. 部署影响对比

| 维度 | 当前 Swaves | 方案 A/B (njs) | 方案 C (Go 内嵌 JS) |
|---|---|---|---|
| 最小部署 | 单二进制 + SQLite | nginx + njs + swaves 二进制 + SQLite | 单二进制 + SQLite |
| 插件分发 | — | nginx 配置 + .js 文件 | .js 文件（由 swaves 加载） |
| 插件热更新 | — | `nginx -s reload` | 文件监听自动重载 |
| 插件隔离 | — | nginx worker 进程隔离 | Go 协程 + VM panic recover |
| 插件调试 | — | nginx error.log | Go logger + 测试框架 |

---

## 7. 与 njs 的对齐点（方案 C 参考 njs 设计）

若选择方案 C，可以借鉴 njs 的以下设计：

1. **请求/响应对象模型**：暴露 `r.headersIn`、`r.headersOut`、`r.uri`、`r.method`、`r.requestBody`、`r.return()` 等接口，与 njs API 对齐，降低迁移成本。

2. **阶段钩子（Phase Hooks）**：参照 nginx 的 access/content/header_filter/body_filter 阶段，定义 swaves 的插件阶段：
   - `onRequest`：请求进入时（鉴权、路由改写）
   - `onResponse`：响应构建后（头注入、内容过滤）
   - `onPostPublish`：文章发布事件
   - `onJobRun`：任务执行事件

3. **SharedDict 等价物**：提供插件内可访问的简单 KV 存储（SQLite 表或内存 map），对应 njs 的 `SharedDict`。

4. **子请求等价物**：在 Go 层允许插件脚本调用内部 handler（不经过 HTTP 栈），对应 njs 的 `r.subrequest()`。

---

## 8. 结论与推荐

### 8.1 结论

| 方案 | 可行性 | 推荐度 | 主要原因 |
|---|---|---|---|
| A：njs 代理层 | ✅ 技术可行 | ⚠️ 低 | 破坏单二进制部署模型，插件能力受限 |
| B：njs + Plugin API | ✅ 技术可行 | ⚠️ 低-中 | nginx 强依赖 + API 设计复杂度不值当 |
| C：Go 内嵌 goja JS | ✅ 技术可行 | ✅ 高 | 无外部依赖、能力最全、符合 swaves 架构风格 |

### 8.2 推荐路径

**短期（插件需求明确前）**：不引入任何插件系统，保持当前内聚架构。

**中期（如确实有扩展需求）**：
1. 选择方案 C（Go 内嵌 goja）
2. 以 njs 的 request/response API 为参考设计 swaves 插件接口
3. 先实现最小 `onRequest` hook，覆盖自定义鉴权/限流用例
4. 渐进式扩展 `onPostPublish`、`onJobRun` 等事件 hook

**njs 本身的定位**：
- njs 作为 **nginx 反向代理层的运维脚本工具**对 swaves 仍有价值（TLS 证书处理、请求日志格式化、流量灰度等），但不是 swaves 业务逻辑插件系统的最佳选择。
- 如果 swaves 的生产部署已经前置 nginx，可以将 njs 用于 **纯运维级扩展**（非业务逻辑），业务插件继续走方案 C。

### 8.3 下一步

若确认推进方案 C：
- [ ] 引入 `github.com/dop251/goja` 并评估与 swaves 的集成点
- [ ] 定义 `PluginRequest`/`PluginResponse` Go 接口
- [ ] 实现 `onRequest` hook 在 Fiber 中间件层的执行位置
- [ ] 设计插件脚本加载路径与文件监听
- [ ] 设计沙箱限制（禁止 `os`、`net` 等危险调用）

---

## 附录：njs 官方能力速查

| njs API | 说明 | 方案 A/B 可用 |
|---|---|---|
| `r.headersIn` / `r.headersOut` | 读写请求/响应头 | ✅ |
| `r.uri` / `r.method` | 请求 URI 和方法 | ✅ |
| `r.requestBody` | 请求体（需 `js_body_filter`） | ✅ |
| `r.return(status, body)` | 直接响应 | ✅ |
| `r.internalRedirect(uri)` | 内部重定向 | ✅ |
| `r.subrequest(uri, cb)` | 子请求（调用后端接口） | ✅ |
| `ngx.shared` (SharedDict) | 跨 worker 共享 KV | ✅ |
| `ngx.log(level, msg)` | 写 nginx 日志 | ✅ |
| `r.variables` | 访问 nginx 变量 | ✅ |
| `crypto` / `fs` (njs 内置) | 加密、文件读取 | ✅ |
| Node.js npm 包 | ❌ 不支持 | ❌ |
| 持久化内存（跨请求） | ❌（仅 SharedDict） | ⚠️ 有限 |
