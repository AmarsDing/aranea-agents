# M74: 语音伴侣（贾维斯桌面语音）— 设计文档

> 版本：v1.0（2026-08-05）
> **同系列**：需求 → [`74-voice-companion.md`](./74-voice-companion.md)；开发计划 → [`74-voice-companion.development.md`](./74-voice-companion.development.md)

---

## 0. 架构决策摘要（ADR）

| # | 决策 | 结论 | 理由 / 替代方案 |
|---|------|------|----------------|
| D1 | 语音链路架构 | **级联流式管线**（流式 ASR → 现有 Chat Turn 管线 → 分句流式 TTS） | 保留平台编排内核（工具/记忆/Team/Graph/审批/流程日志）。替代：端到端 S2S Realtime API——旁路全部编排能力，与平台定位冲突，否决 |
| D2 | 语音传输通道 | **独立 WS 端点 `/v1/voice`**（二进制 PCM 帧 + JSON 控制帧） | 与 `/v1/ws` 事件总线分离，避免高频音频帧污染事件通道与限流统计；可按会话独立鉴权/限流/空闲回收。Tauri 内嵌 WS 隧道已支持二进制帧透传（`server.rs` `axum_to_tungstenite` Binary 分支），桌面端无需改动代理 |
| D3 | ASR/TTS 接入 | biz 层 `SpeechProvider` 窄端口 + data 层适配器（首接火山引擎，预留阿里/OpenAI） | 与 LLM Provider 体系同构；新增 Provider 不动网关与前端 |
| D4 | TTS 触发时机 | 后端 SentenceChunker 按句切分 LLM delta，逐句流式合成，音频 chunk 即产即推 | 首音延迟最优（首句阈值低）；前端零解析负担。替代：前端收全文再合成——延迟不可接受，否决 |
| D5 | 打开应用等桌面操作 | **客户端工具桥**：Agent 调用普通工具 → 后端经 WS 路由到桌面客户端执行 → 结果回传 Turn | Agent 在后端运行但操作必须在用户本机执行。安全沿用 tool_confirm_gate + 授权体系 |
| D6 | 桌面载体 | 复用已有 Tauri 2.x 壳（`web/src-tauri`），贾维斯 UI 为 SPA 新路由 `/companion` | 已有内嵌代理/WS 隧道/Android 路径/通知插件；零新容器成本。替代：Electron——项目已从 Electron 迁出，否决 |
| D7 | 3D 形象 | **抽象科幻 HUD**（Three.js 程序化能量核 + 粒子环 + 线框壳），`AvatarRenderer` 接口预留 VRM 插拔 | 零美术资产依赖、最贴近贾维斯气质、性能开销小。替代：VRM 人形——需模型资产与口型系统，后置 |
| D8 | 语音消息数据形态 | ASR 终稿 = 普通用户消息（metadata 标记 `input_modality=voice`）；音频留档为可选 Artifact 附件 | 语音对话记录与普通聊天天然互通，Web 端可见可回放 |
| D9 | Speech Provider 配置存储 | System Settings 新增 `speech` 分组（JSON），凭据走既有敏感字段加密 | YAGNI：首期无需多 profile 管理，不新建 Ent 表；后续如需多配置再升级为独立 Schema |
| D10 | 播报 TTS 与 63-tts 工具的关系 | 各自独立：播报管线是会话级 I/O（本模块）；63-tts 是 Agent 主动调用产出音频附件的工具 | 两者可后续共用 SpeechProvider 端口；本模块不实现 63 工具 |

---

## 1. 总体架构

```
┌─ 桌面客户端（Tauri 2.x，扩展现有 web/src-tauri）───────────────────────┐
│  SPA 路由 /companion（Vue 3 + Quasar + Pinia + Three.js）               │
│   ├ HudCanvas.vue        — Three.js 科幻 HUD（状态驱动动画）             │
│   ├ CompanionChatPanel   — HUD 旁滑出聊天窗（复用 chat 组件族）          │
│   ├ HoloConfirmCard.vue  — 全息确认卡（客户端工具确认/开启动画）          │
│   └ features/companion/voice — 采集/VAD/播放调度                         │
│  Tauri 原生层（Rust，新增 commands）                                     │
│   ├ 窗口管理（透明/无边框/置顶/迷你球形态）                               │
│   └ client_tool_executor — open_app / open_url / screenshot / ...       │
└──────────────┬──────────────────────────────────────────────────────────┘
               │ /v1/ws（事件/工具桥，已有）＋ /v1/voice（音频通道，新增）
┌──────────────┴──────────────────────────────────────────────────────────┐
│  Aranea 后端                                                             │
│  internal/server/voice_ws.go（新）    — 语音网关：会话生命周期/帧路由      │
│  internal/voice/（新包）                                                 │
│   ├ session.go              — 语音会话编排（ASR↔Chat↔TTS 胶水）           │
│   ├ session_state_machine.go— 显式状态机（AS-FSM-01）                    │
│   ├ sentence_chunker.go     — delta 分句器                               │
│   └ tts_scheduler.go        — TTS 合成调度/背压/取消                     │
│  internal/biz/speech.go（新）         — SpeechProvider 窄端口 + 配置模型   │
│  internal/data/speech/（新）          — 火山/阿里/OpenAI 适配器            │
│  internal/tools/clientbridge/（新）   — 客户端工具桥（注册/路由/超时）     │
│  ── 以下现有，零改动复用 ──                                               │
│  Chat Turn 管线 / CancelRun / tool_confirm_gate / event bus / artifact   │
└──────────────────────────────────────────────────────────────────────────┘
```

