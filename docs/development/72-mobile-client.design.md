# 移动客户端（Aranea Mobile）设计文档

> 编号 72 · 状态：已批准 · 2026-07-28
> 需求见 [72-mobile-client.md](./72-mobile-client.md)

## 1. 总体架构

```
手机（Android）                     用户 VPS                    用户 PC
┌───────────────────────┐  HTTPS  ┌───────────────┐  frp 隧道  ┌──────────────────┐
│ Tauri 2 Android App   │◄═══════►│ frps + Caddy  │◄══════════►│ frpc             │
│ ├ WebView             │   WSS   │ (自动HTTPS)   │            │   ↓ 本机转发      │
│ │  └ SPA（移动适配）   │         └───────────────┘            │ Go 后端 :8000    │
│ ├ axum 同源代理        │                                          │ (绑 127.0.0.1,  │
│ │  （上游地址可配置）   │                                          │  业务零改动)     │
│ └ 前台服务保活+本地通知  │                                          └──────────────────┘
└───────────────────────┘
```

核心原则：**手机 App 是瘦客户端**。Go 后端不下移到手机；App 只认 VPS 域名，不知道 PC 真实 IP。

## 2. 手机 App 设计（web/src-tauri）

### 2.1 复用桌面壳架构

桌面端现状（[main.rs](../../web/src-tauri/src/main.rs) + [server.rs](../../web/src-tauri/src/server.rs)）：

- 绑定 `127.0.0.1:0` 随机端口，axum 内嵌服务 + rust-embed SPA，WebView 加载 loopback 地址
- axum 代理 `/healthz`、`/v1/*`、`/api/*`、`/openapi/*` 到上游（含 WebSocket 双向隧道）
- 上游地址硬编码 `http://127.0.0.1:8000` / `ws://127.0.0.1:8000`

移动端改动：**上游地址可配置**，其余全部复用（reqwest 已启用 rustls，tokio-tungstenite 已启用 `rustls-tls-webpki-roots`，wss 直连远程无需改依赖）。

### 2.2 同源代理模式（关键设计决策）

| | 桌面端 | Android 端 |
|---|--------|-----------|
| runtime-config `backendUrl` | `http://127.0.0.1:8000`（same-site 直连，端口不参与 site 计算） | **留空**（同源模式） |
| API/WS 流量 | WebView → `127.0.0.1:8000` | WebView → axum loopback → 远程 `https://<域名>` |
| Cookie | SameSite=Lax 同站有效 | 经代理同源读写，**零跨站问题** |
| 后端 CORS | 无需改动 | 无需改动 |

**为什么移动端必须同源**：远程后端是 `https://` 跨站域名，若 WebView（`http://127.0.0.1` 源）直连，SameSite=Lax cookie 不随请求携带；改成 `SameSite=None; Secure` + CORS 凭证模式会扩大 CSRF 面且要动后端。同源代理把跨站问题收敛到 Rust 层（reqwest/tungstenite 无 SameSite 概念），前端与后端均零改造。

**runtime-config 分发**：`spa_handler` 拦截 `assets/config/runtime-config.json`，按平台返回——桌面端保持现有 baked 常量；Android 端返回 `{}`（触发 [runtime.ts](../../web/src/config/runtime.ts) 同源分支）。

### 2.3 上游地址配置

- **存储**：JSON 文件存于平台应用私有目录（Android：`Context.getFilesDir()`；桌面：`%APPDATA%` 同级），通过 `tauri::path::PathResolver::app_config_dir()` 解析
- **结构**：`BackendConfig { url: String }`，如 `https://aranea.example.com`
- **运行时生效**：上游地址存 `Arc<RwLock<Option<Upstream>>>`；proxy handler 每次请求时读取——改配置即时生效，不重启 App
- **默认值**：桌面端无配置文件时回退 `http://127.0.0.1:8000`（现状不变）；Android 端无配置时代理返回 `503 + 未配置提示`，由前端配置页引导
- **配置接口（loopback only）**：
  - `GET /__local/backend-config` → `{ "url": "..." | null }`
  - `PUT /__local/backend-config` `{ "url": "https://..." }` → 校验（必须 https/wss、无路径、URL 合法）→ 落盘 → 原子更新
  - 这两个端点只挂 loopback，不会被 frp 暴露（frpc 转发的是 PC 的 Go 后端，不是手机的 axum）
- **重定向约束**：reqwest 客户端 `redirect Policy::none()`（现状），配置校验要求用户填最终 https 地址

### 2.4 首次启动流程（状态机）

```
启动 → 读配置
  ├─ 有配置 → 正常代理模式
  └─ 无配置（Android）→ SPA 加载 → 前端探测 GET /__local/backend-config 为空
       → 路由到服务器配置页 → 用户填写 → PUT 保存 → 回首页 → 登录
```

前端配置页是 SPA 内的移动端路由（`/mobile/server-setup`），非原生界面，零额外原生代码。

### 2.5 前台服务保活与通知（P2）

- Android ForegroundService（`dataSync` 类型）随 App 启动，维持 WS 长连（复用 axum 隧道 → 后端 `/v1/ws`）
- 订阅全局监控会话（`session_id=*`），过滤 Important 级事件（任务完成/失败/待确认，对应 AS-EVT-01 分级）
- 触发本地通知（`tauri-plugin-notification`），点击通知拉起 App 并深链到对应任务页
- 国产 ROM 白名单引导：部署文档说明小米/华为/OPPO 手动允许后台运行

### 2.6 Android 适配点（Rust 侧）

