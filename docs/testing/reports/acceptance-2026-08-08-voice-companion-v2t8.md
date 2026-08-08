# M74 语音伴侣 V2-T8 集成验收报告

> **验收任务**：74-voice-companion V2-T8（语音打开微信全流程、离线降级提示、留档回放）
> **验收标准来源**：[74-voice-companion.md §2.5/§2.7](../../development/74-voice-companion.md)
> **执行时间**：2026-08-08
> **执行者**：AI
> **结论**：服务端链路 **通过**（🟡）；桌面端在线「打开微信」全链路 **通过**（✅，2026-08-08 晚补测，含 ACL 缺口修复）；语音端到端/留档回放留待真机验收（同 V1-T10 口径）

---

## 1. 验收环境

| 项 | 值 |
|----|----|
| 后端 | `bin/admin.exe -conf configs`（独立 GOCACHE 构建，含 V2-T8 修复） |
| 数据库 | PostgreSQL（存量库，用于验证种子补播迁移） |
| 桌面端 | 第一轮**未连接**（恰好覆盖 DESKTOP_CLIENT_OFFLINE 降级路径）；第二轮**已连接**（`AraneaAgents.exe`，含差距 4 ACL 修复构建，19:10 重打包） |
| 语音配置 | ASR 已配置 / TTS 未配置（恰好覆盖麦克风门控分支） |
| Agent | `__spirit__`（deepseek-v4-flash） |

---

## 2. 验收发现与修复（三处装配缺口）

| # | 缺口 | 根因 | 修复 | 验证 |
|---|------|------|------|------|
| 差距 1 | 存量库缺 `client_open_app`/`client_open_url` 工具行 | 存量库在 client 工具加入种子清单**前**已应用 `builtin_platform_tools` 迁移，迁移门控跳过 | 新增版本化迁移 `20261202 builtin_platform_tools_client_reseed`（[ddl_migration_registry.go](../../../internal/data/ddl_migration_registry.go)）复跑幂等种子（ON CONFLICT DO NOTHING） | ✅ `/v1/tools` 列表可见两工具且 `enabled=true` |
| 差距 2 | 未配置 ASR/TTS 时麦克风按钮未置灰 | 前端无语音服务可用性探测 | 新增 `GET /v1/voice/status` 探测端点（[voice_ws.go](../../../internal/server/voice_ws.go) `VoiceStatusProbe`，DB-first/env-fallback 实时判定）；前端 [voiceStatus.ts](../../../web/src/features/companion/voiceStatus.ts) 拉取 + companion store `voiceAvailable`/`voiceMicDisabled` + HudCanvas 门控提示 | ✅ 实测返回 `{asr_available:true, tts_available:false}`，门控正确 |
| 差距 3 | chat 主链路调用 client 工具报 `tool not found` | `ClientBridge` 仅注入 graph/team/task 路径，chat 主链路 agent 构建缺失 | [chat_orchestrator.go](../../../internal/service/chat_orchestrator.go) `RuntimeTooling.ClientBridge` 字段 + [wire.go](../../../cmd/admin/wire.go) `provideRuntimeTooling` 注入 + [chat_orch_agent_build.go](../../../internal/service/chat_orch_agent_build.go) 透传 `TRPCExtensionDeps.ClientBridge` | ✅ 见 §3 全链路实测 |
| 差距 4 | 桌面端在线后 invoke 被 ACL 拒绝：`Command client_open_app not allowed by ACL` | Tauri v2 的 app 命令 ACL 权限（`allow-client-open-app` 等）需 `tauri-build` 在构建期显式声明命令才会自动生成；仅前端 invoke + Rust `generate_handler!` 不够 | [build.rs](../../../web/src-tauri/build.rs) 以 `AppManifest::new().commands(&["client_open_app","client_open_url"])` 声明触发权限自动生成；[capabilities/default.json](../../../web/src-tauri/capabilities/default.json) 增补 `allow-client-open-app`/`allow-client-open-url` 两条权限 | ✅ 见 §3.1 在线实测 |

**配套提示词修复**（差距 3 衍生）：spirit 提示词未明确客户端工具语义时 agent 误用 `exec_command` 在服务器侧探测应用路径。`CAPABILITIES.md` 新增「客户端工具（用户本机控制）」章节（直接调用/禁止服务器侧探测/离线如实转述），`DECISION.md` 与 intent 分类器同步放行「打开本机应用/网址」类请求不再追问澄清；同时清理了覆盖新提示词的 legacy `agent___spirit___*` 提示词文件。

---

## 3. 全链路实测（需求 §2.5）

「打开微信」文本入 chat 主链路，逐项对照 §2.5 验收标准：

| §2.5 验收标准 | 结果 | 证据 |
|----------------|------|------|
| Agent 识别为客户端工具调用，在用户本机（而非服务器）执行 | ✅ | 日志：`Executing tool client_open_app with args: {"target": "wechat"}`；不再误用 `exec_command` |
| 执行前弹出确认卡（全息渲染归真机项） | ✅（服务端门） | 会话进入 `awaiting_confirmation`/`tool_confirmation`；活动流产出 `confirm/tool_blocked` 活动（content=「工具 client_open_app 需要确认后执行」） |
| 目标不在白名单/找不到时明确提示，不静默失败 | ✅（桥已结构化错误码） | V2-T3/T4 已验 `NOT_WHITELISTED`/`TARGET_NOT_FOUND` 回传 |
| 桌面客户端不在线时明确告知，不假装执行 | ✅ | approve 续跑后工具返回 `{"error_code":"DESKTOP_CLIENT_OFFLINE","ok":false}`；agent 回复「桌面客户端未连接……请先启动桌面端」，未伪造成功 |

