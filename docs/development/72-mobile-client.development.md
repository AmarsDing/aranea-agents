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
| 前端 | `web/src/layouts/MobileLayout.vue`（新增） | 移动底部 Tab 布局；P2 挂载通知 watcher + onAction 深链 |
| 前端 | `web/src/pages/mobile/`（新增） | ServerSetup 等移动页面 |
| 前端 | `web/src/router/routes.ts` | 移动路由表 + 断点守卫 |
| 前端 | `web/src/config/runtime.ts` | P2：`buildWsUrl` 跨源默认携带存储 token（显式参数优先） |
| 前端 | `web/src/services/authToken.ts`（新增，P2） | localStorage 登录 JWT 存取 + Bearer 头构造 |
| 前端 | `web/src/services/localNotification.ts`（新增，P2） | tauri-plugin-notification 封装（懒加载、非 Tauri no-op、深链 onAction） |
| 前端 | `web/src/features/chat/blockingStepNotification.ts`（新增，P2） | 阻塞步骤→通知描述纯函数（confirm/clarify 过滤、去重、截断、深链路由） |
| 前端 | `web/src/features/chat/composables/useBlockingStepNotifications.ts`（新增，P2） | activity store 阻塞步骤 watcher（仅 document.hidden 通知） |
| 前端 | `web/src/features/mobile/pairingQr.ts`（新增，P3） | 配对二维码 payload 构造/解析纯函数（JSON 信封 + 裸 URL 兼容） |
| 前端 | `web/src/components/mobile/PairingQrDialog.vue`（新增，P3） | 桌面端配对二维码弹窗（MainLayout 菜单入口，qrcode dataURL） |
| 前端 | `web/src/services/qrScanner.ts`（新增，P3） | tauri-plugin-barcode-scanner 封装（懒加载、非 Tauri unavailable、权限分级） |
| 前端 | `web/src/features/mobile/offlineCache.ts`（新增，P3） | 会话列表 localStorage 缓存（按 agent 隔离、50 上限、损坏降级） |
| 前端 | `web/src/features/mobile/useNetworkStatus.ts`（新增，P3） | navigator.onLine + online/offline 事件响应式封装 |
| 前端 | `web/src/features/mobile/useOfflineSessionList.ts`（新增，P3） | 会话列表离线回退门控（live 空 &&（离线‖loadError）→ 缓存） |
| 前端 | `web/src/realtime/ws-transport.ts`（P3） | 僵尸连接检测（WS_ZOMBIE_TIMEOUT_MS 无下行强关）+ reconnectNow + online 事件接线 |
| 后端 | `internal/server/http.go` | 登录限流中间件挂载 |
| 后端 | `internal/server/login_ratelimit.go`（新增） | IP 限流 + 失败锁定 |
| 后端 | `api/kratos/admin/v1/admin.proto`（P2） | `Admin.token=11`（仅 Login 响应填充） |
| 后端 | `pkg/auth/middleware.go`（P2） | 新增 `IssueTokenForWorkspace`/`SetCookieWithToken`；中间件 cookie→Bearer→query 原有支持 |
| 后端 | `internal/service/admin.go`（P2） | Login 签一次 JWT 同写 cookie 与响应体 |
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
| 2.1 | Android ForegroundService 保活 WS（Kotlin 侧） | ⏸ 环境阻塞：同 0.8（无 JDK） |
| 2.2 | tauri-plugin-notification 本地通知 + 点击深链跳转 | ✅ 代码集成完成（运行验证依赖 Android 环境）：Rust 注册插件 + capabilities `notification:default`；`services/localNotification.ts` Tauri 门控封装（懒加载、非 Tauri no-op、onAction 深链路由）；纯函数 `features/chat/blockingStepNotification.ts`（6 单测：confirm/clarify 过滤、去重、截断、深链路由）；`useBlockingStepNotifications` watcher 挂 MobileLayout（仅 document.hidden 时通知）；cargo check 通过；vitest 1040/1040；lint 仅剩 skill 模块历史新增违规（见下） |
| 2.3 | 后端 Token 认证（login 响应 token + Bearer + WS token query） | ✅（TDD）：proto `Admin.token=11` + make api 重生成；`pkg/auth` 新增 `IssueTokenForWorkspace`/`SetCookieWithToken`（round-trip 单测）；Login 签一次 JWT 同写 cookie 与响应体；前端 `services/authToken.ts`（localStorage，4 单测）+ axios 请求拦截器附 Bearer + `buildWsUrl` 跨源默认带存储 token（显式参数优先，4 单测）+ login 存储/logout 清除；中间件 cookie→Bearer→query 优先级原本已支持零改动；go build 绿；vitest 1040/1040 |
| 2.4 | 手工验收 AC-4/5 | 📋（依赖真机 + frp 公网环境） |