**依赖方向**（符合分层铁律）：

- `server` → `voice`（会话编排）→ `biz`（SpeechProvider 端口、ChatSender 端口）
- `biz` 不依赖 `pkg/trpc-agent-go`；SpeechProvider 端口为纯 Go 接口
- `data/speech` 实现 biz 端口（火山等 SDK 仅在 data 层）
- `tools/clientbridge` 注册为普通 ToolSet，经 `tools.Registry()` 装配，对 Agent 透明
- 客户端工具桥的下行路由复用 `event.Bus` + WS 连接管理，不新建传输

## 2. 语音通道协议（`/v1/voice`）

### 2.1 连接与鉴权

- 端点：`GET /v1/voice?session_id=<chatSessionID>`（WS Upgrade）
- 鉴权：与 `/v1/ws` 相同（JWT，复用 `SessionAuthorizer` 校验会话归属，防 IDOR）
- 单会话单语音连接：同 session 第二连接到达时旧连接收到 `voice.replaced` 后关闭
- 空闲回收：10 分钟无音频帧自动关闭 ASR 上游（连接保留），再说话时懒重连

### 2.2 上行帧（客户端 → 服务端）

| 帧 | 格式 | 说明 |
|----|------|------|
| 音频帧 | Binary：PCM s16le 16kHz mono，20ms/帧（640B） | AudioWorklet 采集重采样后直发 |
| `voice.start` | JSON `{type, sample_rate, language, dialog_mode?}` | 开启/恢复语音会话，协商参数 |
| `voice.stop` | JSON | 退出语音模式，关闭 ASR/TTS 会话 |
| `voice.commit` | JSON | 手动提交当前语句（PTT 兼容兜底） |
| `voice.barge_in` | JSON `{detect_ms}` | 前端 VAD 检测到人声（≥200ms 持续）触发打断 |
| `voice.cancel` | JSON | 显式取消当前 Turn（按钮/热键） |
| `ping` | JSON | 心跳（复用 pong 约定） |

### 2.3 下行帧（服务端 → 客户端）

| 帧 | 格式 | 说明 |
|----|------|------|
| `voice.state` | JSON `{state}` | 状态机广播：idle/listening/thinking/speaking/interrupted/error |
| `asr.partial` | JSON `{text, seq}` | 实时字幕（前端覆盖式渲染） |
| `asr.final` | JSON `{text, duration_ms}` | 终稿；随即进入 Chat 管线 |
| `turn.accepted` | JSON `{turn_id}` | 消息已被 Chat 管线接受 |
| `tts.start` | JSON `{turn_id, encoding, sample_rate}` | 播报开始；声明音频编码（首期 `pcm_f32le_16k`，备选 `mp3`） |
| 音频帧 | Binary | TTS chunk，即产即推 |
| `tts.end` | JSON `{turn_id}` | 播报结束（完整或被打断） |
| `voice.error` | JSON `{code, message, retryable}` | ASR/TTS/管线错误 |

### 2.4 序列（正常一轮）

```
client                      voice gateway                 chat pipeline
  │── voice.start ─────────→│                              │
  │── PCM frames ──────────→│── ASR session ──┐            │
  │←─ asr.partial ×N ───────│←─ partial ──────┘            │
  │   (VAD 端点/静音判停)     │←─ final                      │
  │←─ asr.final ────────────│── SendChatMessage ──────────→│
  │←─ turn.accepted ────────│←─ accepted                   │
  │←─ voice.state(thinking) │                              │── LLM delta ──┐
  │←─ tts.start + audio ────│←─ SentenceChunker → TTS ←────┘               │
  │←─ voice.state(speaking) │                              │
  │←─ tts.end / state(listening)                           │
```

### 2.5 服务可用性探测（`GET /v1/voice/status`，V2-T8）

- HTTP GET（非 WS），复用 WS 同源鉴权；响应 `{asr_available, tts_available}`（bool）
- 探测逻辑：`VoiceStatusProbe`（wire 注入）经 `SpeechConfigReader` 实时读 ASR/TTS 配置（DB-first/env-fallback，同 §3.3），`SpeechASRConfigured`/`SpeechTTSConfigured` 判定——每次调用实时读 DB，配置热生效
- 用途：前端麦克风门控——`features/companion/voiceStatus.ts` 挂载时拉取写入 companion store `voiceAvailable`（三态 null/true/false），`voiceMicDisabled` 派生；ASR 或 TTS 任一未配置即禁用麦克风按钮并提示前往「系统设置 → 语音服务」配置，避免用户点了才发现没配的坏体验

## 3. SpeechProvider 端口（biz）与适配器（data）

### 3.1 端口定义（`internal/biz/speech.go`）

