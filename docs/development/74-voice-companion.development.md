# M74: 语音伴侣（贾维斯桌面语音）— 开发计划

> 版本：v1.0（2026-08-05）
> **同系列**：需求 → [`74-voice-companion.md`](./74-voice-companion.md)；设计 → [`74-voice-companion.design.md`](./74-voice-companion.design.md)

---

## 1. 模块定位

语音伴侣是 Aranea 平台的**桌面语音交互形态**：科幻 HUD + 连续语音对话 + 桌面系统控制。语音只是新 I/O 形态，Chat Turn 编排内核零改动。桌面载体复用已有 Tauri 2.x 壳。

**技术栈**：Go（语音网关/Provider 适配器/工具桥）| Vue 3 + Three.js + Web Audio（HUD/语音前端）| Rust/Tauri（窗口/客户端工具执行）| 火山引擎流式 ASR/TTS（首接）

## 2. 代码锚点

### 2.1 现有复用锚点（已验证存在）

| 锚点 | 路径 | 复用方式 |
|------|------|----------|
| Tauri 桌面壳 | `web/src-tauri/`（`src/lib.rs`/`src/server.rs`/`tauri.conf.json`） | 内嵌代理 + WS 隧道（已支持二进制帧透传）+ Android 路径 + 通知插件 |
| WS 服务端 | `internal/server/ws.go`、`ws_message_handler.go` | 上行分发模式、`RunCanceller`（打断复用）、`SessionAuthorizer`（语音连接鉴权复用） |
| Chat 发送端口 | `api/kratos/chat/v1/chat.proto`（SendChatMessage）、`internal/server/ws.go` `ChatSender`/`WSTurnExecutor` | ASR 终稿入 Chat 管线 |
| 工具注册/装配 | `internal/tools/toolset.go`（Registry）、`toolset_assemble.go` | 客户端工具 ToolSet 注册 |
| 工具确认门 | `internal/agent/tool_confirm_gate.go`（catalog + session/persisted grant） | 客户端工具确认与「始终允许」 |
| 前端 WS 传输 | `web/src/realtime/ws-transport.ts` | 事件/工具桥通道 |
| 前端聊天组件族 | `web/src/components/chat/`（ChatComposer/ChatMessagePanel）、`features/chat/` | 聊天窗口复用 |
| 事件信封 | `web/src/features/chat/v2Types.ts`（V2WsEnvelope） | 工具桥下行事件形态参照 |
| 资产底座 | 27-artifact（`internal/service/artifact.go` 等） | 语音留档/截图回传 |
| 配置体系 | `configs/config.yaml`、System Settings | Speech Provider 配置注入 |

### 2.2 新增锚点（V1 后端 + 前端 ✅；客户端工具桥后端 ✅；Tauri 执行器 ✅；语音服务设置 ✅）

| 锚点 | 路径 | 说明 |
|------|------|------|
| 语音网关 | `internal/server/voice_ws.go`（+`voice_ws_test.go`） | `/v1/voice` 端点 ✅ |
| 语音会话包 | `internal/voice/`（session.go / session_state_machine.go / sentence_chunker.go / tts_scheduler.go） | 会话编排/状态机/分句/调度 ✅ |
| Speech 端口 | `internal/biz/speech.go` | StreamingASR/TTSProvider 窄接口 + 配置模型 ✅ |
| Speech 适配器 | `internal/data/speech/`（volcengine_asr.go / volcengine_tts.go / ws_conn.go / volc_frame.go / registry.go / env_config.go） | 火山首接 ✅ |
| 客户端工具桥 | `internal/tools/clientbridge/`（bridge.go / toolset.go + 测试）、`internal/server/ws_client_tool.go`、`internal/service/client_tool_bridge.go` | 工具注册/确认门接入/invoke 路由/pending 超时/离线降级/审计+流程日志 ✅（V2-T3） |
| Tauri 工具执行器 | `web/src-tauri/src/client_tools.rs`、`whitelist.rs`；前端 `web/src/services/clientTools.ts`、`web/src/realtime/ws-transport.ts`（+`useEnvelopeStream.ts` 接线） | open_app/open_url + 能力注册/invoke 分发 ✅（V2-T4）；screenshot 留待后续 |
| 前端伴侣模块 | `web/src/features/companion/`（types/voice/hud）、`web/src/stores/companion.ts`、`web/src/components/companion/`（HudCanvas/CompanionChatPanel）、`web/src/pages/CompanionPage.vue` | HUD/语音/聊天窗 ✅（V1-T7/T8）；确认卡 ✅（V2-T5） |
| 全息确认卡 | `web/src/components/companion/HoloConfirmCard.vue`、`web/src/features/companion/useCompanionConfirms.ts`、`launchParticles.ts` | 确认队列派生/全息卡渲染/粒子发射 ✅（V2-T5） |
| 语音确认拦截 | `internal/voice/confirm_words.go`、`internal/service/voice_confirm.go`、`internal/voice/session.go`（handleASRFinal 拦截） | 「好的/算了」词表 → ConfirmActivity 决议，不建 Chat Turn ✅（V2-T5） |
| 语音留档 | `internal/voice/wav.go`（EncodeWAV）、`internal/voice/archive.go`（AudioArchiver 端口）、`internal/service/voice_archive.go`（VoiceAudioArchiver）、`internal/voice/session.go`（bufferUtterance/archiveUtterance）、`internal/agent/options.go`（MergeVoiceMetaIntoUserOptionsJSON） | 语句 PCM → WAV → Artifact（audio/wav）；options_json 盖章 input_modality/asr_provider/asr_duration_ms；开关/存储失败均降级不阻断 Turn ✅（V2-T6） |
| 语音服务设置 | `internal/biz/speech_setting.go`（SpeechSetting/ApplySpeechPatch）、`internal/data/system_setting.go`（GetSpeech/UpdateSpeech）、`internal/data/speech/system_config.go`（SystemSpeechConfigReader）、`internal/data/sql/migrations/20260808_speech_columns.sql`；前端 `web/src/features/system-settings/speech.ts`、`web/src/components/settings/SpeechServiceFields.vue`、`SystemSettingsPage.vue`（speech Tab） | DB-first/env-fallback 字段级合并；凭据 write-only + mask placeholder；配置热生效 ✅（V2-T7） |
| 语音可用性探测 | `internal/server/voice_ws.go`（`GET /v1/voice/status` + VoiceStatusProbe）、`internal/biz/speech_setting.go`（SpeechASRConfigured/SpeechTTSConfigured）；前端 `web/src/features/companion/voiceStatus.ts`、`stores/companion.ts`（voiceAvailable/voiceMicDisabled）、`components/companion/HudCanvas.vue`（麦克风门控） | 未配置 ASR/TTS 时麦克风置灰 + 配置引导提示 ✅（V2-T8） |
| 客户端工具种子补播 | `internal/data/ddl_migration_registry.go`（迁移 `20261202 builtin_platform_tools_client_reseed`） | 存量库补齐 client_open_app/client_open_url ✅（V2-T8） |
| chat 主链路工具桥装配 | `internal/service/chat_orchestrator.go`（RuntimeTooling.ClientBridge）、`internal/service/chat_orch_agent_build.go`（TRPCExtensionDeps.ClientBridge）、`cmd/admin/wire.go`（provideRuntimeTooling 注入） | chat 主链路可调 client_open_app/client_open_url ✅（V2-T8） |
| 语音听写（后端） | `internal/voice/session.go`（StartParams.Mode / ModeDictation / dictation() / handleASRFinal 早退）、`internal/server/voice_ws.go`（voiceControlMessage.Mode） | mode=dictation 终稿仅下行文本，不建 Turn 不播报 ✅（V6-T1） |
| 语音听写（前端） | `web/src/features/chat/composables/useVoiceDictation.ts`（+spec）、`useChatWorkspace.ts`（装配/会话切换停止）、`components/chat/ChatComposer.vue`（听写条 + 录音态按钮）、`ChatMessagePanel.vue` / `ChatPage.vue` / `MobileChatPage.vue`（透传）、i18n zh-CN/en-US | 麦克风按钮 → 听写 → 终稿填入输入框 ✅（V6-T2） |
| HUD TwinSprite 光球 | `web/src/features/companion/hud/parts/SpriteOrbCore.ts`（value noise 光球 + Fresnel，uIntensity 扩展）、`OrbitRing.ts`（260 粒子倾斜环）、`HudPart.ts`（部件接口）、`hudParams.ts`（状态参数纯函数）、`HudScene.ts`（组合器，无后处理直渲）、`components/companion/HudCanvas.vue`（网格/扫描线/光晕舞台） | 视觉完全复刻 TwinSprite SpriteOrb/SpriteOverlay ✅（V7） |

