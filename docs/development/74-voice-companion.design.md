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
| D11 | HUD 科幻重构方向（V5） | **方舟反应堆 · 混合式**（**已替代**，见 D12）：液态 simplex 能量核 + 同心刻度仪表环 + UnrealBloomPass 辉光 + DOM 全息化 + 程序合成音效 | 初版「能量核+粒子环+线框壳」视觉不达标（无真辉光、缺 Jarvis 刻度仪表、控件未 HUD 化、无音效）。替代：纯液态 Orb（缺精密机械感，不 Jarvis）/ 全景 Stark 工作台（面板无真实数据、噪音大），均否决 |
| D12 | HUD 视觉方向再修订（V7） | **完全复刻 TwinSprite SpriteOrb**：value noise 顶点置换光球 + 单粒子轨道环 + CSS 光晕 + 网格/扫描线舞台；**移除 Bloom 后处理与反应堆部件族**（替代 D11 的 3D 场景部分；DOM 全息化与音效引擎沿用） | 用户指定以既有 TwinSprite 项目（`F:\TwinSprite`）视觉为准——该视觉已经真机验证达标、风格简洁；V5 反应堆 6 部件 + Bloom 复杂度冗余且与目标风格不符。保留本产品增量：boot 点亮过场、speaking 相机微抖、burst 脉冲、打断红闪、thinking 无变色约束 |

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
│   ├ session_wake.go         — V10 唤醒/休眠（dormant/退出词/静默计时）     │
│   ├ session_state_machine.go— 显式状态机（AS-FSM-01）                    │
│   ├ wake_words.go           — 唤醒词剥离/退出词匹配（同音词表）            │
│   ├ sentence_chunker.go     — delta 分句器                               │
│   └ tts_scheduler.go        — TTS 合成调度/背压/取消/休眠哨兵            │
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
| `voice.start` | JSON `{type, sample_rate, language, dialog_mode?, agent_key?, team_id?, mode?}` | 开启/恢复语音会话，协商参数；`mode` 空 = 对话（默认），`dictation` = 听写（§13） |
| `voice.stop` | JSON | 退出语音模式，关闭 ASR/TTS 会话 |
| `voice.commit` | JSON | 手动提交当前语句（PTT 兼容兜底） |
| `voice.barge_in` | JSON `{detect_ms}` | 前端 VAD 检测到人声（≥200ms 持续）触发打断 |
| `voice.cancel` | JSON | 显式取消当前 Turn（按钮/热键） |
| `voice.wake` | JSON `{source}` | 唤醒（V10，§16）：`source ∈ kws/manual/system`；dormant 态受理进 listening |
| `ping` | JSON | 心跳（复用 pong 约定） |

### 2.3 下行帧（服务端 → 客户端）

| 帧 | 格式 | 说明 |
|----|------|------|
| `voice.state` | JSON `{state}` | 状态机广播：idle/dormant(V10)/listening/thinking/speaking/interrupted/error |
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

> **As-built（V5.2 Turn 派发 busy 重试，2026-08-09）**：ASR 终稿派发 Chat Turn 撞
> `CHAT_TURN_BUSY`（准入 TOCTOU 竞态：前一轮 Turn 尚在准入期、锁内复查发现活跃运行）
> 时不再整轮失败——`Session.executeTurnWithBusyRetry` 短退避重试（3 次 × 250ms，对齐
> channel 入口 `runNativeTurnWithBusyRetry`），重试后准入重查、`AllowQueue=true` 时消息
> 转入排队队列等待执行，回复 delta 经事件总线正常进 TTS；重试耗尽才走
> `handleTurnFailure`。首次重试记 `voice.turn.busy_retry` 进程日志（K4，非每次）。

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
- **SAUC 协议契约（As-built 2026-08-09）**：`full_server_response` 的 `result.utterances` 为**连接级累积列表**——已 `definite` 的语句会持续出现在后续每帧响应中；适配器必须按 definite 游标去重（`volcASRSession.finalCursor`），保证一条定稿语句只发射一次 `ASREventFinal`（否则听写模式每个重复 Final 追加进输入框 → 无限输入事故）。definite 总数回退视为服务端开启新累积窗口，游标归零。产出新终稿的帧跳过 Partial（`result.text` 与终稿同源，避免字幕回闪）

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

> **As-built（V5.2 纯标点句过滤，2026-08-09）**：`cut`/`hardCut` 送出前经
> `speakable()` 判定——不含任何可朗读字符（unicode 字母/数字，覆盖 CJK）的句子
> （纯标点/纯符号）不下发 TTS。此前此类句子被火山拒绝（`45002001 No readable text!`）
> 并计入连续失败（调度器连续 3 次中止，该 Turn 后续句全部无声）。flush 语义不受影响：
> 尾句被丢弃时由会话层补空文本 flush 哨兵驱动 `OnDrained`（tts.end 不缺失）。
>
> **As-built（V5.3 播报文本清洗扩展，2026-08-09）**：真机发现 LLM 回复的 markdown
> 加粗 `**文本**` 被火山 TTS 逐字读作「星星」。`cleanForSpeech` 在原有
> 代码块/URL/图片/表格/链接剥离基础上，增剥：成对强调符（`**`/`__`/`~~`/`*`/`_`，
> 保留内文；`*`/`_` 单字符规则带词边界守卫，不误伤 `3*4`、`snake_case`）、行首
> `#` 标题、`-`/`*`/`+` 及 `1.`/`3、` 列表符、`---` 分隔线、`>` 引用符、emoji
> （显式枚举 Unicode 区块 1F000–1FAFF/2600–26FF/2700–27BF/2B00–2BFF + FE0F/200D/20E3，
> 保留 `°`/`℃`/`×` 等 So 类常用符号）；流式切句拆散的残余成串标记（`\*{2,}`/`~{2,}`）
> 一并剥除。

### 4.2 TTS 调度器

- 单 Turn 单 TTS 会话；句队列上限 8（防 LLM 快、TTS 慢时内存膨胀；满时 chunker 暂停消费 delta——背压）
- 音频 chunk 直推 WS；句间无额外间隔（NFR4 句间间隙由前端播放调度保证 <150ms）
- 取消：Turn cancel / barge_in → `ctx.Done()` → 关闭 TTS 会话 → 发 `tts.end{interrupted:true}`

> **As-built（V5.3 句子级空闲超时，2026-08-09）**：`synthesize` 增加
> `ttsSentenceTimeout`（15s，包级变量便于测试缩短）空闲计时——任一 chunk
> （Data/End/Error）到达即重置；超时放弃本句返回 `errTTSSentenceTimeout`，worker
> 继续后续句，`OnDrained` 必达。此前挂死句（火山 End 帧永不到达/连接半死）会把
> worker 饿死在单句上，该 Turn 后续句与 `tts.end` 全部阻塞。

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

> **As-built（V5.3 澄清问题口播，2026-08-09）**：会话订阅 chat 侧
> `StepCreatedEvent kind=clarify`（澄清门触发）→ `maybeBroadcastClarification` 解析
> `ClarificationEnvelope`，经 `clarificationSpeech` 渲染口播文本（引导语 + 逐题
> 题干/可选项 + 作答引导，中文序号「第 N 个」「两」）喂 SentenceChunker 走既有 TTS。
> 仅 listening/thinking/speaking 态播报；随后的 TurnCompleted 走既有 flush/drain 收尾
> 回 listening；用户语音作答经自由文本澄清路径续跑（与 WS 文字消息同入口
> `Execute`），语音侧无特判。流程日志 step：`voice.clarify.broadcast`。

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

- 新路由 `/companion` → `pages/CompanionPage.vue`（桌面端 Tauri 默认入口页可配置；Web 端经主导航侧栏「工作台 → 语音伴侣」进入，2026-08-12 起）
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
| `hud/HudScene.ts` | Three.js 场景组合器（§7.4）；实现 `AvatarRenderer` 接口（预留 VRM 替换）；视觉拆分为 `hud/parts/*` 模块化场景部件（V7 起为 TwinSprite 光球两部件） |
| `hud/parts/`（V7 重建） | SpriteOrbCore（TwinSprite value noise 光球）/ OrbitRing（260 粒子倾斜轨道环），统一 `HudPart.update(dt, timeS, params, audio)` 接口；V5 反应堆部件族（ReactorCore/ReactorRings/Starfield/SpectrumRing/ShockwavePool/EnergyParticles）已随 D12 移除 |
| `audio/uiSounds.ts`（V5 新增） | 科幻音效引擎：Web Audio 程序合成（boot/chirp/ping/ding/buzz/cut），零音频资产，localStorage 开关 |
| `components/companion/HudCanvas.vue` | canvas 宿主、点击/双击/拖拽交互、频谱数据桥 |
| `components/companion/CompanionChatPanel.vue` | 滑出聊天窗壳（内容由 Page 注入，复用 ChatMessagePanel 组件族） |
| `components/companion/HoloConfirmCard.vue` | 全息确认卡（confirm 事件渲染 + 粒子发射动画，V2-T5） |