```go
// Stability:evolving
type StreamingASRProvider interface {
    Open(ctx context.Context, cfg ASRSessionConfig) (ASRSession, error)
}

// Stability:evolving
type ASRSession interface {
    Write(pcm []byte) error          // 20ms PCM 帧
    Events() <-chan ASREvent         // Partial/Final/Error/VadEnd
    Close() error
}

// Stability:evolving
type StreamingTTSProvider interface {
    Open(ctx context.Context, cfg TTSSessionConfig) (TTSSession, error)
}

// Stability:evolving
type TTSSession interface {
    Write(text string, flush bool) error // 分句写入；flush=尾句强制合成
    Audio() <-chan TTSAudioChunk         // 音频 chunk + End/Error
    Close() error
}

// Stability:evolving — 配置读取（System Settings speech 分组）
type SpeechConfigReader interface {
    ASRConfig(ctx context.Context) (ASRProviderConfig, error) // driver/endpoint/credential/language
    TTSConfig(ctx context.Context) (TTSProviderConfig, error) // driver/endpoint/credential/voice/speed
}
```

- 端口方法数 ≤5（DB-N3）；凭据经既有敏感字段加密读取，禁止入日志（DB-N8）
- `ASREvent`/`TTSAudioChunk` 为 biz 层类型，data 层负责从 Provider SDK 类型单向转换

### 3.2 适配器（`internal/data/speech/`）

| 文件 | 职责 |
|------|------|
| `volcengine_asr.go` | 火山流式 ASR（WS 协议，双向流，服务端 VAD 端点检测） |
| `volcengine_tts.go` | 火山流式 TTS（WS 协议，流式文本输入/音频输出） |
| `registry.go` | driver 名 → 工厂函数注册表（`volcengine` / `aliyun` / `openai` 预留） |

- 错误翻译：适配器错误统一翻译为 `apierror`（Provider 不可用 → `CodeUnavailable`，配置缺失 → `CodeFailedPrecondition`），复用 `entErrToBizErr` 同层语义
- 重连策略：ASR 会话断线指数退避重连 ≤3 次，失败上抛 `voice.error{retryable}`；TTS 单句失败跳过该句并记 Warn（K3 降级日志），连续 3 句失败关闭会话

### 3.3 配置模型（System Settings `speech` 分组）

| key | 说明 |
|-----|------|
| `speech.asr.driver` | `volcengine`（首期唯一实现） |
| `speech.asr.endpoint` / `speech.asr.app_key` / `speech.asr.access_key` | 连接与凭据（sensitive） |
| `speech.asr.language` | 默认 `zh-CN` |
| `speech.tts.driver` / `endpoint` / 凭据 | 同上 |
| `speech.tts.voice` | 音色（火山音色 ID） |
| `speech.tts.speed_ratio` | 语速，默认 1.0 |
| `speech.archive_user_audio` | 语音留档开关，默认 `false` |

管理面 UI：System Settings 新增「语音服务」Tab（V2 期；V1 期可仅配置文件/环境变量注入）。

## 4. TTS 分句调度（`internal/voice/`）

### 4.1 SentenceChunker

- 输入：Chat delta 流（订阅该 Turn 的文本增量，来源为 event bus 的流式事件）
- 切分点：`。！？；\n` 后跟非空白字符，或累积 ≥ 80 字符硬切
- 首句优化：累积 ≥ 6 字符且遇任意标点即切（保首音延迟）；后续句最小 12 字符（合并碎句，防 TTS 抖动）
- 尾句：Turn 文本结束时 `flush=true` 强制送出残余
- 过滤：markdown 代码块/URL/表格行不参与播报（剥离后送 TTS；纯工具调用 Turn 无文本则不启动 TTS）

### 4.2 TTS 调度器

- 单 Turn 单 TTS 会话；句队列上限 8（防 LLM 快、TTS 慢时内存膨胀；满时 chunker 暂停消费 delta——背压）
- 音频 chunk 直推 WS；句间无额外间隔（NFR4 句间间隙由前端播放调度保证 <150ms）
- 取消：Turn cancel / barge_in → `ctx.Done()` → 关闭 TTS 会话 → 发 `tts.end{interrupted:true}`

## 5. 语音会话状态机（AS-FSM-01）

实体：`VoiceSession`（6 状态 > 3，必须显式状态机）
文件：`internal/voice/session_state_machine.go`

| 状态 | 含义 |
|------|------|
| `idle` | 语音模式未开启 |
| `listening` | 采集中（ASR 会话活跃） |
| `thinking` | ASR 终稿已入 Chat 管线，等 LLM 首句 |
| `speaking` | TTS 播报中 |
| `interrupted` | 播报被打断（过渡态，→ listening） |
| `error` | 错误（可恢复 → listening；不可恢复 → idle） |

转换表（`Transition(from, event) (to, error)`）：

| from \ event | voice_start | asr_final | first_tts_audio | tts_end | barge_in | turn_failed | voice_stop |
|---|---|---|---|---|---|---|---|
| idle | listening | — | — | — | — | — | — |
| listening | — | thinking | — | — | listening(忽略) | error | idle |
| thinking | — | — | speaking | listening(无文本 Turn) | listening+cancel | error | idle |
| speaking | — | — | — | listening | interrupted | error | idle |
| interrupted | — | — | — | — | — | — | idle |
| error | listening | — | — | — | — | — | idle |

> `interrupted` 为过渡态：进入即触发本地停播 + CancelRun，~300ms 红闪反馈后自动转换到 `listening`（无需事件）。