## 3. 现状评估与差距

| 现状 | 差距 |
|------|------|
| 无任何 ASR/TTS 实现；63-tts 仅占位（停用工具种子） | SpeechProvider 端口 + 火山适配器全新建设 |
| Chat 管线成熟（流式 delta/CancelRun/确认门/事件总线） | 仅需新增「ASR 终稿 → ChatSender」与「delta → TTS」两段胶水，内核零改动 |
| Tauri 壳已具备代理/WS 隧道/Android 路径 | 缺客户端工具执行 commands、透明无边框窗口配置、热键/托盘 |
| 前端无音频采集/播放/3D 代码 | 语音前端与 HUD 全新建设；引入 `three` 依赖 |
| 工具治理完善（Registry/确认门/授权） | 仅需新增 client 工具组 + invoke/result 路由协议 |

## 4. Phase 划分与任务清单

> 状态标记：📋 待开始 ⏳ 进行中 ✅ 完成 🟡 部分完成

### Phase V1 — 语音管线 MVP（P0）

目标：桌面端说完话 < 2.5s 开始听到回复；语音对话记录与普通聊天互通。

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V1-T1 | SpeechProvider biz 端口 + 配置模型（`internal/biz/speech.go`，System Settings `speech` 分组读取，含单测） | ✅ | 端口单测通过；配置缺失返回 CodeFailedPrecondition（V1 配置读取为 env 实现，System Settings 分组归 V2-T7） |
| V1-T2 | 火山流式 ASR 适配器（`internal/data/speech/volcengine_asr.go`，含 mock 单测） | ✅ | Write/Events/Close 契约测试通过；真机联调归 V1-T10。2026-08-08 健壮性修复：Open 同步等待 full client request 首帧应答（默认 3s 超时），上游静默/协议不匹配 fail-fast 返回 UNAVAILABLE（原静默挂起至 10min 空闲回收）；error frame 解析 code/message 上抛。2026-08-08 真机校准：X-Api-Key 鉴权（`volc.bigasr.sauc.duration`）；full client request 显式 seq=1、音频帧自 2 续号（bigmodel 端点 autoAssignedSequence 严格连续）；末帧 flags=0b0010 无序号规避端点末帧绝对值约定差异；服务端末帧应答后 close 1000（reason "finish last sequence"）按流结束处理不再误报 ASR_ERROR（误报会把 thinking 态经 recoverToListening 打断）。2026-08-09 多轮对话修复：asrPump 检测事件流终结后 CAS 摘除已终结会话（`voice.asr.upstream_end` 进程日志），下一句 WriteAudio 懒重开新连接（对齐 reclaimIdleASR 模式）；两句全链路真机探针回归通过。2026-08-09 无限输入修复：SAUC `result.utterances` 为连接级累积列表，已 definite 语句在每帧响应中持续回放，`handleResponse` 原实现每帧重放首个 definite → 听写模式重复 Final 无限追加输入框；修复为 definite 游标去重（`finalCursor`，总数回退归零），产出新终稿的帧跳过 Partial 防字幕回闪；回归测试 `TestASRFinalDedupesCumulativeUtterances`（累积回放 4 帧场景） |
| V1-T3 | 火山流式 TTS 适配器（`internal/data/speech/volcengine_tts.go`，含 mock 单测） | ✅ | 分句写入 → 音频 chunk 契约测试通过；真机联调归 V1-T10。2026-08-08 真机校准：重写为 TTS V3 单向流式（`api/v3/tts/unidirectional/stream`，volc_frame 扩展 event/sessionID/errCode 字段，事件 152 合成完成/350-351 句界/352 音频）；X-Api-Key 鉴权 + `seed-tts-2.0` + `zh_female_vv_uranus_bigtts`（音色须与模型代际匹配）；合成探针（`test/voice-ack-fix/tts_synth_probe.go`）产出 10 chunks / 3.59s PCM |
| V1-T4 | `/v1/voice` 语音网关（`internal/server/voice_ws.go`：鉴权/单会话/帧路由/空闲回收） | ✅ | 二进制帧收发、鉴权拒绝、会话替换集成测试通过 |
| V1-T5 | SentenceChunker + TTS 调度器（`internal/voice/`，含分句边界/背压/取消单测） | ✅ | 分句单测覆盖标点/硬切/flush/markdown 剥离。2026-08-08 真机修复：`synthesize` 收到句级 `TTSAudioChunkEnd` 即返回——火山 152 后保持 WS 连接不关 audio 信道，原「无动作」会把 worker 饿死在本句，后续句子与 OnDrained（tts.end）全链路阻塞 |
| V1-T6 | 语音会话编排（`internal/voice/session.go`：ASR 终稿 → ChatSender；delta 订阅 → TTS） | ✅ | 端到端 mock 集成测试通过（文本进 → 音频出）；真机联调归 V1-T10 |
| V1-T7 | 前端语音采集/播放（`features/companion/voice/`：AudioWorklet 采集 16k PCM、gapless 播放调度，含 vitest） | ✅ | 采集重采样正确性单测；播放队列调度单测（pcm 11 + vad 9 + audioPlayback 8 + useVoiceSession 8 + audioCapture 覆盖于 useVoiceSession 用例；真机联调归 V1-T10） |
| V1-T8 | `/companion` 路由 + 基础 HUD 三态 + 聊天面板滑出（HudCanvas/HudScene/CompanionChatPanel/companion store） | ✅ | hudParams 纯函数 11 单测 + companion store 7 单测通过；`pnpm lint && pnpm test && pnpm build` 全绿（1284 测试）；三态动画与聊天窗收发的真机验证归 V1-T10 |
| V1-T9 | 流程日志 step 登记（`stepTitleRegistry` + 52-flow-logger §5.1 同步）+ 进程日志埋点（K1/K2/K3/K6） | ✅ | 10 条 voice.* step 已登记（含 idle_reclaim/enqueue_fail 两条进程日志 step）；Monitor Logs 可见 voice.* 步骤 |
| V1-T10 | 集成验收（真机：延迟测量、字幕准确性、播报连续性、文字聊天回归） | 📋 | NFR1 < 2.5s 实测达标；`make build && make test` 通过 |