实测时序（日志 `logs/aranea-pipeline.log`）：

```
18:07:30  Executing tool client_open_app {"target":"wechat"}     ← 工具找到（差距 3 修复前为 not found）
18:07:30  confirm/tool_blocked 活动产出                          ← 确认门触发
          ConfirmActivity approved=true → {"accepted":true}      ← 人工确认
          工具续跑 → DESKTOP_CLIENT_OFFLINE                      ← 离线降级
          assistant 回复「桌面客户端未连接」                      ← 如实转述
```

### 3.1 桌面端在线全链路补测（差距 4 修复后，第二轮）

构建：重打包 `AraneaAgents.exe`（19:10，含差距 4 ACL 修复）→ 桌面端在线（`desktop_companion` 能力已注册）→ Playwright 经 CDP（9222）驱动 WebView 自动化（[drive-05-full.cjs](../../../test/v2t8-desktop/drive-05-full.cjs)）。

| §2.5 验收标准 | 结果 | 证据 |
|----------------|------|------|
| 全息确认卡弹出并等待确认 | ✅ | 截图 `test/v2t8-desktop/11-confirm-card.png`：确认卡展示工具 `client_open_app` + 参数 `{"target":"wechat"}` + 四按钮（允许本次/拒绝/会话内始终允许/始终允许），会话状态「等你确认」 |
| approve 后桌面端实际启动应用 | ✅ | 后端日志 19:14:42：`CallableTool client_open_app executed successfully, result: {"ok":true,"output":"launched D:\\Program Files (x86)\\Tencent\\Weixin\\Weixin.exe"}`（白名单用户覆盖命中真实安装路径） |
| 执行结果如实回传并播报 | ✅（文字） | 截图 `12-result.png`：活动流「已批准 · client_open_app」→ agent 回复「✅ 微信已成功打开！已在您的电脑上启动微信（D:\Program Files (x86)\Tencent\Weixin\Weixin.exe）」，会话状态「完成」；语音播报待 TTS 凭据 |

实测时序（19:14，UTC+8）：

```
19:14:21  发送「打开微信」
19:14:41  confirmation_guard / permission_guard before_tool 通过 → 确认卡渲染（截图 11）
19:14:42  点击「允许本次」→ client_tool.invoke 路由至桌面端 → Rust 白名单解析 → spawn 成功
19:14:42  audit_log after_tool status=ok；result 帧回传 → agent 生成成功回复（截图 12）
```

## 4. 语音可用性探测实测（差距 2 回归）

```
GET /v1/voice/status → 200 {"asr_available":true,"tts_available":false}
```

- 本环境 TTS 未配置 → 前端 `voiceAvailable=false` → 麦克风置灰 + 提示前往「系统设置 → 语音服务」配置，符合预期
- 探测每次调用实时读 DB（配置热生效，无需重启）

## 5. 留档（需求 §2.7）验收状态

| §2.7 验收标准 | 结果 |
|----------------|------|
| 开启留档后语音以音频附件挂用户消息 | ⏳ 真机项（V2-T6 单测已覆盖 PCM→WAV→Artifact 链路与降级） |
| 开关默认关闭，关闭时不产生音频文件 | ✅（V2-T6 单测） |
| 附件随消息持久化可回放 | ⏳ 真机项 |

## 6. 自动化回归

| 检查 | 结果 |
|------|------|
| `go build ./...`（独立 GOCACHE） | ✅ |
| voice / server / service / clientbridge 包测试 | ✅（V2-T3~T7 各任务验收时已全绿，本轮无回归） |
| vitest（companion 相关） | ✅（差距 2 修复时 companion.spec.ts 同步补例） |

## 7. 遗留真机项（同 V1-T10 口径）

依赖 ASR/TTS 凭据，须真机验收：

1. ~~桌面端在线时：全息确认卡 → 微信实际启动~~ ✅（2026-08-08 第二轮，见 §3.1）；其中**语音确认「好的」→ 粒子发射动画 → HUD 语音播报结果**仍待 TTS 凭据到位后实测
2. 语音留档回放（开启开关后语音消息挂音频附件、重开会话可回放）—— 2026-08-08 晚尝试 WS 直连实测（[drive-06-voice-archive.cjs](../../../test/v2t8-desktop/drive-06-voice-archive.cjs)，SAPI 合成 16k PCM 语音流式推送）：**阻塞于凭据无效**，但顺带修正一处配置错误——
   - 配置修正：DB 中 ASR endpoint 为旧版 `wss://openspeech.bytedance.com/api/v2/asr`，与适配器实现的 SAUC v3 协议不匹配（WS 握手成功但服务端不应答 v3 帧，客户端静默挂起无报错）；已改为正确的 `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel`（配置热生效）
   - 凭据阻塞：v3 端点握手被拒（`ASR_UNAVAILABLE: bad handshake`，retryable=true 正确下行），即 DB 中 AppKey/AccessKey 非有效火山凭据；留档链路（PCM→WAV→Artifact）仅有 V2-T6 单测覆盖，真实 ASR 终稿触发的端到端留档待有效凭据
   - 附带观察：协议不匹配场景下客户端无任何错误反馈（静默挂起），建议后续为 ASR 首包响应增加超时告警（非本任务范围，未改代码）
3. 打断 ≤300ms 停播实测（V2-T1）、AEC 调优（V2-T2）—— 同阻塞于 TTS 凭据（无播报则无打断对象）

---

**关联文档**：需求 [74-voice-companion.md](../../development/74-voice-companion.md) §2.5/§2.7；设计 [74-voice-companion.design.md](../../development/74-voice-companion.design.md) §2.5/§6；进度 [74-voice-companion.development.md](../../development/74-voice-companion.development.md) V2-T8 行