前端镜像同一状态枚举驱动 HUD 动画（§7.4），以服务端 `voice.state` 广播为准。

## 6. 客户端工具桥（`internal/tools/clientbridge/`）

> **实现状态（2026-08-08，V2-T3 ✅）**：后端已落地。锚点：`clientbridge/bridge.go`（pending 登记/30s 超时/离线降级/审计+流程日志）、`clientbridge/toolset.go`（client ToolSet：open_app/open_url）、`internal/server/ws_client_tool.go`（capabilities 注册 + invoke 路由 + result 上行）、`internal/service/client_tool_bridge.go`（AuditRecorder 适配 + wire Provider）、`internal/data/builtin_tools_seed.go`（client 分组种子，opt-in + reqConfirm）。流程日志 step：`client_tool.invoke/result/timeout`（见 52-flow-logger §5.1）。

### 6.1 工具清单

| 工具 key | 参数 | 风险 | 确认 | Phase |
|----------|------|------|------|-------|
| `client_open_app` | `{target}`（应用名/路径，白名单解析） | medium | 是（可授权免确认） | V2 |
| `client_open_url` | `{url}`（http/https 白名单协议） | low | 是 | V2 |
| `client_screenshot` | `{}` → 返回图片 Artifact | medium | 是 | V3 |
| `client_system_control` | `{action: volume_up/volume_down/mute/...}` | medium | 是 | V3 |
| `client_file_read` | `{path}`（授权目录内） | high | 是（强制） | V3 |

注册方式：`tools.Registry()` 新增 `ToolSetFactory`；种子入 `builtin_tools_seed.go`（`toolGroups` 新分组 `client`，默认 opt-in）；确认策略入 `tool_confirm_gate` catalog（requiresConfirm=true，支持 session/persisted grant——复用「始终允许」语义）。

### 6.2 执行协议（复用 `/v1/ws`）

```
Agent 调用 client_open_app
  → clientbridge executor（不本地执行）
  → 生成 invocation_id，登记 pending（30s 超时）
  → event bus → WS 下行 {type:"client_tool.invoke", invocation_id, tool, args, session_id}
       路由目标：同 user + 同 session 且 capabilities 含 desktop_companion 的连接
  → Tauri executor 执行（§8.3）
  → WS 上行 {type:"client_tool.result", invocation_id, ok, output|error}
  → pending resolve → tool result 交还 Turn
```

- **离线降级**：无可用连接时立即返回结构化错误 `DESKTOP_CLIENT_OFFLINE`（Agent 可转述「桌面客户端未连接」）
- **超时**：30s 无 result → `CLIENT_TOOL_TIMEOUT`
- **确认门时序**：确认发生在后端（tool_confirm_gate，与现有工具一致），确认卡 UI 由 `client_tool.invoke` 前的现有 confirm 事件驱动；全息确认卡是 confirm 事件在 `/companion` 路由下的渲染形态
- **审计**：invoke/result/timeout 各写一条流程日志 + 审计事件（复用 audit 域）

> **As-built（V2-T5 语音确认拦截，2026-08-08）**：
>
> - 词表匹配（`internal/voice/confirm_words.go`）：ASR 终稿归一化（小写/去空白/去首尾标点）后**整句精确匹配**；approve 词表 {好的/好/好吧/好呀/嗯/行/可以/确认/同意/批准/允许/执行/打开吧/开吧/ok/okay/yes/yeah/sure}，deny 词表 {算了/取消/拒绝/不要/别/不行/不用/不用了/先别/no/nope/cancel}。有意保守：误命中代价仅为一次 resolver 查询（无待决议确认时照常落入 Chat 管线）
> - 决议（`internal/service/voice_confirm.go`）：`VoiceConfirmResolver` 实现 `voice.ConfirmResolver` 窄端口（Stability:evolving）；在 **spirit 树 + 精确 session 两路**收集 `kind=confirm + status=tool_blocked` step（口径与前端 `useCompanionConfirms` 一致），取最早 `StartedAt` 者复用 `ConfirmActivity` 全量校验（归属/状态机/授权）与恢复路径；语音路径只发 approve/deny，不发 always
> - 拦截（`internal/voice/session.go`）：`handleASRFinal` 词表命中且 resolver 返回 resolved=true 时**不创建 Chat Turn**，停留 listening 并下行 `{type:"confirm.resolved", decision:"approve"|"deny"}` 帧；resolver 故障降级为普通语句（NFR7）
> - 流程日志：`voice.confirm.resolved` step 已登记（52-flow-logger §5.1 同步）

### 6.3 Tauri 执行器（`web/src-tauri/src/`）

| 文件（新增） | 职责 |
|--------------|------|
| `client_tools.rs` | Tauri commands：`client_open_app`（Windows `Start-Process`/白名单映射表；macOS `open -a`）、`client_open_url`、`client_screenshot`（screenshots crate → PNG → 经 artifact 上传接口回传） |
| `whitelist.rs` | 可启动目标白名单（名称→路径映射，用户可在设置中维护；默认含常见浏览器/IM） |

- 前端经 `invoke()` 调原生命令；白名单校验在 Rust 侧强制（不信任 JS 入参）
- 移动端（Android）调用返回 `UNSUPPORTED_CAPABILITY`