### Phase V2 — 打断 + 客户端工具桥（P0/P1）

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V2-T1 | 语音状态机显式化 + barge-in 链路（前端 VAD → 本地停播 → voice.barge_in → CancelRun → TTS 终止 → 前端清队列） | 🟡 | 后端状态机/转换表单测 + Cancel 链路 V1 已落地；2026-08-07 前端 VAD 接线完成（decideVadAction 纯函数 14 单测 + onPcm16k 喂帧 → bargeIn/commit，本地 50ms 淡出停播）；打断 ≤300ms 停播实测待真机（同 V1-T10 凭据依赖） |
| V2-T2 | AEC 验证与调优（echoCancellation 在扬声器场景实测，误打断率评估） | 📋 | 播报中无误触发打断 |
| V2-T3 | 客户端工具桥后端（`internal/tools/clientbridge/`：注册/种子/确认门接入/invoke 路由/pending 超时/离线降级，含单测） | ✅ | 2026-08-08 完成：bridge.go（pending 30s 超时/离线 DESKTOP_CLIENT_OFFLINE/审计+流程日志）+ toolset.go（client 工具组 open_app/open_url，确认门 requiresConfirm）；WS 侧 ws_client_tool.go（register_capabilities/client_tool.result 上行 + RouteClientToolInvoke 按 desktop_companion 能力过滤扇出）；service 适配 client_tool_bridge.go（MonitorAuditRepo→AuditRecorder）+ wire 注入；流程日志 client_tool.invoke/result/timeout 三 step 登记。路由/超时/离线/回环单测全绿（clientbridge + server 包） |
| V2-T4 | Tauri 工具执行器（`client_tools.rs`/`whitelist.rs`：open_app/open_url，白名单 Rust 侧强制） | ✅ | 2026-08-08 完成：whitelist.rs（内置默认 ∪ 用户覆盖 whitelist.json；别名归一化大小写不敏感；Windows 环境变量展开；裸绝对路径一律拒绝——路径注入防护）+ client_tools.rs（open_app：Win `cmd /C start` 裸名走 App Paths / 绝对路径直启，macOS `open -a`，Linux 直启；open_url：Win `rundll32 url.dll,FileProtocolHandler` 避免 `&` 重解析，macOS `open`，Linux `xdg-open`；URL 校验仅 http/https、≤2048 字符、无空白控制字符；Android → UNSUPPORTED_CAPABILITY；错误码 NOT_WHITELISTED/TARGET_NOT_FOUND/INVALID_URL/SPAWN_FAILED）。前端：clientTools.ts（Tauri executor + client_tool.result 帧构造）+ ws-transport（register_capabilities/client_tool.invoke 分发）+ useEnvelopeStream 接线（连接后声明 desktop_companion 能力）。cargo test 33 绿 + vitest 19 绿 + pnpm lint 0 errors；screenshot 范围外（留待后续），真机启动验收归 V2-T8 |
| V2-T5 | 全息确认卡 + 科幻开启动画（HoloConfirmCard + 粒子发射；语音确认「好的」映射 confirm 通过） | ✅ | 2026-08-08 完成：①后端语音确认拦截——confirm_words.go（归一化整句精确匹配 approve 19 词/deny 12 词，误命中代价仅一次 resolver 查询）+ service/voice_confirm.go（spirit 树+精确 session 两路收集 kind=confirm+tool_blocked，取最早 StartedAt，复用 ConfirmActivity 全量校验）+ session.go handleASRFinal 拦截（resolved 时不建 Chat Turn，下行 confirm.resolved 帧；resolver 故障降级普通语句）；voice.confirm.resolved step 登记+52-flow-logger 同步。②前端——HoloConfirmCard（全息扫描线/倒计时/确认/取消/始终允许三路径）+ useCompanionConfirms（activityV2Store 派生确认队列，单一数据源）+ launchParticles（makeBurstParticles 纯函数可注入 RNG + spawnLaunchBurst DOM 发射）+ CompanionPage DECISION_REPLY 映射 approveAlways 走既有 confirmActivityGrant API。③HUD 科幻增强——hudParams 新增 vibrationGain/arcSpeedFactor/coreWobble/rippleGain 四状态驱动参数；HudScene 新增全息弧线组/粒子环双频声波震动/能量核顶点摆动/声波涟漪。验证：go build+voice/server/service 测试全绿（独立 GOCACHE）；vitest 195 文件 1462 测试全绿；pnpm lint 0 errors。真机三路径交互+语音确认端到端归 V2-T8 |
| V2-T6 | 语音留档（archive_user_audio 开关 + Artifact 附件 + 消息 metadata 标记） | ✅ | 2026-08-08 完成：①biz 端口——`SpeechConfigReader.ArchiveUserAudio`（env 实现读 `SPEECH_ARCHIVE_USER_AUDIO`，非法值按 false）+ `TurnInput.Voice *VoiceTurnMeta`（ASRProvider/DurationMs/Archive）。②voice 包——listening 态按语句缓冲 PCM（8 MiB 上限截断 Warn 一次）；终稿取出缓冲 → `EncodeWAV`（16k s16le mono 头封装，wav_test 覆盖）→ `AudioArchiver` 端口；确认拦截/cancel/stop 丢弃缓冲。③service 实现——`VoiceAudioArchiver` 开关关闭/读取失败/存错三类降级均不阻断 Turn；文件名时间戳+原子序号防版本堆叠。④盖章——`prepareTurnUserOptions` 调 `MergeVoiceMetaIntoUserOptionsJSON` 写 input_modality/asr_provider/asr_duration_ms，留档 Ref 并入 options_json.attachments（刻意绕开 LLM 附件链路，防能力拒绝+字节注入）。⑤装配——wire 注入 ArtifactUsecase 构造 archiver；newASR 透传 `Driver`（asr_provider 数据源）；VoiceWSServer 新增 archiver 参数。验证：独立 GOCACHE 下 wire 重生成 + go build 全绿；voice/server/service/biz/data-speech 测试全绿（含 archiver 6 单测 + session 留档用例）；araneactl lint 0 违规；voice.archive.saved/degraded/truncate 三 step 登记 + 52-flow-logger 同步。真机留档回放验收归 V2-T8 |
| V2-T7 | System Settings「语音服务」管理面 Tab（音色/语言/凭据，敏感字段） | ✅ | 2026-08-08 完成：①存储——`system_settings` 单例 14 离散列（DDL 迁移 `20260808_speech_columns.sql`，raw SQL，同 planner_model_columns 模式）；string 空串/speed_ratio=0/archive NULL 均表「未设置」。②biz——`SpeechSetting` + `ApplySpeechPatch`（空字段保留、凭据仅非空+updateXxxCred 才替换、三态开关）；`SystemSettingRepo.GetSpeech/UpdateSpeech`；speed_ratio<0 拒绝。③读取——`SystemSpeechConfigReader`（DB-first/env-fallback 字段级合并，读取失败回退 env Warn 降级 K3，每次调用实时读 DB 热生效）；wire 绑定由 Env 切换为 System 实现。④API——`SystemSettings.speech` 消息 + Update 字段 22-35；凭据 write-only（仅回 has_api_key/configured），optional bool 传三态。⑤前端——「语音服务」Tab（SpeechServiceFields.vue + speech.ts 差分 patch，凭据 mask placeholder，i18n zh/en）。验证：独立 GOCACHE 下 go build + biz/data(PG)/data-speech/service/voice/server 测试全绿 + vet 0；vitest 201 文件 1521 用例全绿；pnpm lint 0 errors（i18n baseline 持平 3767）+ build 通过；araneactl lint 0 违规。真机「改配置→新语音会话生效」验收归 V2-T8。2026-08-08 增补 X-Api-Key 凭据双模式：proto 字段 36-37（write-only）+ DDL `20260809_speech_api_key_columns.sql` + `SpeechCredOK`（api_key 单 key 或 legacy AppKey+AccessKey 对，api_key 非空优先，`setVolcAuthHeader` 统一选路） |
| V2-T8 | 集成验收（语音打开微信全流程、离线降级提示、留档回放） | 🟡 | 2026-08-08 服务端链路验收通过（详见 `docs/testing/reports/acceptance-2026-08-08-voice-companion-v2t8.md`）：①验收中发现三处装配缺口并修复——存量库 client 工具种子补播（迁移 20261202）、未配置语音服务时麦克风置灰（`/v1/voice/status` 探测 + voiceAvailable 门控）、chat 主链路 ClientBridge 装配（RuntimeTooling→wire→TRPCExtensionDeps 注入链）；②修复后「打开微信」全链路实测通过——agent 直调 `client_open_app`（不再 tool not found/不再误用 exec_command）→ 触发 tool_blocked 确认门 → approve 续跑 → 桌面端离线返回 `DESKTOP_CLIENT_OFFLINE` → agent 如实告知「桌面客户端未连接」（符合需求 §2.5 降级条款）；③`/v1/voice/status` 实测 `{asr:true, tts:false}` 门控正确。**桌面端在线补测（同日晚）**：④发现第四处缺口——桌面端 invoke 被 ACL 拒绝（`Command client_open_app not allowed by ACL`），根因 Tauri v2 app 命令权限需 `build.rs` 声明 `AppManifest::commands()` 才会自动生成；修复（build.rs 声明 + capabilities 增补 `allow-client-open-app`/`allow-client-open-url`）并重打包后真机全链路通过——确认卡弹出 → 「允许本次」→ 桌面端白名单解析 → 实际启动 `Weixin.exe`（后端日志 `launched D:\Program Files (x86)\Tencent\Weixin\Weixin.exe`）→ agent 如实回复成功（截图 `test/v2t8-desktop/11/12`）。仍待真机项（语音确认「好的」端到端、HUD 语音播报、语音留档回放、打断实测）：同日晚 WS 直连实测语音留档，修正 ASR endpoint 配置错误（v2 `/api/v2/asr` 与 SAUC v3 协议不匹配 → 改 `/api/v3/sauc/bigmodel` 热生效），但 v3 握手被拒确认 DB 凭据无效（`ASR_UNAVAILABLE: bad handshake`），语音项全部阻塞于有效火山 ASR/TTS 凭据，同 V1-T10。**2026-08-09 凭据到位后全链路复测通过**：火山 X-Api-Key（api-key-20260808215540）经 `PUT /v1/system-settings` 写入（ASR `volc.bigasr.sauc.duration` / TTS `seed-tts-2.0` + `zh_female_vv_uranus_bigtts`），`/v1/voice/status` 双 true；修复两个真机 bug（ASR close 1000 误报、TTS 调度器 End 饿死，见 V1-T2/V1-T5）后全链路探针（`test/voice-ack-fix/voice_chain_probe.go`）通过——voice.start → 真实中文语音（TTS 合成产物转 s16le 作 ASR 输入）→ asr.partial/final 终稿准确 → turn.accepted → tts.start → 12.16s 音频下行 → tts.end；进程日志确认 TTS worker 正常 drain 退出（2.5s） |