新增依赖：`three`（npm）；Web Audio 全原生 API，无音频库依赖。

### 7.3 与 chat 数据流的关系

- ASR 终稿 → 复用 `useChatSender` 同一发送路径（WS `user_message` 或 HTTP，与打字一致）；语音字幕仅为 store 中的 transient 状态，不落消息流
- 流式回复渲染：复用现有 `activityV2Store` / `useChatStreamManager`；companion store 订阅同一 WS transport（`/v1/ws` 与 `/v1/voice` 双连接并存）
- 打字与语音混用：聊天窗打字发送走完全相同的管线；播报仅对语音模式开启时的 Agent 回复生效

### 7.4 HUD 场景设计（Three.js）

> **v3 修订（2026-08-09，V7 TwinSprite 光球复刻，ADR-D12）**：视觉方向由用户指定回归既有 TwinSprite 项目（`F:\TwinSprite` 的 SpriteOrb/SpriteOverlay）——shader、几何、配色、粒子环、相机、电平驱动公式全部原值移植；移除 V5 反应堆部件族与 Bloom 后处理。V5/V2 反应堆设计存档见文末。
>
> ~~v2 修订（V5 反应堆科幻重构，ADR-D11）~~：已被 D12 替代，存档见文末。

#### 渲染管线（v3）

- **无后处理**：TwinSprite 原配方为加法混合（AdditiveBlending）+ CSS 呼吸光晕即成辉光——`WebGLRenderer(alpha, antialias)` 直渲，V5 的 `EffectComposer/UnrealBloomPass` 全部移除
- 相机 TwinSprite 原值：fov 45、position z 5.2；`setPixelRatio(min(devicePixelRatio, 2))`
- HUD 不可见（`document.hidden`）降帧 15fps 保留；性能预算 NFR5 ≥40fps（无 Bloom 后余量更大）

#### 场景构成（v3，`hud/parts/` 两部件）

`HudScene.ts` 为组合器（renderer + 相机 + 帧循环 + 状态参数/音频快照分发），部件统一 `HudPart` 接口（`update(dt, timeS, params, audio)` + `setTint(a, b)` + `dispose()`），帧间零分配：

| 部件 | 文件 | TwinSprite 原值 | 状态驱动 |
|------|------|----------------|----------|
| 光球核心 | `parts/SpriteOrbCore.ts` | Icosahedron(1.15, 48) + 3D value noise 双层顶点置换 + Fresnel 边缘光；`uAmp = 0.12 + level×0.38`；配色 `#123a6e→#4dd8e8`，加法混合 | 电平驱动振幅/发光；thinking 噪声流速 ×3 + 收缩 0.85×；`uIntensity` 调光（本产品扩展：boot 过场/待机微光） |
| 轨道粒子环 | `parts/OrbitRing.ts` | 260 粒子，半径 1.55~1.80、y 抖动 ±0.06，倾斜 π×0.42；size 0.035、opacity 0.85；转速 `0.002+level×0.02` rad/帧、xz 缩放 `1+level×0.12` | 电平驱动转速/缩放；thinking 转速 ×3；opacity ×intensity |

#### 舞台背景与光晕（v3，HudCanvas.vue 作用域 CSS）

- **网格**：accent 6% 双线 48px，radial mask（中心 30% → 75% 渐隐）——TwinSprite SpriteOverlay 原值
- **扫描线**：2px 横向渐变（accent 45%），7s 垂直扫动，opacity 0.5——原值
- **CSS 光晕**：radial-gradient accent 22% → transparent 65%，3.2s 呼吸（scale 0.96↔1.05）；语音模式开启时 opacity 0.55↔1（TwinSprite 原值），待机降为 0.25↔0.55

#### 状态映射（v3）

| 状态 | 光球/环表现 |
|------|------------|
| idle（待机） | intensity 0.35 微光，uAmp 0.12 呼吸，环慢转 |
| listening | 麦克风电平（中低频 bins 2..48 均值归一化 ×1.6，TwinSprite `useAudioLevel.sampleMic` 公式）驱动 uAmp/环速/环缩放 |
| thinking | 噪声流速 ×3 + 环转速 ×3 + 核心收缩 0.85×（**无颜色变化**，V5.1 约束沿用） |
| speaking | 播放侧振幅驱动强震（uAmp → ~0.5）+ 相机高频微抖（`shakeGain`，V5.1 沿用）+ 入场 burst 电平脉冲 |
| interrupted/error | 红系 `#4c0519→#f87171`（打断 300ms 红闪提亮沿用） |

- **boot 点亮过场（本产品增量）**：语音模式开启 ~1.2s 内 intensity 0.35→1；关闭立即回待机微光
- **电平平滑**：TwinSprite 0.2/帧（60fps）→ dt 归一化 ×12/s（`HudScene` 侧）

#### DOM 全息化（v2 沿用，V7 未变）

| 控件 | 全息化设计 |
|------|-----------|
| 状态指示 | 角括号 + 扫描线动画 + 大写等宽字母 + 发光描边的 HUD 标签 |
| 实时字幕 | 打字机逐字浮现 + 全息边框 |
| 麦克风按钮 | 反应堆式圆形按钮：脉动光环，语音模式开启时光环旋转 |
| HoloConfirmCard | 微调对齐新视觉语言（描边/发光色值统一） |

#### 科幻音效引擎（v2 沿用，V7 未变，`features/companion/audio/uiSounds.ts`）

- **Web Audio 程序合成，零音频资产**：上电扫频（boot sweep）/ 唤醒 chirp / 思考声纳 ping（循环，间隔 ~2s）/ 确认 ding / 拒绝 buzz / 打断切音
- 音量压低（约 -18dB 混音级）；`localStorage` 开关可关（默认开）
- 合成参数（波形/频率包络/时长）为纯函数可单测；播放调度注入 `AudioContext` 接口便于 mock
- 触发点与状态机联动：voice mode on → boot；listening → chirp；thinking → ping 循环；confirm approve → ding；deny → buzz；barge_in → cut

#### 架构约束（v3）

- `AvatarRenderer` 接口不变（VRM 预留）；`hudParams` 纯函数锁定 TwinSprite 原值（uAmp 常量 0.12/0.38、环速公式、配色 `#123a6e/#4dd8e8`）+ 本产品增量（`noiseSpeedFactor`/`orbScale`/`intensity`/`shakeGain`），单测同步锁定
- 动画循环统一 `requestAnimationFrame` + delta time；部件帧间零分配；单文件 ≤500 行红线

> **As-built（V7 TwinSprite 复刻，2026-08-09）**：
>
> - **移植保真**：`SpriteOrbCore` VERT/FRAG shader 与 TwinSprite `SpriteOrb.vue` 逐行一致（唯一扩展 `uIntensity` 乘算最终颜色，加法混合下等效调光，供 boot/待机微光）；`OrbitRing` = TwinSprite `buildRing()` 原值（260 粒子/半径 1.55~1.80/y ±0.06/倾斜 π×0.42/size 0.035/opacity 0.85）；相机 fov 45 z 5.2；电平平滑 0.2/帧 → dt×12/s；环转速/缩放公式按 dt×60/s 归一化
> - **删除清单**：`ReactorCore/ReactorRings/Starfield/SpectrumRing/ShockwavePool/EnergyParticles` 六部件文件与 Bloom 后处理（`BloomComposer`）全部移除；`hudParams` 反应堆参数族（bloomIntensity/ringExpand/ringBoot/particleGain/rippleGain 等）同步删除，V5 场景归档不再保留
> - **本产品增量保留**：boot 点亮过场（1.2s intensity 0.35→1，`setVoiceMode(false)` 立即回待机）；speaking 相机高频微抖（`shakeGain`，双正弦 47.3/31.7/41.9/37.1Hz × level×0.045）；进入 speaking 自动 burst（0.6s 电平冲高 0.8 衰减）；打断 300ms 红闪提亮（×0.5，上限 1.5）；thinking 噪声流速/环转速 ×3 + 核心收缩 0.85×（不变色）
> - **舞台背景/光晕**：网格（accent 6%、48px、radial mask 30%→75%）+ 垂直扫描线（accent 45%、7s、opacity 0.5）+ CSS 呼吸光晕（accent 22%→65%，3.2s；语音开 0.55↔1 / 待机 0.25↔0.55），均 TwinSprite `SpriteOverlay.vue`/`SpriteOrb.vue` 原值；DOM 全息化（状态角标/打字机字幕/麦克风脉动光环）与音效引擎 V5 成果沿用未变
> - **listening 电平源**：`HudCanvas.micLevelFromSpectrum()` = TwinSprite `useAudioLevel.sampleMic` 公式原样移植（bins 2..48 均值 /255 ×1.6 钳制），经 `HudScene` 构造注入 `getMicLevel`/`getPlaybackLevel` 拉取（场景不 import voice 模块，单向依赖）
> - **验证**：`hudParams.spec` 11 例锁定 TwinSprite 原值（uAmp 基值/增益、thinking ×3/0.85、配色、红警示、boot intensity 曲线）；`pnpm lint && pnpm test && pnpm build` 全绿（205 文件 1562 测试）；浏览器实测四态（待机微光 → boot 点亮 → 思考中收缩/高速噪声 → 正在播报电平强震）与 TwinSprite 视觉一致