> **As-built（V2-T4，2026-08-08）**：
>
> - `client_open_app`：Windows 裸名走 `cmd.exe /C start ""`（App Paths/PATH 解析）、绝对路径直接 spawn（detached 无 shell 窗口）；macOS `open -a`（裸名）/`open`（路径）；Linux 直接 spawn。**未用 `Start-Process`**（避免 PowerShell 启动开销与窗口策略差异）
> - `client_open_url`：Windows 用 `rundll32.exe url.dll,FileProtocolHandler`（避免 cmd 对 query 中 `&` 的重解析）；macOS `open`；Linux `xdg-open`；URL 校验仅放行 http/https、≤2048 字符、无空白/控制字符、非空 host
> - 白名单：内置默认 ∪ 用户覆盖（config dir `whitelist.json`）；别名归一化（trim + 小写）；Windows 候选支持 `%ENV%` 展开；**裸绝对路径不作为别名接受**（路径注入防护），命中别名后按序选首个可用候选；`UnknownAlias → NOT_WHITELISTED`、`NoUsableTarget → TARGET_NOT_FOUND`
> - 结构化错误码：`NOT_WHITELISTED` / `TARGET_NOT_FOUND` / `INVALID_URL` / `UNSUPPORTED_CAPABILITY` / `SPAWN_FAILED`，经 `client_tool.result` 帧（`error` 字段 `CODE: message` 格式）回传后端桥
> - `client_screenshot` 未随 V2-T4 落地（范围裁剪，留待后续任务）
> - 前端接线（同属 V2-T4）：`services/clientTools.ts`（`isDesktopCompanion` 探测 + Tauri executor + `executeClientToolInvoke` 参数归一化 + `client_tool.result` 帧构造）、`realtime/ws-transport.ts`（`register_capabilities` 上行 + `client_tool.invoke` 分发）、`realtime/useEnvelopeStream.ts`（连接后声明 `desktop_companion` 能力；invoke 处理本地失败也即时回帧，避免挂到 30s 桥超时）

> **As-built（V2-T8 集成修复，2026-08-08）**：集成验收发现三处装配缺口并修复——
>
> - **种子补播**：存量库在 client 工具加入种子清单前已应用过 `builtin_platform_tools` 迁移，导致缺 `client_open_app`/`client_open_url` 两行；新增版本化迁移 `20261202 builtin_platform_tools_client_reseed`（`ddl_migration_registry.go`）复跑幂等种子（ON CONFLICT DO NOTHING），启动时自动补齐
> - **chat 主链路装配**：`ClientBridge` 此前仅注入 graph/team/task 路径，chat 主链路 agent 构建缺失导致 `CallableTool client_open_app not found`；修复路径为 `service.RuntimeTooling.ClientBridge` 字段 + `cmd/admin/wire.go provideRuntimeTooling` 注入 + `chat_orch_agent_build.go` 透传至 `TRPCExtensionDeps.ClientBridge`
> - **提示词引导**：spirit 提示词此前未明确客户端工具语义，agent 误用 `exec_command` 在服务器侧查找应用；`CAPABILITIES.md` 新增「客户端工具（用户本机控制）」章节（直接调用规则/禁止服务器侧探测/离线如实转述），`DECISION.md` 与 intent 分类器同步放行「打开本机应用/网址」类请求不再追问澄清

## 7. 前端设计（`web/src/`，遵循 aranea-frontend-guide 分层）

### 7.1 路由与页面

- 新路由 `/companion` → `pages/CompanionPage.vue`（桌面端 Tauri 默认入口页可配置；Web 端手动访问）
- 布局：HUD 画布（左/中心）+ 聊天面板（右，滑出抽屉）；移动端 HUD 居中 + 全屏抽屉

### 7.2 功能模块（`features/companion/`）

| 文件 | 职责 |
|------|------|
| `stores/companion.ts` | Pinia store：voiceState、实时字幕、HUD 状态、确认卡队列（单一数据源铁律） |
| `features/companion/types.ts` | 共享类型（VoiceState/VoiceError；红线 #12 展示组件经此引类型） |
| `voice/audioCapture.ts` | getUserMedia（echoCancellation+noiseSuppression）→ AudioWorklet → 重采样 16k PCM → WS 二进制帧 |
| `voice/vad.ts` | 能量 + 过零率 VAD，双重职责：①播报期人声检测（持续 ≥200ms 触发 barge_in）；②判停兜底（~700ms 静音且服务端未端点时发 `voice.commit`）。**语句端点判定以火山 ASR 服务端 VAD 为主**（尾延迟 ~300-600ms，计入 NFR1 预算），前端兜底仅在服务端失效时生效。含 `decideVadAction(evt, state)` 纯函数：VAD 事件 × 状态机镜像 → barge_in/commit 动作（V2-T1 接线） |
| `voice/audioPlayback.ts` | PCM chunk 队列 → AudioBuffer 按序调度（gapless，句间 <150ms）；barge_in 时 50ms 淡出清空 |
| `voice/useVoiceSession.ts` | `/v1/voice` 连接生命周期（createVoiceSessionClient）+ useVoiceSession composable（采集/播放/可视化桥接、状态机镜像写入 companion store、与 chat sender 的衔接） |
| `hud/hudParams.ts` | 状态 → HUD 参数纯函数（§7.4 状态驱动参数，与 Three.js 解耦以便单测） |
| `hud/HudScene.ts` | Three.js 场景（§7.4）；实现 `AvatarRenderer` 接口（预留 VRM 替换） |
| `components/companion/HudCanvas.vue` | canvas 宿主、点击/双击/拖拽交互、频谱数据桥 |
| `components/companion/CompanionChatPanel.vue` | 滑出聊天窗壳（内容由 Page 注入，复用 ChatMessagePanel 组件族） |
| `components/companion/HoloConfirmCard.vue` | 全息确认卡（confirm 事件渲染 + 粒子发射动画，V2-T5） |