### Phase V3 — 深度控制 + 体验打磨（P2）

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V3-T1 | client_screenshot（截图 → Artifact → 图片理解链路） | 📋 | 截图回传对话可问答 |
| V3-T2 | client_system_control / client_file_read（授权目录） | 📋 | 确认门强制；越权拒绝 |
| V3-T3 | 透明无边框置顶窗口 + 迷你球形态 + 全局热键 + 托盘 | 📋 | 形态切换与热键可用 |
| V3-T4 | 移动端适配（Android 布局/权限/降级） | 📋 | 需求 §2.9 验收标准 |
| V3-T5 | HUD 性能调优 + 音色/语速配置 UI + 通知联动 | 📋 | NFR5 ≥40fps |

### Phase V4 — 音频附件理解（P2，与 59 号共建）

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V4-T1 | ASR Provider 复用到 59 附件链路（音频附件 → STT → 注入；失败降级） | 📋 | 59-multimodal §2.3 验收标准；同步更新 59 三件套状态 |

### Phase V5 — HUD 科幻重构（反应堆形态 + Bloom + 音效，P1）

目标：初版 HUD 视觉不达标，按 ADR-D11「方舟反应堆 · 混合式」重构（设计 §7.4 v2）：液态 simplex 能量核 + 同心刻度仪表环 + UnrealBloomPass 辉光 + DOM 全息化 + 程序合成音效。`AvatarRenderer` 接口与 hudParams 纯函数架构保持不变。

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V5-T1 | 渲染管线升级：EffectComposer + UnrealBloomPass + OutputPass 接入（three/examples/jsm，零新依赖），Bloom 参数入 hudParams | ✅ | hudParams `bloomIntensity` 六态纯函数单测（listening 1.0 / speaking 1.1+振幅增益 / interrupted 1.3 等）；`HudScene` 组合器接入三 Pass，flash/burst 瞬时提亮叠加 |
| V5-T2 | 场景重构：HudScene 拆分为组合器 + `hud/parts/*`（ReactorCore simplex 液态核 / ReactorRings ×3 刻度环 / Starfield / SpectrumRing 迁移 / ShockwavePool 迁移），移除线框壳与全息弧线组 | ✅ | parts 统一 `HudPart` 接口（update/setTint/dispose），HudScene 235 行 ≤500；`ringExpand`/`ringSpeedFactor`/`coreWobble`/`rippleGain` 纯函数单测全绿 |
| V5-T3 | 启动过场：语音模式开启 ~1.2s 序列（bootProgress 驱动核心点亮 + 刻度环逐层展开 + boot 音效联动） | ✅ | `bootProgress` 1.2s 推进（BOOT_SECONDS），核心 0.75→1 / Bloom 0.35→1 / 环组 0.8→1；`ringBoot [0,1]×3` 交错纯函数（内环 0→0.5、中环 0.25→0.75、外环 0.5→1.0），ReactorRings 逐环透明度+缩放弹入（待机保留 20% 幽影）；快速关闭立即回待机（bootProgress=0）；boot 音效经 `useUiSounds` voiceModeOn watch 联动；hudParams.spec 25 例全绿 |
| V5-T4 | DOM 全息化：状态标签（角括号+扫描线）/ 打字机字幕 / 反应堆式麦克风按钮 / HoloConfirmCard 视觉对齐 | ✅ | 状态标签：四角括号 + hud-scan 扫描线 + 等宽大写 + 发光描边；字幕：逐字 span 打字机浮现（索引复用旧字不重放）+ 全息边框发光；麦克风：hud-mic-pulse 脉动光环常开 + 开启时 hud-mic-orbit 虚线轨道环旋转（置灰隐藏光环）；HoloConfirmCard 沿用同色系 rgba(0,229,255) 全息语言无需改动 |
| V5-T5 | 音效引擎 `audio/uiSounds.ts`：boot/chirp/ping/ding/buzz/cut 程序合成 + localStorage 开关 + 状态机联动 | ✅ | `SOUND_SPECS` 纯数据六配方（boot 上电扫频 180→880Hz 双段等）；`UiSoundEngine` 注入 `AudioContextLike` 可 mock，主音量 0.5（约 -18dB），浏览器自动播放策略 resume 钩子；`useUiSounds` 状态机联动（boot/chirp/ping 循环/cut）+ 确认 ding/buzz 页面接线；localStorage 开关默认开 + CompanionPage 音量切换按钮；uiSounds.spec 9 例全绿 |
| V5-T6 | 总验收：`pnpm lint && pnpm test && pnpm build` 全绿 + 五状态/过场/音效浏览器实测 + 三件套同步 | ✅ | 2026-08-09：`pnpm lint` 0 errors（warnings 均为存量基线）；`pnpm test` 204 文件 1560 测试全绿；`pnpm build` 通过。浏览器实测（localhost:9001/#/companion）：idle 青色反应堆+刻度环+星空渲染 ✅ → 点麦克风 listening（ cyan +「正在聆听…」全息角括号标签）✅ → 环境音 ASR 终稿「你好。」→ thinking 金色高亮 Bloom ✅ → TTS 播报 → speaking → listening 回落 ✅ → 聊天面板自动滑出完整 Turn（记忆召回+回复）✅ → 再点关闭回 idle ✅；音效开关按钮（「关闭科幻音效」）渲染且默认开 ✅；全程控制台零错误（仅 THREE.Clock 弃用警告，存量）。interrupted/error 态由状态机单测覆盖。同日修复听写无限输入根因（ASR 累积回放去重，见 V1-T2），真机探针 `test/dictation-dupcheck/` 验证两句连续听写恰好 2 条 asr.final 无重复 |

