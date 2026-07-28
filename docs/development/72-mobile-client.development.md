# 移动客户端（Aranea Mobile）开发计划

> 编号 72 · 2026-07-28
> 需求见 [72-mobile-client.md](./72-mobile-client.md) · 设计见 [72-mobile-client.design.md](./72-mobile-client.design.md)

## 模块定位

为 Aranea-Agents 提供 Android 移动客户端：Tauri 2 Rust 壳（复用桌面端 axum 内嵌代理架构）+ 前端移动布局适配 + 后端登录限流 + frp 公网接入模板。Go 后端业务接口零改动。

## 代码锚点

| 层 | 路径 | 说明 |
|---|------|------|
| Rust 壳 | `web/src-tauri/src/main.rs` | 入口；Android/桌面平台分支 |
| Rust 壳 | `web/src-tauri/src/server.rs` | axum 代理；上游地址可配置改造 |
| Rust 壳 | `web/src-tauri/src/config.rs`（新增） | BackendConfig 读写与校验 |
| Rust 壳 | `web/src-tauri/Cargo.toml` | 新增 platform 依赖（P2: 通知插件） |
| 前端 | `web/src/layouts/MobileLayout.vue`（新增） | 移动底部 Tab 布局 |
| 前端 | `web/src/pages/mobile/`（新增） | ServerSetup 等移动页面 |
| 前端 | `web/src/router/routes.ts` | 移动路由表 + 断点守卫 |
| 前端 | `web/src/config/runtime.ts` | 同源模式已支持，无需改动 |
| 后端 | `internal/server/http.go` | 登录限流中间件挂载 |
| 后端 | `internal/server/login_ratelimit.go`（新增） | IP 限流 + 失败锁定 |
| 部署 | `build/frp/`（新增） | frps/frpc/Caddyfile 模板 + README |
| 打包 | `web/scripts/build-tauri.mjs` | 桌面流程不变；Android 走 tauri CLI |

## 现状评估

- Rust 壳已有完整的同源代理 + WS 隧道 + rust-embed SPA，上游地址硬编码 `127.0.0.1:8000`
- 前端 `runtime.ts` 已支持 `backendUrl` 留空走同源，移动适配断点/移动页面缺失
- 后端登录接口 `POST /v1/admins/login`（`internal/service/admin.go`）无限流保护
- 本机无 Android 构建环境（JDK/SDK/NDK 缺失），APK 编译需先装环境

## Phase 划分与任务清单

### P0：链路打通 + 安全基线（本期）

| # | 任务 | 状态 |
|---|------|------|
| 0.1 | Rust：`config.rs` BackendConfig 读写 + URL 校验 + 单测 | ✅ |
| 0.2 | Rust：`server.rs` 上游可配置（RwLock 运行时生效）、`/__local/backend-config` GET/PUT、runtime-config 平台分发、单测 | ✅ |
| 0.3 | Rust：`main.rs` Android 平台分支（不创建桌面窗口） | ✅ |
| 0.4 | 后端：登录限流中间件（TDD，单测全覆盖）+ 挂载 http.go | ✅ |
| 0.5 | 前端：`/mobile/server-setup` 配置页（i18n + 连通性测试 + vitest） | ✅ |
| 0.6 | 前端：`MobileLayout` 骨架 + 断点路由守卫（仅框架，页面 P1 填充） | ✅ |
| 0.7 | 部署：`build/frp/` 模板（frps.toml / frpc.toml / Caddyfile / README） | ✅ |
| 0.8 | `tauri android init` 脚手架（需先装 tauri CLI；无 SDK 不编译） | ⏸ 环境阻塞：本机无 JDK，`tauri android init` 报 "Java not found"。已装 @tauri-apps/cli@2.11.4 devDependency + .gitignore 预排除 `/gen`；装好 JDK 17 后执行 `pnpm tauri android init` 即可 |
| 0.9 | 验证：`cargo test` + `go build ./...` + `go test ./internal/server/...` + `pnpm lint/test/build` | ✅（cargo 11/11；go build exit 0，test/install-probe 多 main 为历史遗留与本改动无关；server 包测试 ok；vitest 990/990；lint 0 errors；quasar build 成功） |

### P1：移动端 UI 完整适配