> **As-built 存档（V5 反应堆，2026-08-09，3D 场景部分已被 D12 替代）**：
>
> - ~~**启动过场逐层展开**~~：V5 的 `ringBoot [0,1]×3` 刻度环逐层锁定展开 + `coreScale` 0.75→1 + Bloom 0.35→1 点亮——V7 起随反应堆部件移除，启动过场由光球 `uIntensity` 单参数点亮替代
> - **DOM 全息化落地**（V7 沿用有效）：状态标签 = 四角括号（`&__state-corner`）+ `hud-scan` 扫描线 + 等宽大写发光描边；字幕 = 逐字 span `hud-char-in` 打字机浮现（按索引复用，旧字不重放）+ 全息边框发光；麦克风 = `hud-mic-pulse` 脉动光环常开 + 开启时 `hud-mic-orbit` 虚线轨道环旋转（voiceDisabled 隐藏光环）；HoloConfirmCard 沿用既有 rgba(0,229,255) 全息语言，色值天然对齐
> - **音效引擎实现**（V7 沿用有效）：`SOUND_SPECS` 六配方（boot=sine 180→880Hz 0.5s + triangle 谐波延迟 0.12s；chirp=660→990Hz 0.12s；ping=520→500Hz 0.25s 循环 2s 间隔；ding=880+1320Hz 双音；buzz=square 140→110Hz；cut=sawtooth 300→60Hz）；`UiSoundEngine` 主音量 0.5、指数扫频 + 线性包络、stop 延后 0.05s 收尾；`useUiSounds` 共享单例引擎 + `AudioContext.resume` 自动播放钩子；开关持久化 `aranea.companion.uiSounds`（默认开），切换按钮在 CompanionPage 右上
> - **性能**：`document.hidden` 降帧 15fps 保留（V7 沿用）；V5 的 Bloom mipmapBlur 半分辨率 / InstancedMesh 刻度/频谱/涟漪已随部件移除

> **As-built 存档（初版，V1-T8 + V2-T5，2026-08-08）**：初版场景为能量核（Icosahedron + 等离子噪声 shader + 顶点摆动）/ 粒子环 ×2（内外两圈反向旋转、双频正弦声波震动 26Hz/41Hz）/ 线框壳 / 频谱环 / Jarvis 全息弧线组 ×3（多弧异速旋转、双色交替、呼吸透明度 + burst 提亮）/ 声浪涟漪池 ×4；`HudParams` 状态驱动参数 vibrationGain/arcSpeedFactor/coreWobble/rippleGain；draw call ≤12、三角形 <8k。**V5 重构后上述元素由 v2 场景部件取代（频谱环与涟漪池保留迁移）**。
>
> **As-built（V2-T5 全息确认卡，2026-08-08，不受 V5 影响的保留项）**：
>
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

## 13. 语音听写模式（聊天页语音输入，V6）

> 需求锚点：需求 §2.11 / §5.6。聊天页麦克风按钮的落地形态——语音转文字入输入框，**不建 Turn、不播报**。

### 13.1 模式语义对比

| 行为 | 对话模式（mode 空） | 听写模式（mode=dictation） |
|------|--------------------|---------------------------|
| ASR 终稿下行 `asr.final` | ✅ | ✅（仅文本，前端填入输入框） |
| 创建 Chat Turn / 派发 Agent | ✅ | ❌ |
| TTS 播报 / 事件总线订阅 | ✅（startEventLoop） | ❌（不订阅，省资源） |
| 确认词拦截（V2-T5） | ✅ | ❌（无待决议确认场景） |
| 语句 PCM 留档（V2-T6） | ✅ | ❌（不缓冲，不落盘） |
| voice.state 终稿后 | thinking → speaking → listening | 停留 listening（连续听写） |

### 13.2 后端实现

- `internal/voice/session.go`：`StartParams.Mode` + 常量 `ModeDictation` + `s.dictation()` 判定；`Start` 按模式跳过 `startEventLoop`；`bufferUtterance` 听写模式不缓冲；`handleASRFinal` 下行 `asr.final` 后直接 return（在确认拦截/建 Turn 之前）
- `internal/server/voice_ws.go`：`voiceControlMessage.Mode` 解析透传
- ASR 一句一连接的懒重开逻辑对听写同样生效（连续听写不中断）
- 日志：`voice.session.start` 流程/进程日志均带 `mode` 字段（K1）

### 13.3 前端实现

- `features/chat/composables/useVoiceDictation.ts`（新增）：依赖注入端口（`clientFactory`/`captureFactory` 可 mock）；`toggle()` 启动 = connect + `startVoice({sampleRate:16000, mode:'dictation'})` + 采集启动；`onFinal` → `deps.onFinalText` 回调；`onClose`/`onReplaced`/采集失败 → teardown + 错误上报；`stop()` 幂等
- `features/chat/composables/useChatWorkspace.ts`：装配 `useVoiceDictation`，`onFinalText` 经 `joinDictationText`（空格连接）写入 `inputText` 草稿；watch 会话切换自动 `stop()`；`onVoiceClick` = `toggle()`
- `components/chat/ChatComposer.vue`：听写状态条（红点脉动 + `dictationPartial` 实时文本/「正在聆听…」占位）；麦克风按钮录音态 = 红色填充 + stop 图标 + aria-label「停止听写」
- i18n：`chat.voiceDictationStop/Listening/Error` + 复用 `companion.voiceNoSession/micUnavailable/voiceChannelClosed`（zh-CN/en-US）；移除废弃 `voicePlaceholder`
- 移动端 `MobileChatPage.vue` 与桌面 `ChatPage.vue` 经 `ChatMessagePanel` 透传 `dictating`/`dictationPartial`，行为一致

### 13.4 测试矩阵

| 层 | 用例 | 位置 |
|----|------|------|
| 后端单测 | 听写终稿仅下行文本 / 跳过确认拦截 / 跳过留档 / 连续终稿 | `internal/voice/session_test.go`（4 例） |
| 前端单测 | 启动/停止/终稿入框/错误三路径/关闭与取代/幂等 | `useVoiceDictation.spec.ts`（17 例） |
| 前端协议 | `voice.start` 携带 `mode=dictation` | `useVoiceSession.spec.ts` |
| 真机探针 | 真实火山 ASR 两句连续听写；断言无 turn.accepted / 无 tts / 无二进制下行 / 状态停留 listening | `test/dictation-e2e/dictation_probe.go` |

---

## 14. 语音快速通道延迟优化（2026-08-09/10，Voice Fast-Path）

> 目标 SLO：用户停口（ASR 终稿）→ 首帧音频下行 ≤ ~2s。真机基线（优化前）：意图识别 3.7-26.6s、主 LLM TTFT 2.5-6.7s、BUILD 冷启动 2.6-2.7s、串行 recall 0.3-3.3s——耗时会聚在「思考默认开启 + 串行编排 + 冷启动」三处项目设计问题，非 LLM 本身慢。

### 14.1 根因结论（评审 2026-08-10）

| 根因 | 性质 | 数据 |
|------|------|------|
| deepseek 服务端默认开思考，意图分类被迫走思考段 | 项目配置问题 | 意图 3.7-26.6s；分类任务思考段对 JSON 输出无收益 |
| 主 LLM 硬编码 `Stream=true` 且不读 catalog `thinking_disabled` | 项目设计问题 | 语音 TTFT 2.5-6.7s |
| BUILD 与 Intent Pass、proactive recall 串行 | 项目设计问题 | recall 语音轮次 hits 恒 0，纯开销 0.3-3.3s |
| 进程首轮 BUILD cache miss | 冷启动 | 2.6-2.7s |

### 14.2 架构原则

1. **SLO 优先**：先埋测量（E），一切优化以预算表日志为验收标准
2. **语音一等公民**：语音优化逻辑集中在 `internal/voice/` + 请求级 ctx 标记，通用 chat 路径零感知
3. **语义不变**：并行化/预热只改时序，不改行为契约；文字路径不回退

### 14.3 已实现优化（P0，2026-08-10）

**A. 全局纯提速（文字+语音共享，无行为变化）**