### Phase V5.1 — 播报无声根因修复 + HUD 反馈修订（2026-08-09）✅

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V5.1-T1 | **TTS 播报无声根因修复**：`createVoiceSessionClient.connect()` 显式 `socket.binaryType = 'arraybuffer'`——浏览器默认 `'blob'` 导致 onmessage 收到 Blob，`instanceof ArrayBuffer` 判空后全部 TTS 二进制帧静默丢弃（ASR/JSON 控制帧正常故仅音频无声；后端日志佐证：Turn 完成 675 字回复 + TTS worker 运行 15s 无 sentence_skip） | ✅ | useVoiceSession.spec 回归例「sets binaryType=arraybuffer…」RED→GREEN，10/10 通过 |
| V5.1-T2 | **thinking 去琥珀**：`TINT_AMBER` 删除，idle/listening/thinking/speaking 统一青蓝系（`#22d3ee→#34d399`），thinking 仅靠转速 ×3/收缩 0.85×/高波动区分；interrupted/error 保留红色警示 | ✅ | hudParams.spec tint 例改写（四态青蓝 + 红警示），26/26 通过 |
| V5.1-T3 | **speaking 粒子 + 震动**：新增 `parts/EnergyParticles.ts`（220 粒子绕核轨道云，`particleGain` 六态增益驱动亮度/轨道速度/尺寸，speaking 振幅外推粒子，vertexColors 内外层双色，帧间零分配）；进入 speaking 自动 `burst()`（涟漪 + Bloom 提亮）；相机按 `shakeGain × level` 高频微抖 | ✅ | `particleGain`/`shakeGain` 纯函数入 hudParams + 单测 2 例；全量 `pnpm test` 204 文件 1562 例全绿、`pnpm lint` 0 errors（存量 warning 基线不变）、`pnpm build` 通过 |

> V5.1 顺带发现（已由 V5.2 修复，见下）：① 03:28 会话 `TURN_FAILED [CHAT_TURN_BUSY/CONFLICT]`——语音 Turn 与进行中 Turn 冲突时整轮无回复无播报（前端仅有 voice.error 提示）；② 03:11 `voice.tts.sentence_skip`——分句器产出无可读文本的句子被火山 TTS 拒绝（`45002001 No readable text!`）后跳过，造成该句无声。