| # | 任务 | 状态 |
|---|------|------|
| 1.1 | 移动版会话列表页 + 聊天页（复用消息流/ChatComposer 紧凑模式） | ✅（1.1a bindings composable `useChatMessagePanelBindings` + 19 单测；1.1b MobileSessionsPage；1.1c MobileChatPage + mobile-chat 路由 + workspace provide/inject；lint/test/build 全过） |
| 1.2 | 移动版任务列表页 + 执行流纵向时间线 | ✅（MobileTasksPage 复用桌面 TaskList 纵向执行流；provide CHAT_SCROLL_EL_KEY 保留懒水合 dwell；视图状态纯函数 `resolveMobileTasksView` + 4 单测；mobile-tasks 路由切换；lint/test/build 全过，vitest 1013/1013） |
| 1.3 | 团队阶段分阶段折叠列表（GraphStageBlock 移动降级视图） | ✅（新增 `GraphStageList.vue` + 纯函数 `graphStageListUi.ts` 11 单测：deriveGraphStageStatus/orderGraphNodesForList/defaultGraphNodeExpanded；TaskCard 按 `$q.screen.lt.sm` 切换 DAG 画布/折叠列表；成员行 → MemberSessionDialog 复用；vitest 1024/1024、lint 0、build 0） |
| 1.4 | 确认卡片/暂停/注入触控化适配 | ✅（MemberSessionDialog 移动端 `maximized`（`$q.screen.lt.sm` + safe wrapper 同 TaskCard）+ 注入输入栏固定底部操作区（flat 模式 flex 列 + 活动流内部滚动 + `env(safe-area-inset-bottom)`）；ConfirmBlock 按钮全宽纵向堆叠 min-height 44px；ClarifyBlock 标题/选项/输入/导航按钮 ≥44px；MemberSessionPanel 输入框/暂停/注入按钮 ≥44px；全部经 `@media (max-width: 599px)` 门控桌面零改动；MemberSessionDialog spec +2 单测（移动/桌面 maximized）；vitest 1026/1026、lint 0、build 0；浏览器 375px 冒烟通过） |
| 1.5 | 手工验收 AC-1/2/3（真实手机 4G） | 📋 |

### P2：保活通知 + Token 认证

| # | 任务 | 状态 |
|---|------|------|
| 2.1 | Android ForegroundService 保活 WS（Kotlin 侧） | 📋 |
| 2.2 | tauri-plugin-notification 本地通知 + 点击深链跳转 | 📋 |
| 2.3 | 后端 Token 认证（login 响应 token + Bearer + WS token query） | 📋 |
| 2.4 | 手工验收 AC-4/5 | 📋 |

### P3：体验优化

| # | 任务 | 状态 |
|---|------|------|
| 3.1 | 桌面端配对二维码 + 手机扫码配置 | 📋 |
| 3.2 | 离线缓存 / 弱网重连优化 | 📋 |

## 验收标准

见需求文档 AC-1~AC-6。P0 出口标准：0.9 全部验证命令通过 + 桌面端无回归 + 设计文档 §2.2 同源代理模式经 loopback 手工验证。

## 环境前置（阻塞 APK 编译，不阻塞开发）

1. JDK 17（如 Microsoft OpenJDK）
2. Android SDK cmdline-tools + platform-34 + NDK + build-tools
3. Rust targets：`aarch64-linux-android`、`armv7-linux-androideabi`、`i686-linux-android`、`x86_64-linux-android`
4. tauri CLI v2（`cargo install tauri-cli --version "^2"` 或 `pnpm add -D @tauri-apps/cli`）

## 改动文件清单（P0）

- 新增：`web/src-tauri/src/config.rs`、`internal/server/login_ratelimit.go`、`internal/server/login_ratelimit_test.go`、`build/frp/*`、`web/src/pages/mobile/ServerSetupPage.vue`、`web/src/layouts/MobileLayout.vue`
- 修改：`web/src-tauri/src/main.rs`、`web/src-tauri/src/server.rs`、`web/src-tauri/Cargo.toml`、`internal/server/http.go`、`web/src/router/routes.ts`、i18n 语言包
- 文档：`docs/development/72-mobile-client.*`（本三件套）
