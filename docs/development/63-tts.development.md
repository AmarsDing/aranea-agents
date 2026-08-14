# 63 TTS 语音 — 开发计划（SUPERSEDED）

> **⚠️ SUPERSEDED（归档 · 禁止开工）**
>
> **不要实现独立 TTS 模块。** 下文 Phase / 任务清单 / 未勾选验收项 **已归档，不是待办**。禁止创建 `api/kratos/tts/v1/`、`internal/biz/tts`、`internal/service/tts`、`internal/tools/tts/`。
>
> 流式 TTS **已并入 Voice (M74)**：[`74-voice-companion.md`](./74-voice-companion.md) / [设计](./74-voice-companion.design.md) / [开发计划](./74-voice-companion.development.md)。实现入口：`internal/voice/tts_scheduler.go`、`internal/data/speech/volcengine_tts.go`。catalog 停用的 `tts` 工具种子 **不是** 实现入口。
>
> 权威说明：[`65-module-cross-reference-full.md`](./65-module-cross-reference-full.md) 编号表与 §1.41。文件保留以免断链。

> **版本**：2026-06-17（重组三件套边界） | **状态**：📦 已归档 / SUPERSEDED（原「❌ 未实现占位」作废；2026-08-15）
> **需求**：[`63-tts.md`](./63-tts.md) · **设计**：[`63-tts.design.md`](./63-tts.design.md)
> **进度真相**：本模块已归档；语音合成以 M74 为准 | **EP**：—

---

## 1. 模块定位

TTS 语音合成：将 Agent 的文本回复转换为语音输出，支持多种语音模型和语言。

**代码锚点**：

| 文件 | 行 | 说明 | 状态 |
|------|----|------|------|
| `internal/data/builtin_tools_seed.go` | 78 | `tts` 工具种子注册：`displayName="文本转语音"`、`category="media"`、`riskLevel="medium"`、`enabled=false`、`paramsSchema={"properties":{"text":{"type":"string"},"voice":{"type":"string"}},"required":["text"]}` | ✅ 存在（停用态） |
| `internal/biz/agent_effective_tools.go` | 59 | `toolGroupsMedia = []string{"read_image", "read_document", "read_spreadsheet", "create_image", "tts"}` — `tts` 归入 media 工具组 | ✅ 存在 |
| `internal/biz/agent_effective_tools.go` | 172 | `registryOptInOnlyKeys["tts"] = true` — `tts` 为 opt-in 工具（默认停用，需 profile/allow 显式启用） | ✅ 存在 |
| `api/kratos/tts/v1/` | — | TTS 独立服务 proto | ❌ 不存在（禁止创建） |
| `internal/biz/tts*.go` | — | TTS Usecase | ❌ 不存在（禁止创建） |
| `internal/data/tts*.go` | — | TTS Repo | ❌ 不存在（禁止创建） |
| `internal/service/tts*.go` | — | TTS Service | ❌ 不存在（禁止创建） |
| `internal/tools/tts/` | — | `tts` 工具执行逻辑 | ❌ 不存在（禁止创建） |
| 前端独立 TTS 播放器 | — | 非 M74 的 Chat「朗读」按钮方案 | ❌ 不存在（禁止按 63 创建；播报见 M74） |

> **结论**：独立 TTS 模块 **SUPERSEDED**。无后端/前端独立实现。catalog `tts` 种子不是入口。流式 TTS 已在 M74：`internal/voice/tts_scheduler.go`、`internal/data/speech/volcengine_tts.go`。禁止按本计划补 `internal/tools/tts/` 或 `api/kratos/tts`。

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| TTS 独立服务（proto/biz/data/service） | ❌ 不存在（禁止创建） | 无代码；模块 SUPERSEDED |
| 流式 TTS（Voice M74） | ✅ | `internal/voice/tts_scheduler.go`、`internal/data/speech/volcengine_tts.go` |
| `tts` 内置工具注册 | ⚠️ catalog 占位 | `internal/data/builtin_tools_seed.go`（停用态；**不是**实现入口） |
| `tts` 工具策略（opt-in） | ⚠️ catalog 占位 | `internal/biz/agent_effective_tools.go`（`registryOptInOnlyKeys`） |
| `tts` 工具实际执行逻辑 | ❌ 不存在（禁止创建） | 无 `internal/tools/tts/`；不要补 |
| 独立前端 TTS 播放器（63 方案） | ❌ 不存在（禁止按 63 创建） | 播报 UI 在 M74 Companion |

---

## 3. 差距与优化

> **已归档**：下列「差距」**不是**待办。独立 TTS 不补；能力在 M74。

1. **归档**：不再建设独立 TTS 服务。
2. **归档**：不再补 `internal/tools/tts/` 执行逻辑。
3. **归档**：Agent 设置 TTS UI 不以 63 / `5-agent-setting` §8 为开工依据；语音配置见 M74 System Settings speech 分组。

---

## 4. 开发阶段

> **已归档**：下列 Phase **不是**路线图。禁止按此开工。

- ~~Phase 1：TTS 框架搭建 + OpenAI TTS 集成~~（归档）
- ~~Phase 2：前端语音播放器 + Agent 设置 TTS 配置 UI~~（归档；播报在 M74）
- ~~Phase 3：多语音模型支持~~（归档；SpeechProvider 在 M74）

---

## 5. 任务清单

> **已归档**：下列任务 **不是** backlog。禁止按此创建代码。

| # | 任务 | 优先级 | EP | 状态 |
|---|------|--------|-----|------|
| 1 | `api/kratos/tts/v1/` proto 定义（若选架构 B/C） | — | — | 📦 已归档（禁止创建） |
| 2 | TTS Service + Usecase + Repo（若选架构 B/C） | — | — | 📦 已归档（禁止创建） |
| 3 | `internal/tools/tts/` 工具执行逻辑实现（若选架构 A/C） | — | — | 📦 已归档（禁止创建） |
| 4 | OpenAI TTS API 集成 | — | — | 📦 已归档（语音合成走 M74 SpeechProvider） |
| 5 | 前端语音播放器组件（Chat 朗读按钮） | — | — | 📦 已归档（播报 UI 在 M74 Companion） |
| 6 | Agent 设置 TTS 配置 UI（5-agent-setting §8） | — | — | 📦 已归档（配置在 M74 speech 分组） |

---

## 6. 验收标准

> **已归档**：独立 TTS 无待验收项。语音合成进度见 [`74-voice-companion.development.md`](./74-voice-companion.development.md)。

- 📦 已归档：Agent 回复可转换为语音（FR-1）→ M74
- 📦 已归档：前端可播放语音回复（FR-2）→ M74
- 📦 已归档：Agent 设置中可配置 TTS Provider / 音色 / 密钥（FR-3）→ M74 speech 分组
- 📦 已归档：`tts` 工具启用后可被 Agent 调用并返回语音文件（FR-4）→ **不实现**

---

## 7. 依赖与风险

> 独立模块已归档。费用/存储/配额由 M74 SpeechProvider 与 Artifact 承担。不要为启用 catalog `tts` 种子去实现 `internal/tools/tts/`。

---

*文档版本：2026-08-15 — SUPERSEDED 归档；能力在 [`74-voice-companion.development.md`](./74-voice-companion.development.md)。文件保留以免断链。*