### Phase V5.2 — 语音 Turn busy 重试 + 纯标点句过滤（2026-08-09）✅

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V5.2-T1 | **CHAT_TURN_BUSY 整轮无回复修复**：`internal/voice/session.go` 新增 `executeTurnWithBusyRetry`（3 次 × 250ms 退避，镜像 channel 入口 `runNativeTurnWithBusyRetry` 策略）+ `isTurnBusyError` 判定（voice 不反向依赖 service，领域字符串对齐 `turn_outcome.go`）；ASR 终稿派发改走重试——busy 竞态重试后准入重查，`AllowQueue=true` 转排队队列，回复 delta 经事件总线正常进 TTS；耗尽才 `handleTurnFailure`。首次重试记 `voice.turn.busy_retry` 进程日志（K4） | ✅ | TDD：session_test 新增 4 例（isTurnBusyError 谓词 6 断言 / 2 次 busy 后成功无 voice.error / 恒 busy 恰好 3 次后 TURN_FAILED / 非 busy 错误不重试快速失败）RED→GREEN；`go test ./internal/voice/ ./internal/server/ -count=1` 与 `-race` 全绿 |
| V5.2-T2 | **纯标点句 TTS 跳句修复**：`sentence_chunker.go` 新增 `speakable()`（unicode 字母/数字，覆盖 CJK），`cut`/`hardCut` 送出前过滤无可读字符句——不再触发火山 `45002001` 计入连续失败（原连续 3 次中止调度器致后续句全无声）；flush 语义由会话层空文本哨兵兜底（tts.end 不缺失） | ✅ | TDD：chunker_test 新增 3 例（纯标点 Flush 丢弃 / 前导标点+可读文本整句保留 / 纯标点 hardCut 丢弃）RED→GREEN；voice 包全量 + `-race` 全绿；`go build ./cmd/... ./internal/... ./pkg/...` 干净；新二进制已部署 :8000 供实测 |

> **V5.2 运行时验证（2026-08-09）**：新二进制 :8000 + 前端 :9001 起服后，真机探针 `voice_chain_probe.go` 两句连说全链路通过——turn1/turn2 均 asr.final 准确（"你好，请用一句话介绍你自己。"）→ turn.accepted → tts.start → ~9.8s 音频下行 → tts.end → 回 listening，无 voice.error；后端日志窗口无 `busy_retry`/`sentence_skip`/`TURN_FAILED`。Companion 页浏览器实测 HUD 正常渲染（青蓝反应堆 + 刻度环 + 粒子云，麦克风未置灰，0 console errors）。
>
> **V5.2 遗留（另行评估）**：①排队 Turn 静默风险——voice 回 listening 后，若此前派发的 Turn 经 busy 重试/准入转入排队队列，其执行时的 delta 会被 `feedDelta` 门禁（`pendingTurns>0 && thinking/speaking`）丢弃，需 FSM 补边（listening + pendingTurns>0 时 delta 唤醒 thinking）且 `pendingTurns` 计数不区分语音/文字 Turn（同 session 文字 run 的 TurnCompletedEvent 会误扣）；②speaking/thinking 态收到新 ASR 终稿被 FSM 非法转换静默忽略（V1 语义：播报中插话请先打断/取消）——是否改排队为产品决策。真机 busy 竞态窗口为亚秒级，日常连说两句主要命中路径为「前一轮收尾完成后直接执行」，已由探针覆盖。

### Phase V5.3 — 澄清口播 + TTS 挂句超时 + 搜索源实测 + 取消竞态静音（2026-08-09）✅

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V5.3-F1 | **澄清问题语音播报**：澄清门触发（`StepCreatedEvent kind=clarify`）时语音侧原本完全静默（澄清卡片只上 UI，无文本 delta，turn 挂起）。`internal/voice/session.go` 新增 `maybeBroadcastClarification`：解析 `ClarificationEnvelope` → `clarificationSpeech` 渲染口播文本（引导语 + 逐题题干/可选项 + 作答引导，中文序号「第 N 个」「两」）→ 喂 SentenceChunker 走既有 TTS；仅 listening/thinking/speaking 态播报；用户语音作答经自由文本澄清路径续跑（`resolveClarificationFreeText`，与 WS 文字消息同入口） | ✅ | TDD：session_test 新增澄清播报用例（kind=clarify 触发口播 / 非 clarify 不触发 / 非法信封 Warn 跳过）；step `voice.clarify.broadcast` 已登记 `flow_log.go`；`go test ./internal/voice/` 全绿 |
| V5.3-F2 | **TTS 句子级空闲超时**：挂死句（火山 End 帧永不到达/连接半死）曾把 worker 饿死在单句上，后续句与 `tts.end` 全部阻塞（2026-08-09 天气 Turn 实测挂 3 分钟）。`tts_scheduler.go` 新增 `ttsSentenceTimeout = 15s` 空闲计时——任一 chunk（Data/End/Error）到达即重置，超时放弃本句返回 `errTTSSentenceTimeout`，worker 继续后续句，`OnDrained` 必达 | ✅ | TDD：`TestSchedulerSentenceTimeout`（缩短包级变量验证超时返回与 worker 续跑）RED→GREEN；voice 包全量 + `-race` 全绿 |
| V5.3-F3 | **WebSearch 源实测选型**：探针 `test/websearch-probe/`（一次性，可整目录删除）实测直连/代理/换源——**Bing 中国（cn.bing.com）直连最优**：195ms/英文 8 条、312ms/中文 10 条真实结果，无需代理；DDG API 走代理默认 UA（`trpc-agent-go-duckduckgo/1.0`）被风控返回 200 空体（换 curl UA 才有 202 数据），直连被墙；百度反爬返回 1KB 验证页。当前 `toolset.go` 的 `NewTool()` 无参构造在本环境必然空响应；切 Bing 需新写 HTML 解析工具（无官方 API），是否实施待定 | ✅ | 探针输出候选源状态/时延/结果量对照表；深挖探针确认 UA 风控行为（trpc UA 空体 vs curl UA 有数据） |
| V5.3-F4 | **取消竞态日志静音**：`cancelActiveRun` 已将 run 置 cancelled（终态）后，EXECUTE/PERSIST 成功路径仍走 `postProcessTurn`——`SetRunStatus("completed")` 被 FSM 拒绝产生 Warn、Session 状态从 interrupted 翻转 completed、after_turn 钩子误触发、「对话轮次执行完成」流程日志噪音。修复：`postProcessTurn` 在用量记账后加 `runWasCancelled` 守卫——已取消则跳过 completed 状态发布/Session 翻转/revision bump/after_turn 钩子，`LogSkip` 替代 `LogDone`；用量记账保留（助手消息已落库，token 真实消耗） | ✅ | TDD：`chat_orchestrator_turn_postprocess_test.go` 新增 2 例（已取消跳过全部完成动作且记账保留 / 未取消对照组照常）RED→GREEN；service 全包测试仅余 models.dev 网络恒败 2 例（环境受限，与改动无关） |

