# 63 TTS（Text-to-Speech）— 设计文档（占位）

> **状态**：占位文档（2026-06-17 创建，补全三件套）
> **同系列**：需求 → [`63-tts.md`](./63-tts.md)；开发计划 → [`63-tts.development.md`](./63-tts.development.md)

---

## 0. 文档导读

本设计文档描述 **TTS 语音合成** 的技术架构、API 契约、数据模型与前端组件设计。当前 TTS 作为独立模块**未规划落地**，本文档为占位，待启动 TTS 功能时补全。

**当前代码现状**（详见开发计划 §1 代码锚点）：
- 无独立 TTS 服务 proto / biz / data / service 实现
- 仅存在一个**停用**的 `tts` 内置工具注册项（media 类，opt-in 策略）

---

## 1. 架构设计（待定）

> 占位：TTS 功能未启动，以下为未来实现时的候选架构方向，待启动时定稿。

### 1.1 候选架构 A：工具化（当前代码已支持）

`tts` 作为内置工具，由 Agent 运行时调用，返回语音文件。复用现有工具框架（`internal/tools/`）。

```
Agent → tts 工具调用 → TTS Provider API → 语音文件 → Artifact 存储 → 返回 URL
```

- **优点**：零新增服务层；复用工具策略（allow/deny/profile）
- **缺点**：仅支持 Agent 主动调用，不支持用户点击「朗读」被动触发

### 1.2 候选架构 B：独立 TTS 服务（未实现）

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

## 3. API 契约（待定）

> 占位：当前无 TTS 独立 proto。`tts` 工具的参数 schema 已在种子中定义。

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

### 3.2 候选独立 RPC（未实现）

| RPC | 方法 | 说明 |
|-----|------|------|
| `Synthesize` | `POST /v1/tts:synthesize` | 文本 → 语音（同步或流式） |
| `ListVoices` | `GET /v1/tts/voices` | 列出可用音色 |

> 待启动时按 `api/kratos/` 既有 proto 约定补全。

---

## 4. 数据模型（待定）

> 占位：当前无 TTS 独立 Ent Schema。候选方案：

- **TTS 配置存储**：复用 `AgentRuntimeSetting` 的扩展字段，或新增 `tts_config` JSON 字段（见 [`5-agent-setting.md` §8](./5-agent-setting.md#8-tabagent-语音合成tts) 「存储：TTS 配置或沙箱配置扩展，结构由后端约定」）
- **语音文件存储**：复用 Artifact 模块（[`27-artifact`](./27-artifact.md)）的云存储能力

---

## 5. 前端组件设计（待定）

> 占位：TTS 前端组件未实现。候选组件：

| 组件 | 位置 | 说明 |
|------|------|------|
| TTS 配置对话框 | Agent 设置 Tab「Agent」§8 | 见 [`5-agent-setting.md` §8](./5-agent-setting.md#8-tabagent-语音合成tts) |
| 语音播放器 | Chat 消息操作区 | 「朗读」按钮 + 音频播放控件（未实现） |

---

## 6. 技术选型（待定）

| 项 | 候选 | 备注 |
|----|------|------|
| TTS Provider | OpenAI TTS / 其他 | 见需求 FR-5 |
| 音频格式 | MP3 / WAV / Opus | 浏览器兼容性考量 |
| 流式合成 | SSE / WebSocket | 复用现有 WS 通道？ |
| 文件存储 | Artifact 模块（S3 / 本地） | 复用 27-artifact |

---

## 7. 状态机（无）

当前 TTS 无状态机需求（工具调用为无状态单次请求）。若引入异步合成任务，需补全状态机（pending → synthesizing → ready / failed）。

---

## 8. 与其他模块的关系

| 模块 | 关系 |
|------|------|
| [`23-tools`](./23-tools.design.md) | `tts` 工具注册、策略（opt-in）、分组（media） |
| [`5-agent-setting`](./5-agent-setting.md) §8 | TTS 配置 UI（交互规格在需求文档） |
| [`27-artifact`](./27-artifact.md) | 语音文件存储（候选复用） |

---

*文档版本：2026-06-17 — 占位创建；待 TTS 功能启动时补全架构、API、数据模型、前端组件设计。*
