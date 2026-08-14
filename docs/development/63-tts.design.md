# 63 TTS（Text-to-Speech）— 设计文档（SUPERSEDED）

> **⚠️ SUPERSEDED（归档 · 禁止开工）**
>
> **不要实现独立 TTS 模块。** 无 `api/kratos/tts`、无 `internal/biz/tts`。禁止按本文候选架构 B/C 新建 `api/kratos/tts/v1/` 服务，也禁止按架构 A 去补 `internal/tools/tts/`。
>
> 流式 TTS **已并入 Voice (M74)**：[`74-voice-companion.md`](./74-voice-companion.md) / [设计](./74-voice-companion.design.md) / [开发计划](./74-voice-companion.development.md)。实现入口：`internal/voice/tts_scheduler.go`、`internal/data/speech/volcengine_tts.go`。
>
> 权威说明：[`65-module-cross-reference-full.md`](./65-module-cross-reference-full.md) 编号表与 §1.41。文件保留以免断链。

> **状态**：📦 已归档 / SUPERSEDED（原「占位文档」作废；2026-08-15）
> **同系列**：需求 → [`63-tts.md`](./63-tts.md)；开发计划 → [`63-tts.development.md`](./63-tts.development.md)

---

## 0. 文档导读

本设计曾描述独立 TTS 模块的候选架构。**该模块已 SUPERSEDED**：禁止按架构 B/C 新建 `api/kratos/tts/v1/`，禁止按架构 A 补 `internal/tools/tts/`。流式 TTS 在 Voice (M74)：[`74-voice-companion.design.md`](./74-voice-companion.design.md)。

**代码现状**：无独立 TTS proto / biz / data / service。catalog 停用的 `tts` 工具种子 **不是** 实现入口。

---

## 1. 架构设计（已归档 · 勿按此实现）

> **已归档**：以下候选架构 **不是** 待定方案。独立 TTS 不落地；播报管线见 M74。

### 1.1 候选架构 A：工具化（当前代码已支持）

`tts` 作为内置工具，由 Agent 运行时调用，返回语音文件。复用现有工具框架（`internal/tools/`）。

```
Agent → tts 工具调用 → TTS Provider API → 语音文件 → Artifact 存储 → 返回 URL
```

- **优点**：零新增服务层；复用工具策略（allow/deny/profile）
- **缺点**：仅支持 Agent 主动调用，不支持用户点击「朗读」被动触发

### 1.2 候选架构 B：独立 TTS 服务（不存在 · 禁止创建）

新增 `api/kratos/tts/v1/` proto + service / biz / data，支持用户被动触发语音合成。

```
前端「朗读」按钮 → POST /v1/tts:synthesize → TTS Service → Provider API → 返回音频流/URL
```

- **优点**：支持用户被动触发；可与 Agent 设置中的 TTS 配置联动
- **缺点**：新增完整服务层；需考虑配额、存储、缓存

### 1.3 候选架构 C：混合（A + B）

工具化（A）覆盖 Agent 主动调用；独立服务（B）覆盖用户被动触发。

---

## 2. 代码分层（当前）

> 当前无 TTS 独立代码分层。以下为 `tts` 工具注册涉及的代码位置（详见开发计划 §1）。

| 层 | 文件 | 说明 |
|----|------|------|
| data | `internal/data/builtin_tools_seed.go` | `tts` 工具种子注册（`enabled: false`） |
| biz | `internal/biz/agent_effective_tools.go` | `tts` 归入 `toolGroupsMedia` + `registryOptInOnlyKeys`（opt-in 策略） |

---

## 3. API 契约（独立 TTS 不存在 · 禁止创建）

> **已归档**：无 TTS 独立 proto。禁止补 `api/kratos/tts/v1/`。`tts` 工具种子不是实现入口。

### 3.1 `tts` 工具参数 schema（已存在）

```json
{
  "type": "object",
  "properties": {
    "text": { "type": "string" },
    "voice": { "type": "string" }
  },
  "required": ["text"]
}
```

- 来源：`internal/data/builtin_tools_seed.go`（`tts` 行的 `paramsSchema`）
- 工具元数据：`displayName="文本转语音"`、`category="media"`、`riskLevel="medium"`、`enabled=false`

### 3.2 候选独立 RPC（不存在 · 禁止创建）

| RPC | 方法 | 说明 |
|-----|------|------|
| `Synthesize` | `POST /v1/tts:synthesize` | 文本 → 语音（同步或流式） |
| `ListVoices` | `GET /v1/tts/voices` | 列出可用音色 |

> 禁止按 `api/kratos/` 补独立 TTS proto。语音合成契约见 [`74-voice-companion.design.md`](./74-voice-companion.design.md)。

---

## 4. 数据模型（已归档）

> **已归档**：无独立 TTS Ent Schema。**不要**新增。配置与留档见 M74 / Artifact 27。

- **TTS 配置存储**：复用 `AgentRuntimeSetting` 的扩展字段，或新增 `tts_config` JSON 字段（见 [`5-agent-setting.md` §8](./5-agent-setting.md#8-tabagent-语音合成tts) 「存储：TTS 配置或沙箱配置扩展，结构由后端约定」）
- **语音文件存储**：复用 Artifact 模块（[`27-artifact`](./27-artifact.md)）的云存储能力

---

## 5. 前端组件设计（已归档）

> **已归档**：不要按 63 做 Chat「朗读」按钮。播报 UI 在 M74 Companion。

| 组件 | 位置 | 说明 |
|------|------|------|
| TTS 配置对话框 | Agent 设置 Tab「Agent」§8 | 见 [`5-agent-setting.md` §8](./5-agent-setting.md#8-tabagent-语音合成tts) |
| 语音播放器 | Chat 消息操作区 | 历史候选；**不要实现**。播报在 M74 |

---

## 6. 技术选型（已归档 · 以 M74 为准）

| 项 | 候选 | 备注 |
|----|------|------|
| TTS Provider | OpenAI TTS / 其他 | 见需求 FR-5 |
| 音频格式 | MP3 / WAV / Opus | 浏览器兼容性考量 |
| 流式合成 | SSE / WebSocket | 复用现有 WS 通道？ |
| 文件存储 | Artifact 模块（S3 / 本地） | 复用 27-artifact |

---

## 7. 状态机（无）

当前独立 TTS 无状态机。异步合成任务 **不要** 按 63 补；播报调度见 M74 `tts_scheduler.go`。

---

## 8. 与其他模块的关系

| 模块 | 关系 |
|------|------|
| [`23-tools`](./23-tools.design.md) | catalog `tts` 种子（opt-in）；**不是**实现入口 |
| [`5-agent-setting`](./5-agent-setting.md) §8 | 历史 UI 规格；配置以 M74 为准 |
| [`27-artifact`](./27-artifact.md) | 语音文件存储（M74 留档复用） |
| [`74-voice-companion`](./74-voice-companion.design.md) | **现网 TTS**：流式播报管线 |

---

*文档版本：2026-08-15 — SUPERSEDED 归档；能力在 [`74-voice-companion.design.md`](./74-voice-companion.design.md)。文件保留以免断链。*