新增依赖：`three`（npm）；Web Audio 全原生 API，无音频库依赖。

### 7.3 与 chat 数据流的关系

- ASR 终稿 → 复用 `useChatSender` 同一发送路径（WS `user_message` 或 HTTP，与打字一致）；语音字幕仅为 store 中的 transient 状态，不落消息流
- 流式回复渲染：复用现有 `activityV2Store` / `useChatStreamManager`；companion store 订阅同一 WS transport（`/v1/ws` 与 `/v1/voice` 双连接并存）
- 打字与语音混用：聊天窗打字发送走完全相同的管线；播报仅对语音模式开启时的 Agent 回复生效

### 7.4 HUD 场景设计（Three.js）

场景构成（`HudScene.ts`）：

| 元素 | 实现 | 状态驱动参数 |
|------|------|--------------|
| 能量核 | Icosahedron + 等离子噪声 shader（emissive） | 缩放/发光强度：idle 呼吸（sin 0.5Hz ±4%）；thinking 收缩 0.85×；speaking 随 AnalyserNode 振幅 1.0-1.15× |
| 粒子环 ×2 | Points 环形轨道（内外两圈，反向旋转） | listening 外环展开 1.2×；thinking 转速 ×3 |
| 线框壳 | Icosahedron wireframe（低透明度） | 常态缓旋 |
| 频谱环 | 128 柱环形布局（listening 态可见） | 实时 FFT 数据（采集侧 AnalyserNode） |
| 颜色 | uniform tint | idle/listening/speaking=青蓝系（#22d3ee→#34d399），thinking=琥珀（#fbbf24），interrupted 红闪（#f87171，300ms） |

- 动画循环统一 `requestAnimationFrame` + delta time；HUD 不可见（迷你态/窗口失焦）时降帧至 15fps
- 性能预算：draw call < 20，三角形 < 50k（NFR5 ≥40fps）

> **As-built（V2-T5 全息确认卡 + HUD 科幻增强，2026-08-08）**：
>
> - HUD 科幻增强（`hud/hudParams.ts` + `hud/HudScene.ts`）：`HudParams` 新增 4 个状态驱动参数——`vibrationGain`（粒子环声波震动增益，listening/speaking=1，thinking=0.25，idle=0）、`arcSpeedFactor`（全息弧线转速因子）、`coreWobble`（能量核顶点摆动幅度）、`rippleGain`（声波涟漪增益）；场景新增 Jarvis 式全息弧线组（多弧异速旋转、双色交替、呼吸透明度 + burst 提亮）、粒子环双频正弦震动（26Hz/41Hz 叠加，点尺寸随增益放大）、能量核顶点噪声摆动、声波涟漪环。纯函数参数与 Three.js 解耦，hudParams 单测覆盖
> - 全息确认卡（`components/companion/HoloConfirmCard.vue`）：confirm step（tool_blocked）渲染为悬浮全息卡（扫描线/边角括号/发光边框），含倒计时与三路径按钮——确认/取消/始终允许；批准时触发粒子流发射（乐观视觉，随后走 grant API）
> - 确认队列（`features/companion/useCompanionConfirms.ts`）：从 `activityV2Store` 派生会话内 tool_blocked confirm steps（单一数据源铁律，WS 事件自动驱动），队首激活渲染；`ConfirmCardModel` 映射工具名/参数摘要
> - 粒子发射（`features/companion/launchParticles.ts`）：`makeBurstParticles` 纯函数（可注入 RNG 便于单测）生成粒子参数，`spawnLaunchBurst` DOM 发射；批准时从确认卡向 HUD 能量核飞行并触发 HUD burst
> - 页面接线（`pages/CompanionPage.vue`）：`DECISION_REPLY` 映射 approve/deny/always → `TOOL_CONFIRM_REPLY.approve/deny/approveAlways`，调既有 `confirmActivityGrant` API（confirm 语义零新增）
> - 测试：launchParticles + useCompanionConfirms + hudParams 新增 vitest 用例全绿；确认卡三路径交互归 V2-T8 真机验收

## 8. Tauri 窗口与原生集成

| 项 | 方案 |
|----|------|
| HUD 窗口形态 | 现有主窗复用：路由即 `/companion`；透明背景（`transparent: true`）+ 无边框 + 置顶为 V3 可选项（V1 保持普通窗口，降低风险） |
| 迷你球形态 | 窗口尺寸切换（~120×120）+ HUD 简化渲染（仅能量核） |
| 全局热键 | `tauri-plugin-global-shortcut`（V3，如 Ctrl+J 唤起/语音模式） |
| 系统托盘 | 复用现有 Tauri 托盘能力（V3 评估） |
| 麦克风权限 | Windows/macOS WebView 自动弹权；Android 需在 `tauri.conf.json` 声明 `RECORD_AUDIO` 权限（V3 移动端期） |
| 通知 | 复用已注册的 `tauri-plugin-notification`（长任务完成语音+通知双通道） |