- **意图识别强制关思考**（`internal/agent/intent/pass.go`）：callsite 强制 `cfg.ThinkingDisabled = true`，不依赖 catalog 行配置。意图是分类任务，思考段纯延迟
- **修复 A：skill 同步缓存键契约**（已落地 2026-08-11，`internal/data/skill.go`）：`updated_at` 是 `SkillVersionHash` 的内容版本标记。此前 reconcile 每 5 分钟全量扫描对所有存活 skill 无条件 `SetUpdatedAt(now)`（`UpsertSkillFromDisk`）并无条件写 `MarkSkillFilesystemMissing(slug, false)`，导致 SkillVersionHash 周期性漂移、全量 agent 构建缓存失效（冷构建 10s 级周期性重现）。修复：两处改为**条件写**——仅内容/元数据/可用性（missing 标志）实际变化时才写库并 bump `updated_at`；`MarkSkillFilesystemMissing` 用 `FilesystemMissingEQ(!missing)` 谓词实现翻转才写，n=0 时再区分「已是目标状态」（no-op 成功）与「slug 不存在」（NotFound）
- **BUILD + Intent Pass + proactive recall 三方并行**（`chat_orchestrator_turn.go` errgroup）：`WithProactiveHits` 在 `eg.Wait()` 后注入，MemoryInject before-model 钩子请求时读取，语义不变（happens-before 由 errgroup 保证）；BUILD 的 AwaitHook 在 Wait 后重绑 turn ctx（防 errgroup 派生 ctx 提前取消导致 await 秒败）
- **C3：embedding 冷启动预热**（已落地 2026-08-11，`internal/knowledge/embedder.go` `Prewarm`）：记忆召回分相计时定位——2579ms 召回中 ~2.4s 是 embedding 服务冷启动（TCP/TLS 握手 + 远端模型懒加载），热路径 embed 仅 191ms（L2 2ms / L3 4ms）。`MultiProviderEmbedder.Prewarm` 发最小 "ping" 请求（RETRIEVAL_QUERY task type，不污染摄取流程日志）把冷启动移出首个召回 Turn；**60s 成功时间戳去重**（高频 voice.start 不放大负载）、**失败仅 Warn（K3）且不参与去重**（下次调用重试，K4）、**未配置静默跳过**；不持锁发网络请求（不阻塞正常 Embed 的 RLock），并发双 ping 无害。双探针挂载：启动探针 `startup.embedding_prewarm`（`cmd/admin/app.go`，readiness 门控后与 spirit 预构建并列）+ voice.start 探针（`chat_voice_prewarm.go` `PrewarmTurn` 起始处，不依赖会话/agent 解析结果，窄接口 `embedPrewarmer` 构造注入，wire 绑定 `*knowledge.MultiProviderEmbedder`）

**B. 仅语音路径（文字零影响）**

- **Voice Fast-Path 关主 LLM 思考**（`internal/agent/voice_fastpath.go`）：`input.Voice != nil` 时 `WithVoiceFastPath(runCtx)` 打标（`chat_orchestrator_turn_phases.go` prepareRunContext），BeforeModel 回调（LayerDynamic, priority 4）per-request 置 `GenerationConfig.ThinkingEnabled=false`。BUILD 产物跨入口缓存共享（cache key 不含入口），思考开关只能请求级改写，不烘进构建期；planner/compress 等深度推理调用点不经此标记，规划质量不受影响。`TurnInput.Voice` 唯一赋值点在 `internal/voice/session.go`，文字路径 hook 直接透传
- **E：首音频延迟预算测量**（`internal/voice/session.go`）：T0 = ASR 终稿派发时刻；首帧 TTS 下行消费 T0（每 Turn 只测一次）；打断复位（迟到帧不产生误导测量）。预算 `firstAudioBudget = 2.5s`，超预算 Warn（K2）+ 流程日志 `voice.tts.start` 带 `first_audio_ms`
- **C1：voice.start 预热 Agent 构建缓存**（`internal/service/chat_voice_prewarm.go`）：voice.start 成功后后台 goroutine 触发 `VoiceTurnPrewarmer.PrewarmTurn`——与 `runNativeAgentTurnBody` 同源解析 dialogMode/provider/model 填充 `BuildTRPCAgentCached`，把首个语音 Turn 的构建冷启动移到「开麦→开口」空窗期。非阻断容错：失败仅 Warn；dictation 模式与团队会话跳过；缓存 key 同源保证预热产物被真实 Turn 命中。接线：`voice_ws.go` setter 注入（`SetTurnPrewarmer`）+ `wire.go`
- **C1b：启动期预构建 `__spirit__`**（已落地 2026-08-11，`ChatService.PrewarmSpiritAgent` + `cmd/admin/app.go`）：voice.start 预热只能覆盖「开麦→开口」空窗，进程首个 Turn 仍付冷构建（实测 2.6-8s）。readiness 门控后后台预构建 spirit agent 缓存（dialogMode 固定 "default" 归一化、合成 session 仅提供 ID——AwaitHook 运行时才从 ctx 解析，构建期安全）；非阻断容错，失败仅 Warn 不阻塞启动（Always-Ready）
- **P0：工具构建 N+1 消除**（已落地 2026-08-11，`internal/agent/tool_build_catalog.go`）：冷构建实测 10.2-10.7s 的主因是 `applyRuntimeToolConfigs` 与 `buildCatalogConfirmTools`（确认门重复构建两次：toolset 策略 + callback chain hook）各对启用工具逐个跑聚合 `GetTool` 查询（70 工具 × 3 ≈ 210 次 × ~49ms）。修复：`toolBuildCatalog` 每次构建只跑两条批量 SQL（`ListToolCatalogEntries` IN 批量 + overrides 列表）生成快照，eff 键集/快照/确认门在 `BuildTRPCLLMAgent` 加载一次，共享给 `buildToolsetsForAgent` 与 `buildCallbackChainOptions`。降级语义与逐键 GetTool 时代完全一致：快照加载失败 runtime config 全跳过（fail-soft）、确认门 fail-closed（所有启用工具需确认）；工具行缺失 fail-closed。配套：构建六段（model/prompt/skill/tools/finalize/new）+ tools 段五子相（rt_config/prune/build/gate/decor）K6 计时日志，超 2s 升级 Warn；`BuildTRPCDeps` 补 LG 注入（修复构建路径日志被 Noop 静默吞掉）
- **C2：ASR partial 投机意图**（已落地 2026-08-11）：投机阶梯 L2/L3。`internal/voice/session.go` 追踪 partial 稳定（文本 500ms 无变化触发，`trackPartialStability`/`fireSpeculation`；同文重发不重置，final/cancel/teardown 停止计时）→ `voice.IntentSpeculator` 端口 → `internal/service/chat_voice_speculate.go` `VoiceIntentSpeculator`：与 Turn 侧 `runIntentPass` 同源解析 session/agent/provider/model/历史，后台预跑意图识别（15s 调用超时），产物存**每会话单槽**（`sourceText` 归一化精确匹配 = sourceHash 语义 + `createdAt` TTL 30s = expiresAt 语义）。ASR final（L3 判定）经 `WithSpeculativeIntent` 校验：一致且有界等待（cap 2s）在途投机完成 → 经 `intent.WithSpeculativeArtifact` 注入 Turn ctx；失配/超时/失败即丢弃走常规意图路径。Turn 消费（`chat_orchestrator_turn.go`）：新增 `SpeculativeArtifactFromContext` 分支——**fresh 语义不剥离澄清残留**（与澄清续跑 key 隔离，澄清门照常评估），复用记 `outcome=reused_speculative`。去重：同文投机在槽不重复调 LLM；新 partial 取代旧槽。dictation/团队会话/A2A/意图未启用跳过
- **语音轮次跳过主动召回**（已落地 2026-08-11，`chat_orchestrator_turn.go` `shouldRunProactiveRecall`）：`input.Voice != nil` 时不启动 recall goroutine（记 `chat.proactive_recall` LogSkip）。真机实测语音轮次 hits 恒为 0（短口语句实体提及稀少），而 recall 含 query embedding + 向量检索耗时 0.3-3.3s；三方并行后 `eg.Wait()` 仍以**最慢 goroutine** 收口，零产出召回是关键路径纯开销，单独即可击穿 ≤2s 停口到首音预算。文字路径行为不变
- **L5：TTS 连接预热**（已落地 2026-08-11，`internal/data/speech/volcengine_tts.go` + `internal/voice/session.go`）：voice.start 预解析 TTS provider 存入会话（`resolveAndPrewarmTTS`，`ensureTTS` 复用不再二次调工厂）并后台预拨一条 WS 连接存**单槽温连接**（`PrewarmTTSConn`）；首个 Turn 首句 `Write` 弹出复用免握手，写失败回退新拨（K3）；会话拆除释放未消费温连接（`ReleaseWarmTTSConn`，一次性语义）。provider 无预热能力（接口断言失败）自动跳过

**C. 前缀稳定化（P1/P2，2026-08-11）**