> **V5.3 验证与部署（2026-08-09）**：独立 GOCACHE 下 `go build ./cmd/... ./internal/... ./pkg/...` 干净；`go test ./internal/voice/ ./internal/biz/ ./internal/event/ ./internal/agent/intent/` 全绿；`internal/service` 全包仅 `TestModelCatalogService_SyncModelCatalog_*` ×2 恒败（models.dev 须代理，环境受限已知项）；go vet 0 问题；改动文件 gofmt 干净。新二进制已部署 :8000（healthz 200）。注：本批次期间共享 GOCACHE 再现幻影编译错误（`decision.AutoResolved undefined` 实际已定义），按既有教训独立缓存绕过。
>
> **V5.3 遗留**：F3 结论待决策——web_search 工具是否从 DDG 切换到 Bing 中国（需新写 HTML 解析实现）；切换前语音/文字 Turn 的 web_search 调用在本环境将持续空响应。

### Phase V6 — 语音听写（聊天页语音输入，P1）✅ 2026-08-09

目标：聊天页麦克风按钮落地「听写」行为（需求 §2.11）——语音转文字填入输入框，手动编辑后发送；不自动发送、不触发 TTS 播报。复用 `/v1/voice` 通道，新增 `mode=dictation` 会话模式（设计 §13）。

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V6-T1 | 后端听写模式：`StartParams.Mode`/`ModeDictation`、Start 跳过事件总线订阅、不缓冲留档、handleASRFinal 终稿仅下行文本；voice_ws 透传 mode | ✅ | session_test.go 4 例（仅下行文本/跳过确认拦截/跳过留档/连续终稿）；`go test ./internal/voice/... ./internal/server/...` 全过 |
| V6-T2 | 前端听写：useVoiceDictation composable + useChatWorkspace 装配 + ChatComposer 听写条/录音态按钮 + i18n + 会话切换自动停止 | ✅ | useVoiceDictation.spec 17 例 + useVoiceSession.spec mode 断言；`pnpm lint && pnpm test && pnpm build` 全绿（204 文件 1556 测试） |
| V6-T3 | 真机验证 + 三件套同步 | ✅ | 探针 `test/dictation-e2e/dictation_probe.go`：真实火山 ASR 两句连续听写，无 turn.accepted/无 TTS/状态停留 listening；浏览器点麦克风 → 后端 `voice.session.start mode=dictation` → 听写条「正在聆听…」+ 按钮「停止听写」→ 再点 teardown（后端 `voice.session.done`），消息区无消息 |

> V6-T3 顺带修复：HEAD 上 `HudCanvas.vue` 引用已不存在的 `createHudScene` 工厂致 SPA 白屏（V5 重构未对齐），改为 `new HudScene(canvas, {getPlaybackLevel, fillMicSpectrum})` 拉取模型并移除失效的 amplitude/spectrum watch；修复后 hudParams.spec 21 例与全量前端测试通过。

### Phase V7 — HUD 视觉完全复刻 TwinSprite 光球（2026-08-09）✅

目标：用户指定以既有 TwinSprite 项目（`F:\TwinSprite`）为视觉准绳——HUD 3D 场景完全复刻其 SpriteOrb（value noise 光球 + 单粒子轨道环 + CSS 光晕 + 网格/扫描线舞台），移除 V5 反应堆部件族与 Bloom 后处理（设计 §7.4 v3，ADR-D12）。保留本产品增量：boot 点亮过场、speaking 相机微抖、burst 脉冲、打断红闪、thinking 无变色约束、DOM 全息化与音效引擎。

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V7-T1 | 3D 场景复刻：新增 `hud/parts/SpriteOrbCore.ts`（TwinSprite shader/几何/配色原值 + `uIntensity` 调光扩展）与 `hud/parts/OrbitRing.ts`（`buildRing` 原值 260 粒子环）+ `HudPart` 接口；删除 ReactorCore/ReactorRings/Starfield/SpectrumRing/ShockwavePool/EnergyParticles 六部件与 BloomComposer；`HudScene` 移除 EffectComposer 直渲（相机 fov 45 / z 5.2、电平平滑 0.2/帧→dt×12/s） | ✅ | hudParams.spec 改写 11 例锁定 TwinSprite 原值（uAmp 0.12+level×0.38、thinking 噪声/环速 ×3 + 收缩 0.85×、配色 `#123a6e→#4dd8e8`、红警示、boot intensity 0.35→1 曲线） |
| V7-T2 | 舞台与光晕：`HudCanvas.vue` 新增网格（accent 6% / 48px / radial mask 30%→75%）+ 垂直扫描线（accent 45% / 7s / opacity 0.5）+ CSS 呼吸光晕（accent 22%→65%，3.2s；语音开 0.55↔1 / 待机 0.25↔0.55）；listening 电平改 TwinSprite `sampleMic` 公式（bins 2..48 均值 /255 ×1.6 钳制）经 `getMicLevel` 拉取注入 | ✅ | `pnpm lint` 0 错误、`pnpm test` 205 文件 1562 用例全绿、`pnpm build` 成功 |
| V7-T3 | 真机验证 + 三件套同步 | ✅ | 浏览器实测（:9001 dev + :8000 后端在线）：待机微光 → 点麦克风 boot 点亮满功率 →「思考中…」（核心收缩/高速噪声）→「正在播报…」（电平强震 + 相机微抖 + burst）→ 退出语音模式干净回待机；网格/扫描线/光晕与 TwinSprite 原值一致；语音全链路（ASR→Chat→TTS 天气问答）在线 |

> V7 版本编号说明：V6 已被「语音听写」占用（Phase V6），HUD TwinSprite 复刻顺延为 V7；代码注释同步标注「V7 TwinSprite 复刻」。

### Phase V8 — 语音快速通道延迟优化 P0（2026-08-10）✅

目标 SLO：停口（ASR 终稿）→ 首帧音频 ≤ ~2s。根因评审结论：耗时全部来自项目设计（思考默认开启 + 串行编排 + 冷启动），非 LLM 本身慢（设计 §14.1）。路线约束：语音优化收敛于 voice 包 + 请求级 ctx 标记，文字输入路径零行为变化（全局项仅纯提速）。