- `main.rs` 按 `cfg!(target_os = "android")` 分支：Android 上 Tauri 由 Activity 托管 WebView，不调用 `WebviewWindowBuilder`（窗口标题/尺寸仅桌面）
- `tauri android init` 生成 `gen/android/` 脚手架（kotlin 工程），构建产物为 APK
- 构建链变更：Android 需要 tauri CLI（当前桌面打包是裸 `cargo build`，见 [build-tauri.mjs](../../web/scripts/build-tauri.mjs)）；引入 `@tauri-apps/cli` devDependency 仅用于 `tauri android build`，桌面打包流程不变

## 3. 前端移动适配设计（web/）

**铁律**：同一代码库不 fork；共用 stores/services/api 层；仅新增视图组件；桌面端行为零回归。

### 3.1 布局切换

- 断点：`$q.screen.lt.sm`（<600px）
- 新增 `src/layouts/MobileLayout.vue`：底部 Tab 导航（会话 / 任务 / 我的）
- 路由层按断点选择 `MainLayout` / `MobileLayout`（`src/router/routes.ts` 静态双路由表 + 守卫重定向，不运行时切换）

### 3.2 页面映射

| 桌面能力 | 移动版实现 | 复用 |
|---------|-----------|------|
| 会话/聊天 | 移动版会话列表 + 聊天页（ChatComposer 紧凑模式、消息流组件） | 组件/store 全复用 |
| 任务执行流 | 纵向时间线渲染现有 activity 组件 | activityV2Store 全复用 |
| DAG 画布（GraphStageBlock） | **降级为分阶段折叠列表**（每阶段卡片含成员/状态/进度） | 数据复用，视图新写 |
| 人工确认/暂停/注入 | 现有组件触控化（最小点击区域 44px、底部操作区） | 组件复用 + 样式覆盖 |

### 3.3 服务器配置页

`/mobile/server-setup`：输入框 + 连通性测试按钮（经 `PUT /__local/backend-config` 保存后调用 `/healthz` 验证）+ 保存。i18n 双语文案。

## 4. 后端设计：登录限流（internal/server）

| 项 | 设计 |
|---|------|
| 位置 | Kratos HTTP 中间件，`selector.Match` 仅匹配 `POST /v1/admins/login` |
| 限流 | 按客户端 IP 令牌桶（`golang.org/x/time/rate`，1 次/5 秒，突发 3），桶空闲 10 分钟清理 |
| 失败锁定 | 同一 IP 连续登录失败 ≥5 次 → 锁定 15 分钟；锁定期内直接返回 `429`；成功登录清零计数 |
| 计数时机 | 中间件包装 handler：返回 401/403 视为失败；其他错误（5xx）不计 |
| IP 提取 | `X-Forwarded-For` 首跳（Caddy/frps 会设置）→ 回退 `RemoteAddr`；**注意**：Go 后端绑 127.0.0.1 + frpc 本机转发时，RemoteAddr 恒为 127.0.0.1，必须依赖 frps/Caddy 透传的 XFF（frp `proxy_protocol` 或 Caddy 反代场景），部署文档需写明 |
| 日志 | 触发限流/锁定时 `loggateway` Warn（结构化字段 IP、剩余锁定时长） |
| 测试 | 单测覆盖：正常放行、限流拒绝、失败累计、锁定、成功清零、桶清理 |

## 5. 公网接入设计（frp + VPS）

- **VPS**：`frps.toml`（bind 7000、token 认证、vhost https 由 Caddy 终结）+ `Caddyfile`（`aranea.example.com` → 反代 frps 的 https 端口，自动 Let's Encrypt）
- **PC**：`frpc.toml` 模板（server_addr/token/local 127.0.0.1:8000/remote 子域路由）
- 模板与部署步骤文档放 `build/frp/`（模板）+ 本设计文档 §5（说明），不新增根目录
- 安全基线：frps token 必填；Caddy 仅暴露 443；PC 防火墙无需放行新端口（frpc 出站连接）

## 6. Token 认证（P2，预设计）

- `POST /v1/admins/login` 响应新增 `token` 字段（长期随机 token，服务端存哈希，可吊销）
- HTTP：`Authorization: Bearer <token>` 与 cookie 并存（中间件任一通过即可）
- WS：`/v1/ws?token=...`（[runtime.ts](../../web/src/config/runtime.ts) 已支持 token query 参数）
- P0/P1 阶段同源代理下 cookie 已够用，Token 仅作 WebView cookie 持久化失效的兜底

## 7. 错误处理

| 场景 | 行为 |
|------|------|
| 未配置上游（Android） | 代理返回 503 文本提示；前端配置页引导 |
| 上游不可达 | 502（现有行为）；前端登录页显示连接失败 |
| 配置 URL 非法 | `PUT /__local/backend-config` 返回 400 + 原因 |
| 上游 http→https 301 | 不跟随（redirect none），配置校验拒绝非 https 地址 |
| WS 隧道上游断 | 关闭客户端侧 socket（现有行为），前端按现有重连逻辑处理 |

## 8. 测试策略

| 层 | 测试 |
|---|------|
| Rust | `cargo test`：配置读写、URL 校验、代理上游选择（桌面回退/Android 未配置 503） |
| Go | `go test ./internal/server/...`：限流中间件全场景 |
| 前端 | `vitest`：配置页校验逻辑、布局断点切换 |
| 手工 | AC-1~AC-6 验收清单（真实手机 + 4G 网络） |

## 9. 已知取舍与风险

- DAG 画布不做手机版（列表替代）
- 国产 ROM 杀后台无法 100% 规避（前台服务 + 文档引导缓解）
- 依赖 frps/Caddy 正确透传 XFF，否则限流粒度退化为全局（部署文档强制写明）
- Tauri 2 Android 需新增 Rust targets（`aarch64-linux-android` 等）、JDK17、Android SDK/NDK（本机当前缺失，构建 APK 前需安装）