- **Prompt cache 前缀稳定化**（已落地 2026-08-11）：DeepSeek prompt caching 从 token 0 匹配，system-message 前缀内任何 per-turn 变化使整段缓存失效。真机分析 spirit agent 初始 prompt ≈10.1k token（5296 system + 4803 tool schema），但 chat_turn 缓存命中率仅 6.3%（team 路径 0%）——根因是所有 BeforeModel 注入钩子用 prepend，per-turn 动态内容（memory/knowledge cue）落在 position 0，每轮失效整个前缀。修复：8 处注入点全部改用 `insertAfterLastSystem`（`runtime_cue_inject`×2、`skill_guidance_inject`×2、`memory_inject`×2、`knowledge_inject`、`reply_reminder`），配合 Hook Layer 排序（Static→SemiStatic→Dynamic）保证三层前缀结构。详见 [28-callback.design.md §3.1](./28-callback.design.md#31-system-message-注入顺序前缀稳定化)
- **P2 深化：per-turn 动态 cue 末尾追加 + intent 搬移**（已落地 2026-08-11）：P1 后真机复测命中率仅 19%——`insertAfterLastSystem` 仍把 per-turn 动态 cue 插在 system 块后、history 前，history 每轮增长使可缓存前缀在动态 cue 处截断，长会话命中率随轮次衰减。彻底修复（两档契约）：会话级稳定 cue（static/semi-static runtime cue、skill guidance）维持 `insertAfterLastSystem`；per-turn 动态 cue 一律 **append 到消息列表末尾**——`memory_inject.go`（含 compaction rebuild 兜底：有既有 cue 原位替换、无则末尾追加）、`knowledge_inject.go`、`reply_reminder_inject.go` 四处改为 append；intent JSON 由框架 content processor 经 `RunOptions.InjectedContextMessages` 固定在 system 块后 history 前（注入点不可控位），新增 `newIntentReorderBeforeHook`（`intent_reorder_inject.go`，LayerDynamic priority 100，晚于全部消息改写钩子）稳定分区搬移到末尾——每次模型调用（含工具循环重入）请求重建，搬移幂等不累积。最终结构：`[system 块 + history + user]` 单调增长可缓存前缀 + 尾部动态段每轮重算。测试契约 `prompt_prefix_position_test.go`：`assertCueAfterBase`×4（稳定档）+ `assertCueAtEnd`×4（动态档）+ intent 搬移×4

**D. TTS 文本清洗（上一轮，2026-08-09）**

- `cleanForSpeech`（`sentence_chunker.go`）：剥离成对 markdown 强调符（`**`/`__`/`~~`/`*`/`_`）、行首标题/列表/分隔线/引用符及 emoji，防 TTS 把 `*` 读作「星星」

**E. TTFT 二轮深化（已落地 2026-08-11）**

- **P0-A：预算表分解日志**：per-turn token/缓存命中入进程日志（`chat_turn_metrics.go`，step `chat.turn_usage` 带 cached_tokens/cache_hit_ratio，前缀稳定化效果直接可验）；记忆 cue 构建计时（`memory_inject.go`，step `agent.memory_cue.build` 带 cue_chars/recall_hits）；turnStreamConsumer logger 关联 session_id（`stream_consumer.go`），TTFT 黑盒可按会话拆段
- **P0-C：单轮共享 embedding**（`memory_composite_recall.go` + `memory_l2_recall.go` + `memory_l3_fused_recall.go`）：composite layered 路径每轮只 Embed 一次（3s 超时 `memoryRecallEmbedTimeout`；失败置 EmbedAttempted，层内降级非向量检索且不再各自重复 embed），向量经 `QueryEmbedding`/`EmbedAttempted` 字段传给 L2/L3 fused；Wire 装配 `SetEmbedder`（`wire_memory.go`，typed-nil 守卫）。此前 L2/L3 各自对同一 query 独立 embed，关键路径多付一次网络往返
- **P0-D：温连接消费后异步补充**（`volcengine_tts.go`）：`popWarm` 消费即 safego 后台重拨补槽，后续句/后续轮首句继续免握手，同步握手彻底移出用户感知关键路径；`released` 标志 teardown 后抑制补充、迟到预热连接到达即关（防泄漏）；补充失败仅少一次预热，下一句回退同步拨号（K3）
- **P1-E：skill overview stale-while-revalidate**（`skill/trpc/db_repository.go`）：TTL 自然过期且已有快照 → 旧快照立即应答 + safego 后台 single-flight 刷新，每轮 ~280ms 同步全量拉 DB 移出请求路径；冷启动/Invalidate 后仍同步拉（变更立即生效语义）；失败退避 30s（loaded 前移）防 DB 故障期每请求重试；`invalidateGen` 代际替代 loaded 零值表达「fetch 期间又 Invalidate」——修复冷启动后 loaded 永零、TTL 永不生效导致缓存形同虚设
- **P1-F：投机意图归一化匹配**（`chat_voice_speculate.go`）：`normalizeSpeculativeMatchText`（小写、去 Unicode 标点/空白）替代 sourceText 精确匹配——ASR final 对 partial 的标点润色（补句号/去逗号）归一化后命中复用投机产物；实体差异（改口/纠错）归一化后仍不同照常失配丢弃；同文标点变体重触发去重不重复调 LLM

### 14.4 待办（P1/P2）

| 优先级 | 项 | 内容 |
|--------|----|------|
| ~~P1~~ ✅ | 首句快速通道（已落地 2026-08-10） | `firstSentenceMinRunes` 6→4，首句遇次要标点提前切出；4 为下限防碎句 |
| ~~P1~~ ✅ | C2 ASR partial 投机意图（已落地 2026-08-11，见 §14.3-B） | partial 稳定 500ms 启动意图识别；final 文本失配即丢弃投机结果走常规路径 |
| P1 | 文字侧 catalog 主 LLM 思考控制 | 构建期烘焙不可行（BUILD 缓存指纹不含 provider/model，运行时经 `WithModel` RunOption 覆盖，语义错位）；正确实现 = BeforeModel 钩子经 `agent.InvocationFromContext` 定位当前模型 + catalog 查找（带缓存）。语音侧已由 Voice Fast-Path 覆盖，本项仅服务文字路径 |
| P2 | D 工具过渡句 filler | 复用 orchestration_progress/工具事件，事件驱动一句话可打断播报，根治工具静默 |
| ~~P2~~ ✅ | L5 TTS 连接预热（已落地 2026-08-11，见 §14.3-B） | voice.start 预拨 TTS WS 温连接，首句弹出复用免握手 |

### 14.5 风险与缓解

| 风险 | 缓解 |
|------|------|
| 投机意图不一致 | sourceHash + expiresAt；final≠partial hash 即丢弃 |
| 预热负载放大 | singleflight 与真实 Turn 合并构建；仅 voice.start 触发，无定时器 |
| fast-path 标记蔓延 | 标记仅 `input.Voice != nil` 注入；禁止新增 ctx flag（项目红线） |
| 并行段 ctx 提前取消 | AwaitHook Wait 后重绑 turn ctx（已修复并回归测试） |

---

## 15. 语音助手前台模式（Voice Butler，2026-08-10 设计）

> 需求锚点：语音是辅助手段，不改精灵既有模式。语音助手与精灵助手**同级**、**各自独立**；语音助手接收语音指令，复杂任务委派精灵异步执行，委派期间语音助手持续陪聊，精灵反馈到达后实时播报。

### 15.1 架构定位

```
用户语音 ⇄ 语音助手（新内置 agent，轻量前台：快答 / 委派 / 播报）
                │ ① delegate_to_spirit(task) 工具调用（异步，立即返回）
                ▼
          精灵助手（__spirit__，完整能力后台执行：思考/规划/工具/团队，零改动）
                │ ② 完成 → 系统推送事件（复用团队完成同款 TurnGateway 链路）
                ▼
          语音会话 eventLoop（delegation 关联订阅）→ TTS 实时播报
```

| 角色 | 实体 | 职责 | 能力集 |
|------|------|------|--------|
| 语音助手 | 新内置系统 agent（独立 agent_key） | 快答闲聊/简单问答、复杂任务委派、结果播报 | 轻量 prompt + `delegate_to_spirit` 单一工具；voice fast-path（关思考） |
| 精灵助手 | `__spirit__`（现状不变） | 复杂任务全能力执行：深度思考/规划/工具链/团队编排 | 现状完整能力，**零改动** |

### 15.2 核心数据流

1. **快答**：用户语音 → ASR → 语音助手 turn（fast-path）→ 直接答 → TTS（现状 V8 链路不变）
2. **委派**：语音助手 LLM 自主判断复杂任务 → 调 `delegate_to_spirit(task)`：
   - 工具内：查/建用户的 spirit 主会话 → 提交**异步** turn（detached ctx goroutine 派发，不等完成）→ 登记 delegation（voice_session_id ↔ spirit_session_id + **task_id**）→ 立即返回「已交办精灵助手」
   - 语音助手收到工具结果 → 口播确认 → **turn 正常结束**，语音助手继续陪聊（每个语音 turn 天然独立，无阻塞点）
3. **播报**：精灵 **task 终态**（`TaskCompletedEvent`；团队场景含系统推送 synthesis 续跑 turn 完成后才终态）→ 事件总线 → voice eventLoop 按 delegation 关联（spirit_session_id + task_id）匹配 → 取该会话最新 completed reply step 全文 → cleanForSpeech 清洗 → TTS 播报 → 回 listening

> **评审修正（2026-08-11，R9）**：播报触发由「TurnCompleted」改为「**TaskCompleted**」。委派 turn 完成 ≠ 任务完成——团队场景下委派 turn 先产出「已组建团队」reply 并终态 turn，但 task 保持 running，最终总结由 checkAllTeamsCompleted 系统推送的 synthesis 续跑 turn（ParentTaskID 续在同一 task）产出。按 taskID 匹配 TaskCompletedEvent 可统一覆盖直答场景（turn 完成即 task 完成）与团队场景（synthesis 后 task 才终态），播报的必是最终总结。

### 15.3 关键设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 委派调用形态 | **异步 turn 提交**（复用系统推送同款 `TurnGateway.ExecuteTurn` 入口） | 框架 AgentTool 是同步调用（父等子），会卡住语音助手，违背「委派后时刻陪聊」 |
| 精灵执行可见性 | spirit **主会话**执行，chat 页可见全过程 | 用户已确认；可追溯可干预；无需新建隐藏会话体系 |
| 分流策略 | 语音助手 LLM **自主决策**，prompt 声明委派边界（需工具/多步/团队 → 必委派；闲聊/单句问答 → 直答） | 用户已确认；agent 模式自然形态 |
| 播报文本 | 首版：**精灵回复全文**经 cleanForSpeech + chunker 分句播报（长文可打断） | 零额外 LLM 调用；播报摘要化（语音助手浓缩 1-2 句）列 P1 优化 |
| 播报时 voice 正忙 | 排队：当前 turn tts.end 后播 | 用户说话优先；不打断进行中的问答 |

### 15.4 组件清单

| # | 组件 | 位置 | 说明 |
|---|------|------|------|
| A | 内置语音助手 agent 种子 | `internal/data/seed_system_admin.go`（spirit 同款：agent 行 + runtime_settings 行 + prompt files） | 独立 agent_key `__voice_butler__`/人格 prompt/委派边界声明；runtime_settings 行**显式** `intent_pass_enabled=false`（**评审修正 R14**：DB 列默认 true，minimal insert 必须显式写 false）+ `tools_profile='chat_only'`（**评审修正 R13**：空 registry 工具集；delegate 工具走 CustomTools 注入，`tool_assembly.go` 无条件追加、绕行 profile allow-set，无需新建 profile） |
| B | `delegate_to_spirit` 工具 | `internal/tools/`（工具实现）+ **orchestrator 条件注入**（`cli_admin_tools.go` 同款 `NewSynthesizeResultsTool` 模式：依赖 TurnGateway/SessionUsecase/Registry 在 service 层，`tools.Registry()` 静态工厂拿不到） | 仅当 agent_key==`__voice_butler__` 时注入；**评审修正 R12**：依赖用窄端口 `biz.TurnExecutorGateway`（`mailbox_waker.go` 先例，Wire 绑 ChatService）；detached goroutine 用 `safego.Go(appctx.Ctx(), ...)` + `ctxuser.WithUserID`（voice 预热同款 session.go:246-252）；`ErrTurnMessageQueued` = 已受理非失败（synthesis 同款语义 spirit_team.go:929）；提交同步失败 → `registry.MarkFailed` → watcher 通知 voice Session 口播「交办失败」 |
| C | DelegationRegistry | **`internal/voice` 具体类型**（进程级单例，Wire 提供；**评审修正 R11**） | **评审修正 R11**：server→service 依赖方向不可逆，registry 不能由 VoiceWSServer 持有再暴露；Wire 单例双向注入——① `VoiceWSServer.SetDelegationRegistry`（`SetTurnPrewarmer` setter 先例 wire.go:2905）→ SessionDeps → Session；② service 层 `voiceButlerTools` 工厂。条目：`{voiceSessionID, spiritSessionID, content, taskID(pending→bound), status}`；**watcher 回调**（同包 Session 注册监听）支撑提交失败口播；voice session 关闭即清条目；进程重启退化为 chat 页查看 |
| D | eventLoop **三路分流** + task 绑定 + 播报队列 | `internal/voice/session.go` | **评审修正 R3**：`V2Bus.Subscribe` 忽略 `EventSubscribeOptions` 过滤参数（全量广播），eventLoop 现状不过滤 session。必须显式三路分流：① `SpiritSessionID()==s.sessionID` → 现有快答路径；② 命中 delegation registry → 委派路径；③ 其余 → 丢弃。否则精灵执行的流式 delta 会串入语音 TTS（串话）。**评审修正 R10**：`ExecuteTurn` 阻塞且 `TurnResult` 无 TaskID → 登记时 taskID 留空；`TaskCreatedEvent.Task.UserMessage == TurnInput.Content`（projector.go:246）→ 按 (spirit_session_id + 内容精确匹配) 绑定 taskID，先注册后提交无窗口期，内容匹配免疫并发外来 turn 错绑；排队 turn 延迟建 task 天然兼容。终态：`TaskCompletedEvent`（含 cancelled，`Task.Status` 区分）/ `TaskFailedEvent` → 取 reply 全文播报。**播报机制**：listening 态无活跃 turn/chunker  flush 源，须一次性 `ensureTTS→Write→Flush→flush 哨兵`（复用澄清播报注入路径 + handleTurnCompleted 哨兵逻辑）；voice 正忙（thinking/speaking）→ session 内 FIFO 队列，回 listening 时排空 |
| E | 前端语音入口改绑 | `web/src/pages/CompanionPage.vue` + `useChatWorkspace` | **评审修正 R5**：Turn 执行按 `sess.AgentID` hydrate agent（`chat_orchestrator_turn.go`，忽略 `input.AgentKey`），仅改 voice.start 的 agent_key 无效。Companion 进入语音模式时须选中/创建 **agent_id 属于 `__voice_butler__` 的会话**（复用现有会话选择体系；无则经会话 API 创建） |

### 15.4.1 评审结论（2026-08-11 代码级核验）

| # | 核验项 | 结论 |
|---|--------|------|
| R1 | 内置 agent 种子机制 | ✅ 可行：`seed_system_admin.go` 四内置 agent 同款三件套（agent 行 `ON CONFLICT(agent_key)` + `SeedSystemAgentRuntimeSettings` + `SeedButlerPromptFiles`），直接复用 |
| R2 | TurnGateway 异步委派入口 | ✅ 可行：`biz.TurnGateway.ExecuteTurn(ctx, TurnInput)` + `ParentTaskID` 系统推送先例（TeamStarter→checkAllTeamsCompleted→synthesis turn）；委派工具经 Wire 注入 `TurnExecutorGateway` 窄接口即可 |
| R3 | 事件订阅过滤 | ⚠️ **必须分流**：`V2Bus.Subscribe(_ biz.EventSubscribeOptions)` **忽略过滤参数**，全量广播；voice eventLoop 现状亦无 session 过滤（隐含假设"同时只有一个语音会话的活动 turn"）。引入委派后精灵后台执行事件与语音会话事件同通道，不分流必串话。按组件 D 三路分流 |
| R4 | 工具注册方式 | ⚠️ **注入路径修正**：`tools.Registry()` 是静态工厂（`func(ctx) (ToolSet, error)`），拿不到 service 层依赖。`delegate_to_spirit` 需要 TurnGateway + Session 查询 + DelegationRegistry，必须走 `cli_admin_tools.go` 同款 orchestrator 条件注入（`NewSynthesizeResultsTool(o.spiritSynthesis())` 先例），按 agent_key 条件挂载 |
| R5 | 前端 agent 绑定 | ⚠️ **绑定会话而非 agent_key**：Turn 执行 `agentID := sess.AgentID`（hydrateAgent），`TurnInput.AgentKey` 仅作透传标记不参与 agent 解析。前端必须选中/创建 agent_id 属于语音助手的会话 |
| R6 | 意图识别开关 | ✅ 可行：`AgentRuntimeSettings.IntentPassEnabled` per-agent 开关（`intent.IntentPassFromAgent`），种子 runtime_settings 行写入 `false` 即可（委派分流由 LLM 自主工具调用决策，无需前置 IntentPass 分类） |
| R7 | 播报文本来源 | ✅ 可行：复用 `spirit_team_usecase.go` 取终稿模式——`stepReader.ListStepsBySessionID` 找最后一条 `Kind==StepKindReply && Status==completed` 的 step 取 Content；voice 侧经注入窄端口（`biz.StepV2Reader` 子集）读取，不反向依赖 service |
| R8 | spirit 主会话查/建 | ✅ 可行：`SessionUsecase.Search(SessionSearchQuery{AgentID, UserID, RootOnly:true})` 查；无则 `SessionUsecase.Create`（OwnerType=agent, AgentID=spirit agent id）建。工具委派目标恒为用户唯一 spirit 主会话（与 chat 页默认会话一致） |
| R9 | 播报触发事件 | ⚠️ **TurnCompleted→TaskCompleted**：见 §15.2 评审修正。团队场景委派 turn 先终态（reply="已组建团队"），task 仍 running；synthesis 续跑 turn 完成后 task 才终态。按 taskID 匹配 `TaskCompletedEvent` 统一覆盖直答/团队两场景 |
| R10 | task_id 登记时机 | ⚠️ **内容匹配绑定**：`ExecuteTurn` 阻塞至 turn 完成（`pipeline.Run` 同步），`TurnResult` 无 TaskID 字段 → 工具提交时拿不到 task_id。但 `TaskCreatedEvent.Task.UserMessage` == `TurnInput.Content`（`projector.go:246` `meta.newTask(..., meta.TaskContent)`）。方案：登记时 taskID 留空（pending），eventLoop 收 `TaskCreatedEvent` 按 (spirit_session_id + UserMessage 精确匹配) 绑定；先注册后提交无漏绑窗口，内容匹配免疫外来并发 turn 错绑；排队 turn（`ErrTurnMessageQueued`）延迟建 task 天然兼容 |
| R11 | Registry 共享位置 | ⚠️ **Wire 单例双向注入**：VoiceWSServer 在 server 层、工具工厂在 service 层，server→service 方向不可逆 → registry 具体类型放 `internal/voice`（纯内存 struct + sync.Mutex），Wire 提供单例；`VoiceWSServer.SetDelegationRegistry` setter 注入（`SetTurnPrewarmer` 先例，`wire.go:2905`），工具工厂经 orchestrator deps 注入。service import voice 无环（voice 仅依赖 biz/event） |
| R12 | ExecuteTurn 阻塞性与容错 | ⚠️ **阻塞确认 + 排队语义**：必须 detached goroutine（`safego.Go(appctx.Ctx())` + `ctxuser.WithUserID`，voice 预热同款）。`ErrTurnMessageQueued` = 已受理（spirit 会话有活跃 run 时排队，排水后照常建 task），非失败；同步错误（admission 拒绝/DB 错）→ 永无 TaskCreated → `registry.MarkFailed` + watcher 回调 voice Session 口播失败，防 delegation 泄漏空等。工具依赖用窄端口 `biz.TurnExecutorGateway`（`mailbox_waker.go:26` 先例） |
| R13 | chat_only profile 可行性 | ✅ `toolProfiles["chat_only"] = {}` 空 registry 工具集；`tool_assembly.go:89` `deps.CustomTools` **无条件追加**、绕行 profile allow-set → delegate 工具注入不受 chat_only 限制；CustomTools 参与 `BuildCacheKey`（cache.go:310）→ 语音助手构建缓存独立，不污染他 agent |
| R14 | intent_pass_enabled 默认值 | ⚠️ **DB 默认 true**（`agent_runtime_setting.go:115` `Default(true)`）→ 种子 minimal insert 必须显式写 `intent_pass_enabled=false`，默认依赖列默认值会静默开启 IntentPass |
| R15 | 委派播报的 TTS 生命周期 | ⚠️ **listening 态无 flush 源**：澄清播报依赖本会话 turn 的 TurnCompleted 触发 `chunker.Flush`；委派播报到达时 voice 在 listening（无活跃 turn）→ 必须自足：`ensureTTS → chunker.Write(全文) → Flush → flush 哨兵`（复用 `handleTurnCompleted` 哨兵逻辑驱动 OnDrained → tts.end → 回 listening）。voice 正忙时入 session FIFO 队列，回 listening 排空（§15.3「播报时 voice 正忙」决策落地） |

### 15.5 与既有能力的关系

- **V8 优化全部继续生效**：语音助手 turn 同样走 fast-path/预热/首句快速通道（标记机制与 agent 无关）
- **精灵零改动**：文字路径/团队编排/深度思考完整保留；委派 turn 在精灵侧就是一次普通用户 turn（来源标记 `voice_delegation` 仅用于审计/播报路由）
- **听写模式不受影响**：dictation 不建 turn 不订阅事件

### 15.6 风险与缓解

| 风险 | 缓解 |
|------|------|
| 语音助手误委派简单问题（响应反而变慢） | prompt 委派边界 + 实测调优；误委派不致命（结果仍正确播报） |
| 委派后进程重启 delegation 丢失 | 内存 registry 限定 voice 存活期；重启后结果在 chat 页可见（主会话执行），不退化正确性 |
| 长回复播报冗长 | 语音打断机制已有；P1 播报摘要化（fast-path LLM 浓缩） |
| 多委派并发完成乱序播报 | registry 按 task 登记，完成一条播一条（FIFO 队列） |
| 精灵后台事件串入语音 TTS（R3） | eventLoop 三路分流（组件 D），非本语音会话且非关联 delegation 的事件一律丢弃 |
| 委派 task 失败/取消无播报 | 匹配 `TaskFailedEvent`/取消终态同样触发播报（口播「任务未能完成」+ 失败简报），避免用户空等 |
| 委派播报进行中用户开口（实施后评审竞态修复） | listening 态存活的 scheduler 必为无主自足播报，`handleASRFinal` 开新 turn 时按 barge-in 语义锁内摘除、锁外 `Cancel()`——否则其 flush 哨兵 OnDrained 会把 thinking 经 EvTTSEnd（无文本 Turn 合法出口）提前拍回 listening，新 turn delta 全丢 + tts.end 缺失。回归：`TestSessionDelegationBroadcastCancelledByNewTurn` |

---

## 16. 语音唤醒与休眠（Wake Word「小媛」，2026-08-12 设计）

> 需求锚点：§2.12。语音模式进入即待命（dormant）：本地唤醒词检测、音频不出设备、云端 ASR 零占用；检出「小媛」唤醒进聆听；退出词或 60s 静默回待命。对齐业界「离线唤醒 + 在线识别」范式（Alexa/小爱同学同款架构）。

### 16.1 技术选型

| 方案 | 隐私 | 成本 | 延迟 | 结论 |
|------|------|------|------|------|
| 服务端 ASR 常开检测唤醒词 | 差（音频全上传） | 高（ASR 长连接 24h 计费） | 高 | 否 |
| VAD 门控 + 服务端唤醒词判定 | 中（有声段上传） | 中 | 中 | 备选 |
| **本地 KWS（sherpa-onnx WASM）** | 好（音频不出设备） | 零 | 低（<100ms 检出） | **采用** |

- 模型：`sherpa-onnx-kws-zipformer-wenetspeech-3.3M-2024-01-01`（encoder int8 ≈4.8MB，WASM SIMD），浏览器 AudioWorklet 常开喂 16kHz 帧
- 关键词表（`keywords.txt` 音素序列）：`x iǎo y uán @小媛` + 叠词 `x iǎo y uán x iǎo y uán @小媛小媛`（双音节词误触发率高于四音节，叠词作兜底线）
- 检出阈值 0.25（可配）——spike 实测校准值：0.7 在该模型上全量漏检不可用（`test/kws-spike/verify-kws.js`）；静态资产经 `web/public/kws/` 分发，加载一次全局缓存
- **KWS 加载失败降级**：模型/WASM 加载异常时自动发送 `voice.wake` 进入 listening（退化为现状「进入即聆听」），不阻塞语音模式

### 16.2 状态机扩展（7 态，AS-FSM-01）

新增 `dormant`（待命：语音模式开、本地 KWS 监听、ASR 关闭）。新事件：`wake` / `sleep_timeout` / `exit_word`。

| from \ event | voice_start | voice_start_dormant | wake | sleep_timeout | exit_word | voice_stop | 其余 |
|---|---|---|---|---|---|---|---|
| idle | listening（dictation / error 恢复，不变） | **dormant**（companion 默认） | — | — | — | — | — |
| **dormant** | — | — | **listening** | — | — | idle | 忽略（含迟到 asr/tts 事件） |
| listening | — | — | — | **dormant** | **dormant** | idle | 照旧 |
| thinking / speaking | — | — | — | — | — | idle | 照旧（交互中不休眠；sleep_timeout 到期被转换表拒绝即忽略） |
| interrupted | — | — | — | — | — | idle | 照旧 |
| error | listening（既有恢复路径，不变） | — | — | — | — | idle | 照旧 |

> 实现注（R4）：`voice_start` 保持 idle/error→listening（dictation + recoverToListening 恢复路径不变）；新增 `voice_start_dormant`（仅 idle→dormant，companion 默认入口）。同事件双目标违背 AS-FSM-01 显式转换表（Transition 签名 from+event→to 无上下文），故拆两事件，`Start` 按 mode 选发。

### 16.3 协议变更（§2 增量）

- 上行新增 `voice.wake` JSON `{source}`——`source ∈ kws | manual | system`（KWS 检出 / 待命态点击麦克风 / 委派系统唤醒）；dormant 态收到即 EvWake
- 下行 `voice.state` 状态集合 +`dormant`；休眠/唤醒一律以服务端状态广播为准（前端不本地翻转）
- `voice.start`：`mode=dictation` 仍发 EvVoiceStart 直进 listening；companion 默认发 EvVoiceStartDormant 进 dormant
- dormant 态前端**不上行音频帧**（门控在采集端）；检出唤醒后将预滚缓冲随实时流续传

### 16.4 后端实现（`internal/voice/`）

**① 唤醒词剥离与退出词匹配（`wake_words.go` 新增）**

- 同音词表：云端 ASR 可能把「小媛」识别为同音字——`{小媛, 小圆, 小袁, 小源, 小园, 小员}`；`StripWakeWord(text)` 剥离句首唤醒词（含叠词与「小媛小媛」连说形态）+ 紧随的逗号/顿号/空格，返回净文本与是否命中
- 退出词表：`{休息吧, 再见, 退出, 退下吧, 不用了}`；`MatchExitWord(text)` 整句归一化精确匹配（复用 `normalizeConfirmWord` 归一化规则）
- **拦截顺序（评审确认）**：唤醒词剥离 → 退出词匹配 → 确认词拦截（confirm_words.go）→ Chat 管线。`不用了` 与确认 denyWords 重叠，退出词优先——代价：pending confirm 期间说「不用了」判为休眠而非拒绝（确认卡仍在 UI 可见，可手动处理），语义可接受

**② 会话唤醒/休眠扩展（实现于 `session_wake.go`，2026-08-13 自 session.go 抽离——AS-COG-01 债务控制）**

- `Wake(source string)`：dormant → EvWake → 懒启动 ASR 上游 → 广播 listening；`source` 入流程日志
- SleepTimer：进 listening 启动 60s 计时；asr.partial / asr.final / barge_in / Turn 活动即重置；到期 EvSleepTimeout → dormant（关闭 ASR 上游，零占用）
- 退出词命中：自足 TTS 应答确认（「好的，我先休息了」）→ 应答 flush 后 EvExitWord → dormant
- 自足 TTS 应答（唤醒「我在」/ 退出确认）：复用 §15 委派播报的无主自足 scheduler 机制（不经 Chat Turn，不占消息流）
- **KWS 无后续内容的单唤醒**：前端检出后上行 `voice.wake`，后端自足应答「我在」；**连续指令形态**：预滚音频续传 → ASR 终稿「小媛，查天气」→ `StripWakeWord` 剥离 → 净文本进既有 Chat 管线（唤醒词不进 Turn）

**③ dormant 委派播报系统唤醒（G1 评审修正）**

dormant **保持**事件总线订阅与 delegation watcher（仅延迟 ASR/TTS 预热）——否则待命期间委派任务终态事件丢失。委派终态到达且当前 dormant：系统 EvWake（`source=system`）→ listening → 走 §15 自足播报 → 播报完按 SleepTimer 规则回 dormant。用户显式交办的动作不被休眠吞掉。

**④ WS 路由（`voice_ws.go`）**：`voice.wake` 帧解析 → `session.Wake(source)`。

### 16.5 前端实现（`web/src/features/companion/`）

- **`voice/wakeWord.ts` 新增**：sherpa-onnx WASM KWS 封装——加载 `/kws/` 静态资产（wasm/data/js + 模型），AudioWorklet 16kHz 帧喂入，检出回调；暴露 `load()` / `start(onDetect)` / `stop()`；加载失败 reject（调用方降级自动唤醒）
- **预滚 ring buffer**：采集端恒定维护 ~1.5s PCM 环形缓冲；dormant 态帧只进缓冲不上行；检出唤醒后先 flush 缓冲再续实时流——保证「小媛，查天气」后半句与唤醒词同句完整到达 ASR
- **`useVoiceSession.ts`**：dormant 门控（state≠listening/thinking/speaking 时音频帧不 send）；`wake(source)` 发 `voice.wake`；`voice.state` dormant 映射 store
- **store + HUD**：`voiceState` union +`"dormant"`；HUD 映射 dormant = 微光呼吸（低 gain 慢脉动，青蓝系不变——琥珀/黄禁用），listening = 点亮（现状样式）；手动唤醒：dormant 态点击麦克风按钮 = `wake("manual")`

### 16.6 流程日志步骤（双轨制，K1/K5）

| step_id | 中文标题 | 时机 |
|---------|---------|------|
| `voice.wake.detect` | 语音唤醒 | EvWake 受理（含 source 字段：kws/manual/system） |
| `voice.sleep.exit_word` | 退出词休眠 | 退出词命中、应答后 EvExitWord |
| `voice.sleep.timeout` | 静默休眠 | SleepTimer 到期 EvSleepTimeout |

进程日志：`Wake`/`Sleep` 关键路径 Info（K5），KWS 降级 Warn（K3），ASR 上游启停 Debug + 耗时（K6）。

### 16.7 测试策略

- 状态机：7 态合法/非法转换全表（`session_state_machine_test.go`）
- `wake_words_test.go`：同音词剥离（句首/叠词/带标点/非句首不剥/无命中）、退出词匹配（归一化/重叠词优先级）
- `session_wake_test.go`（11 例）：Wake 懒启动 ASR、SleepTimer 重置/到期、退出词拦截顺序（先于 confirm）、退出词应答后必达 dormant（含 50 轮竞态压测回归 `TestSessionExitWordRaceStress`）、dormant 委派系统唤醒播报、自足应答不经 Chat Turn、唤醒词剥离净文本进 Turn
- 前端：`wakeWord.spec.ts`（加载失败降级 / 检出回调）、`useVoiceSession.spec.ts`（dormant 门控不上行、wake 帧、预滚 flush）
- 运行时：真机「小媛」唤醒 / 连说指令 / 退出词 / 60s 静默 / 待命委派播报全链路

### 16.8 风险与缓解

| 风险 | 缓解 |
|------|------|
| KWS 模型权重 license 未显式声明（代码 Apache-2.0） | 商用分发前确认权重授权；必要时自训练或换开放权重模型（架构不变，仅换资产） |
| 双音节唤醒词误触发率高于四音节 | 阈值 0.25（spike 实测校准）可配；叠词「小媛小媛」兜底关键词；误唤醒代价低（说「休息吧」即回待命） |
| ASR 同音误识别（小圆/小袁…）致剥离失败 | 同音词表覆盖；剥离失败时整句进 Chat 管线（退化为现状，不丢指令） |
| 退出词与确认词「不用了」重叠 | 拦截顺序退出词优先（§16.4①，评审确认）；确认卡 UI 保留可手动处理 |
| 预滚缓冲延迟或丢失致指令截断 | 1.5s 环形容量 + wake 后原子 flush；实测校准 |
| dormant 迟到 ASR/TTS 事件串扰 | 状态机 dormant 行仅受理 wake/voice_stop，其余事件转换表拒绝即忽略 |

---

*文档版本：2026-08-13 v1.8 — §16 实施校准落档：KWS 阈值 0.7→0.25（spike 实测，0.7 全量漏检）；唤醒/休眠实现抽离 `session_wake.go`（AS-COG-01 债务控制，session.go 1579→1414）；测试归口 `session_wake_test.go`（11 例含竞态压测回归）。2026-08-12 v1.7 — 追加 §16 语音唤醒与休眠设计（本地 KWS「小媛」+ dormant 七态机 + 退出词/静默休眠 + dormant 委派系统唤醒）。v1.6 — §15.4.1 补充评审落档（R10-R15）：task_id 内容匹配绑定（ExecuteTurn 阻塞、TurnResult 无 TaskID）、Registry Wire 单例双向注入、chat_only profile + CustomTools 绕行、intent_pass_enabled 默认 true 须显式写 false、委派播报 TTS 自足 flush + 正忙 FIFO 排队。v1.5 — §15 代码级评审落档（§15.4.1 R1-R9）：事件订阅三路分流修正（V2Bus 全量广播不过滤）、工具装配改 orchestrator 条件注入、前端改绑语音助手**会话**（非 agent_key）、播报触发 TurnCompleted→TaskCompleted。v1.4 — 追加 §15 语音助手前台模式设计（同级双助手：语音前台委派 + 精灵后台执行 + 完成实时播报）。v1.3 — 追加 §14 语音快速通道延迟优化（根因评审结论 + P0 已实现项 + P1/P2 待办）。v1.2 — §7.4 v3：HUD 视觉完全复刻 TwinSprite 光球（ADR-D12，替代 D11 反应堆 3D 场景；DOM 全息化/音效引擎沿用）；ADR 表 +D12。v1.1 — 追加 V6 语音听写模式设计（§13）+ voice.start 协议 mode 字段（§2.2）。*