> lint 备注：`components/skills/SkillFilterBar.vue`（+4）与 `SkillTable.vue`（+5）存在新增硬编码中文，源自 skill 管理标签/排序改动（另一任务线），未随本任务修复。

### P3：体验优化

| # | 任务 | 状态 |
|---|------|------|
| 3.1 | 桌面端配对二维码 + 手机扫码配置 | ✅（`features/mobile/pairingQr.ts` payload 纯函数 + 7 单测；`components/mobile/PairingQrDialog.vue` 桌面二维码弹窗（MainLayout 菜单入口）；`services/qrScanner.ts` 扫码封装（非 Tauri 降级 unavailable）+ ServerSetupPage 扫码按钮；Rust 侧 `#[cfg(mobile)]` 条件注册 barcode-scanner 插件；i18n 双语 key 补齐） |
| 3.2 | 离线缓存 / 弱网重连优化 | ✅（a. ws-transport 僵尸连接检测：55s 无下行帧以 4000 强关；b. `reconnectNow()` + window online 事件即时重连（跳过退避），`ws-transport-weaknet.spec.ts` 7 单测；c. `offlineCache.ts` localStorage 缓存（按 agent 隔离、50 上限、损坏降级）+ `useNetworkStatus` + `useOfflineSessionList`（live 空 &&（离线‖loadError）回退缓存）+ MobileSessionsPage 接线 + MobileLayout 全局离线横幅，17 单测；vitest 1462/1462、lint 0 error、build 0、check:i18n 3767=baseline） |

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

## 改动文件清单（P2）

- 新增：`web/src/services/authToken.ts`（+spec）、`web/src/services/localNotification.ts`、`web/src/features/chat/blockingStepNotification.ts`、`web/src/features/chat/composables/useBlockingStepNotifications.ts`（+spec）、`web/src-tauri/capabilities/default.json`
- 修改：`api/kratos/admin/v1/admin.proto`（`Admin.token=11`）+ 生成物、`pkg/auth/middleware.go`（Issue/SetCookieWithToken）、`internal/service/admin.go`（Login 返回 token）、`web/src/config/runtime.ts`（buildWsUrl token fallback）、`web/src/services/axiosHandler.ts`（Bearer 拦截器）、`web/src/features/admin/types.ts`/`api.ts`（AdminSession.token）、`web/src/stores/auth.ts`（login 存储/logout 清除）、`web/src/layouts/MobileLayout.vue`（通知挂载）、`web/src-tauri/Cargo.toml`+`lib.rs`（通知插件）、i18n 语言包（mobile.notify\*）
- 测试：`pkg/auth/cookie_test.go`（+2）、`web/src/config/__tests__/runtime.spec.ts`（新增 4）

## 改动文件清单（P3）

- 新增：`web/src/features/mobile/pairingQr.ts`（+spec）、`web/src/components/mobile/PairingQrDialog.vue`、`web/src/services/qrScanner.ts`、`web/src/features/mobile/offlineCache.ts`（+spec）、`web/src/features/mobile/useNetworkStatus.ts`（+spec）、`web/src/features/mobile/useOfflineSessionList.ts`（+spec）、`web/src/realtime/__tests__/ws-transport-weaknet.spec.ts`
- 修改：`web/src/realtime/ws-transport.ts`（僵尸检测 + reconnectNow + online 接线）、`web/src/features/constants/timeouts.ts`（WS_ZOMBIE_TIMEOUT_MS）、`web/src/pages/mobile/ServerSetupPage.vue`（扫码按钮）、`web/src/pages/mobile/MobileSessionsPage.vue`（离线缓存接线）、`web/src/layouts/MainLayout.vue`（配对菜单入口）、`web/src/layouts/MobileLayout.vue`（离线横幅）、`web/src-tauri/Cargo.toml` + `lib.rs`（barcode-scanner 插件 `#[cfg(mobile)]` 注册）、i18n 语言包（mobile.pairing\*、mobile.offlineCached）
- 文档：`docs/development/72-mobile-client.*`（本三件套）