## 9. 数据模型

### 9.1 消息留档（复用现有实体，无新表）

- 用户消息：`content` = ASR 终稿；`metadata` 增加 `input_modality="voice"`、`asr_provider`、`asr_duration_ms`（JSON 元数据，非新列——若 messages 表 metadata 不支持则随 V1 实施评估最小变更）
- 语音留档（开关开启时）：Artifact（PreviewKind=audio）+ 消息附件引用（复用 `AttachmentRef` 链路）；留档失败仅 Warn 降级，不阻断消息（K3）

> **As-built（V2-T6 语音留档 + 语音溯源元数据，2026-08-08）**：
>
> - **元数据落点**：语音溯源元数据落在用户消息 `options_json`（非 messages.metadata）——`chatagent.MergeVoiceMetaIntoUserOptionsJSON` 在 `prepareTurnUserOptions` 阶段盖章 `input_modality="voice"` + `asr_provider`（ASR 配置 Driver，经 `ASRSessionConfig.Driver` 透传）+ `asr_duration_ms`（空 provider/零时长省略），既有键全保留
> - **留档链路**：`voice.Session` 在 listening 态按语句缓冲上行 PCM（上限 8 MiB ≈ 4.4 分钟 @16kHz，超限截断并 Warn 一次）；ASR 终稿 = 语句边界，PCM 经 `voice.EncodeWAV` 封装后由 `voice.AudioArchiver` 端口（`internal/service/voice_archive.go` 实现）落 Artifact——文件名 `voice-<UTC时间戳>-<μs>-<序号>.wav`（原子序号兜底低分辨率时钟，避免同 session+name 版本堆叠）
> - **开关**：`speech.archive_user_audio`（V1 读 env `SPEECH_ARCHIVE_USER_AUDIO`；V2-T7 切 System Settings 分组，端口不变）；开关关闭/读取失败/存储失败均返回零值 Ref 降级（K3），**不阻断 Turn 派发**
> - **展示态附件刻意绕开 LLM 附件链路**：留档 Ref 合并进 `options_json.attachments` 仅供 UI 回放，**不经** `Options.AttachmentIDs`——避免 `validateTurnAttachmentCapabilities` 把 audio/* 当 file 附件拒绝、及 WAV 字节注入 LLM 上下文
> - **生命周期对齐 Chat Turn**：留档在 `appctx` + userID 传播上执行（独立于 WS 连接存活）；`archiveUtterance` 同步执行于 asrPump（本地产物存储 + 单次 DB 写入，时延可忽略）
> - **确认拦截/取消/停止分支直接丢弃缓冲**，保证下一句从空缓冲开始
> - **wire 装配**：`provideVoiceWSServer` 注入既有 `*biz.ArtifactUsecase`（实现 `artifact.Saver` 窄接口）构造 `VoiceAudioArchiver`；`VoiceWSServer` 新增 `archiver` 字段，`SessionDeps.Archiver` 为 nil 时整体关闭留档（不缓冲 PCM）
> - **流程日志**：`voice.archive.saved` / `voice.archive.degraded` / `voice.archive.truncate` 三 step 已登记（52-flow-logger §5.1 同步）

### 9.2 Speech 配置（System Settings `speech` 分组，见 §3.3）

- 无新 Ent Schema；凭据字段走既有敏感字段加密（DB-N8）
- V2 期管理面 UI 落 System Settings 新 Tab

> **As-built（V2-T7 System Settings「语音服务」Tab，2026-08-08）**：
>
> - **存储落点偏差（vs D9「JSON 分组」）**：实现为 `system_settings` 单例表上 14 个离散列（DDL 迁移 `20260808_speech_columns.sql`，raw SQL 读写）——与 `planner_model_columns`/`refine_llm` 同模式（ent generator 被 tablewriter 版本冲突阻塞，列走 DDL 管理）。string 空串 = 未设置；`speech_tts_speed_ratio=0` = 未设置（proto3 零值对齐）；`speech_archive_user_audio` 为 nullable 三态（NULL = 未设置）
> - **读取语义（DB-first / env-fallback 字段级合并）**：`internal/data/speech/system_config.go` `SystemSpeechConfigReader` 实现 `biz.SpeechConfigReader`——每个字段独立取「DB 非空值 ⊻ env（`SPEECH_ASR_*`/`SPEECH_TTS_*`/`SPEECH_ARCHIVE_USER_AUDIO`）」，部分填写的 DB 行不会遮蔽其余 env 值；留档开关 NULL 回退 env，**V1 env 开关升级后不被静默覆盖**
> - **热生效**：每次 `ASRConfig/TTSConfig/ArchiveUserAudio` 调用实时读 DB（系统设置单例读取低开销），保存后新连接即生效，无需重启；读取失败回退 env 并 Warn（K3）
> - **wire 装配**：`SpeechConfigReader` 绑定由 `EnvSpeechConfigReader` 切换为 `SystemSpeechConfigReader`（构造注入 `biz.SystemSettingRepo` + Logger）
> - **API 契约**：`SystemSettings.speech` 消息（asr/tts/archive_user_audio）+ `UpdateSystemSettingsRequest` 字段 22-35；**凭据 write-only**——响应永不返回 app_key/access_key 明文，仅给 `has_api_key`/`configured` 标志；更新时空凭据 = 保留已存值（`updateXxxCred` 仅在新值非空时置位）；`speed_ratio < 0` 拒绝（BadRequest）
> - **前端**：System Settings 新增「语音服务」Tab（`SpeechServiceFields.vue` + `features/system-settings/speech.ts` 表单态/差分 patch）——已存凭据以 mask placeholder 呈现、仅输入新值才提交；`archive_user_audio` 三态经 proto3 `optional bool` 传递（未触碰 = 键缺省 = 保留已存/env）；i18n zh-CN/en-US 全量
> - **流程日志**：管理面配置读写属管理操作非业务流程，沿用 System Settings 既有惯例不新增 flow step（K1-K7 评估）

## 10. 技术选型

| 项 | 选择 | 理由 |
|----|------|------|
| ASR（首接） | 火山引擎流式语音识别（WS 双向流） | 中文效果与尾延迟（~300ms）业界领先；支持服务端 VAD 端点检测 |
| TTS（首接） | 火山引擎流式语音合成（WS 流式文本输入） | 中文音色自然、首包快（~300ms）；与 ASR 同账号体系 |
| 备选 Provider | 阿里 Paraformer/CosyVoice、OpenAI gpt-4o-transcribe/TTS | 端口预留，按注册表扩展 |
| 采集/播放 | Web Audio（AudioWorklet + AudioBuffer 调度） | 桌面/移动 WebView 全支持；PCM 直推免 decode 延迟 |
| 编码格式 | 上行 PCM s16le 16k mono；下行 PCM f32le 16k（备选 mp3） | 最低处理延迟；带宽可接受（上行 ~32KB/s） |
| 3D 渲染 | Three.js（程序化几何 + shader） | 零美术资产；与 Vue 同栈 |
| 桌面容器 | Tauri 2.x（现有） | 见 D6 |
| 打断 VAD | 前端能量/过零率 VAD + 服务端 ASR VAD 双保险 | 前端先停播（<300ms），服务端终判 |

## 11. 日志与关键节点（双轨制，K1-K7）

新增 step_id（实施时登记 `stepTitleRegistry` + 同步 52-flow-logger §5.1；domain 使用已注册 TraceDomain，若需新增 `voice` 域须在 event 包注册）：

| step_id | 节点 | 级别 |
|---------|------|------|
| `voice.session.start` / `voice.session.done` | K1 语音会话入口/出口 | Info |
| `voice.asr.final` | K6 识别终稿（含时长） | Info |
| `voice.tts.start` / `voice.tts.end` | K1 播报起止 | Info |
| `voice.barge_in` | K5 打断事件 | Info |
| `voice.provider.fallback` | K3 降级（TTS 跳句/Provider 重连/退回文字模式） | Warn |
| `voice.error` | K2 错误路径 | Error + `loggateway.Err` |
| `voice.confirm.resolved` | K5 语音确认决议（V2-T5） | Info |
| `voice.archive.saved` | K6 语音留档保存（V2-T6，含时长/大小/artifact_id） | Info |
| `voice.archive.degraded` | K3 留档降级（开关读取失败/存储失败，消息正常派发；V2-T6） | Warn |
| `voice.archive.truncate` | K3 留档截断（语句 PCM 超 8 MiB 上限；V2-T6） | Warn |
| `client_tool.invoke` / `client_tool.result` / `client_tool.timeout` | K6/K7 客户端工具生命周期 | Info/Warn |

限流（红线 4）：`asr.partial` 上行不下发流程日志；音频帧不写日志；高频事件走计数器汇总。

## 12. 与其他模块的关系

| 模块 | 关系 |
|------|------|
| [`1-chat`](./1-chat.design.md) | 执行内核；ASR 终稿复用 ChatSender/WSTurnExecutor 入口；CancelRun 复用于打断 |
| [`23-tools`](./23-tools.design.md) | 客户端工具经 Registry 装配；确认门 catalog 扩展 |
| [`27-artifact`](./27-artifact.design.md) | 语音留档/截图回传的资产底座 |
| [`34-event-system`](./34-event-system.md) | 工具桥下行路由复用 event bus + WS 连接管理 |
| [`59-multimodal-agent`](./59-multimodal-agent.design.md) | V4 音频附件 STT 复用 ASR Provider 端口；截图理解走其图片链路 |
| [`63-tts`](./63-tts.design.md) | 职责独立（见 D10）；可后续共用 SpeechProvider 端口 |
| [`72-mobile-client`](./72-mobile-client.design.md) | Android 载体与权限声明 |
| [`9-provider`](./9-provider.design.md) | 管理面模式参照（System Settings 配置/敏感字段） |
| [`18-monitor`](./18-monitor.md) | 语音流程日志入 Monitor Logs 流程 Tab |

---

*文档版本：2026-08-05 v1.0 — 立项设计定稿（经头脑风暴评审：级联流式管线 + Tauri 桌面端 + 抽象科幻 HUD + 深度系统控制 + 可插拔 Provider + 桌面&移动端）。*