| # | 任务 | 状态 | 验收 |
|---|------|------|------|
| V8-T1 | **意图识别强制关思考**（全局）：`intent/pass.go` callsite 强制 `ThinkingDisabled=true`（分类任务思考段无收益，实测 3.7-26.6s 纯延迟），不依赖 catalog 行配置 | ✅ | `pass_thinking_test.go` 断言请求体 thinking disabled；`go test ./internal/agent/intent/` 全绿 |
| V8-T2 | **主 LLM 思考开关读取 catalog**（全局）：`trpc_build.go` 从 catalog `thinking_disabled` 注入 `GenerationConfig.ThinkingEnabled`，解除硬编码 `Stream=true` 不读配置 | ✅ | `go build ./...` 干净；llmcompat 单测全绿 |
| V8-T3 | **BUILD + Intent Pass + recall 三方并行**（全局）：`chat_orchestrator_turn.go` errgroup 化；recall 从串行段移入并行（语音轮次 hits 恒 0，纯开销 0.3-3.3s）；`WithProactiveHits` Wait 后注入语义不变；AwaitHook Wait 后重绑 turn ctx（防 errgroup 派生 ctx 提前取消致 await 秒败） | ✅ | service 全包测试通过（仅 models.dev 网络恒败 2 例，环境受限）；`chat.build_intent_parallel` 计时日志 |
| V8-T4 | **Voice Fast-Path**（仅语音）：`internal/agent/voice_fastpath.go` ctx 标记 + BeforeModel hook（LayerDynamic priority 4）per-request 关主 LLM thinking；`prepareRunContext` 仅 `input.Voice != nil` 打标（唯一赋值点 voice/session.go），文字路径透传 | ✅ | `voice_fastpath_test.go` 4 例（打标关思考/无标记透传/空 args 安全/hook 注册）；agent 包全绿 |
| V8-T5 | **E：首音频延迟预算测量**（仅语音）：`session.go` T0=ASR 终稿派发、首帧下行消费（每 Turn 一次）、打断复位；预算 2.5s 超预算 Warn（K2）+ `voice.tts.start` 流程日志带 `first_audio_ms` | ✅ | `session_prewarm_test.go` T0 记录/消费/打断复位用例；voice 包全绿（含 -race） |
| V8-T6 | **C1：voice.start 预热构建缓存**（仅语音）：`chat_voice_prewarm.go` VoiceTurnPrewarmer 与 Turn 同源解析 dialogMode/provider/model 填充 BuildTRPCAgentCached（实测 cache miss 2.6-2.7s 移至开麦空窗）；非阻断容错、dictation/团队会话跳过；voice_ws setter + wire 接线 | ✅ | `chat_voice_prewarm_test.go`（nil 安全/会话缺失/团队跳过）+ `session_prewarm_test.go` 触发与 dictation 跳过用例；service/voice/server 全绿 |
| V8-T7 | 全量验证 + 三件套同步 | ✅ | 2026-08-10：`go build ./...` 干净；voice/agent/server/service 测试全绿（仅 3 例 models.dev 网络恒败，环境受限已知项）；`make lint` 0 违规 + fmtcheck OK；设计 §14 落档 |

> **V8 P1/P2 待办**（设计 §14.4）：首句快速通道（firstSentenceMinRunes 6→4）、C2 ASR partial 投机意图（500ms 稳定 + hash 复用）、D 工具过渡句 filler（P2）、L5 TTS 连接预热（P2）。

## 5. 总验收标准

1. 需求文档 §3 验收总览 13 项按 Phase 逐项达标
2. 每 Phase 完成后：后端 `make api && make wire && make build && make test && make lint`；前端 `pnpm lint && pnpm test && pnpm build` 全绿
3. 运行时验证（R3）：读 `logs/aranea-pipeline.log` 确认 voice.* 流程日志 + 实际桌面端操作验证
4. 回归：Chat/Team/工具治理既有单测不破坏
5. V7 视觉：HUD 四态（待机/boot/思考/播报）与 TwinSprite SpriteOrb 视觉一致（网格/扫描线/光晕/光球/粒子环原值）；`hudParams.spec` 11 例锁定 TwinSprite 原值常量

## 6. 改动文件清单（预估）

**后端新增**：`internal/server/voice_ws.go`、`internal/voice/*`（4 文件）、`internal/biz/speech.go`、`internal/data/speech/*`（3 文件）、`internal/tools/clientbridge/*`
**后端修改**：`internal/server/ws.go`（工具桥下行挂载）、`internal/tools/toolset.go`（注册）、`internal/data/builtin_tools_seed.go`（client 工具组种子）、`internal/agent/tool_confirm_gate.go`（catalog）、`internal/event/flow_log.go`（step 登记）、`cmd/admin/wire*.go`（注入）、System Settings 相关（speech 分组）
**前端新增**：`web/src/pages/CompanionPage.vue`、`web/src/features/companion/*`（~10 文件）、`web/src/features/chat/composables/useVoiceDictation.ts`（+spec，V6）、`web/src/features/companion/hud/parts/`（SpriteOrbCore/OrbitRing/HudPart，V7）
**前端修改**：`web/package.json`（+three）、路由注册、System Settings 页（语音 Tab，V2）、`useChatWorkspace.ts` / `ChatComposer.vue` / `ChatMessagePanel.vue` / `ChatPage.vue` / `MobileChatPage.vue` / i18n locales（V6 听写）、`components/companion/HudCanvas.vue`（V7 网格/扫描线/光晕舞台）、`features/companion/hud/HudScene.ts` 与 `hudParams.ts`（V7 TwinSprite 复刻重写；V5 反应堆六部件 + BloomComposer 已删除）
**Tauri 新增**：`web/src-tauri/src/client_tools.rs`、`whitelist.rs`；**修改**：`Cargo.toml`（+screenshots 等，V3）、`tauri.conf.json`（窗口/权限，V3）
**文档**：本三件套 + `65-module-cross-reference-full.md`（新增模块卡片）+ 59/63 三件套状态联动（V4/V2 期）

## 7. 风险与对策

| 风险 | 对策 |
|------|------|
| ~~火山 ASR/TTS 账号/凭据未就绪阻塞 V1~~（2026-08-09 解除：X-Api-Key 到位，全链路真机通过） | V1-T1 端口先行，适配器 mock 可测；凭据到位即真机联调 |
| 打断误触发（扬声器回声） | 前端 AEC + 人声持续阈值 + 服务端 VAD 双保险（V2-T2 专项实测） |
| 客户端工具安全面 | 白名单 Rust 侧强制 + 确认门 + 授权目录 + 审计日志；高危 file 操作 V3 才开放 |
| WebView2 透明窗口兼容性（V3） | V1 保持普通窗口；V3 透明化前先做 spike 验证 |
| 移动端 WebView 音频自动播放限制 | 语音模式进入需用户手势（按钮），满足自动播放策略 |

---

*文档版本：2026-08-09 v1.4 — 新增 Phase V7 HUD TwinSprite 光球复刻（T1-T3）；锚点表新增 HUD TwinSprite 光球；总验收标准补充 V7 验证项；改动文件清单新增 V7 相关文件。v1.3 — ASR 累积回放去重修复（听写无限输入根因，`finalCursor` 游标）；Phase V6 语音听写完成（含真机验证）；HUD V5 拉取模型对齐修复入档。*
