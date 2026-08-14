# 63 TTS（Text-to-Speech）— 需求文档（SUPERSEDED）

> **⚠️ SUPERSEDED（归档 · 禁止开工）**
>
> **不要实现独立 TTS 模块。** 无 `api/kratos/tts`、无 `internal/biz/tts`。禁止新建独立 TTS 服务，也禁止按本文把停用的 `tts` 工具种子补成执行逻辑。
>
> 流式 TTS **已并入 Voice (M74)**：[`74-voice-companion.md`](./74-voice-companion.md) / [设计](./74-voice-companion.design.md) / [开发计划](./74-voice-companion.development.md)。实现入口：`internal/voice/tts_scheduler.go`、`internal/data/speech/volcengine_tts.go`。
>
> 权威说明：[`65-module-cross-reference-full.md`](./65-module-cross-reference-full.md) 编号表与 §1.41。文件保留以免断链；下文用户故事 / 功能清单 / 验收框已归档（非 ⏳/❌ 待办）。

> **状态**：📦 已归档 / SUPERSEDED（原「占位文档」作废；2026-08-15）
> **同系列**：设计 → [`63-tts.design.md`](./63-tts.design.md)；开发计划 → [`63-tts.development.md`](./63-tts.development.md)

---

## 0. 文档导读

本 PRD 曾描述独立 TTS 模块的产品需求。**该模块已 SUPERSEDED**：独立 TTS 未落地；流式 TTS 在 Voice (M74)。下文用户故事与清单仅作历史记录。

**代码现状**：无 `api/kratos/tts`、无 `internal/biz/tts` / `internal/data/tts` / `internal/service/tts`。catalog 停用的 `tts` 工具种子 **不是** 实现入口。语音合成走 [`74-voice-companion.md`](./74-voice-companion.md)。

---

## 1. 产品定位

TTS（Text-to-Speech）语音合成：将 Agent 的文本回复转换为语音输出，支持多种语音模型和语言。

- **目标用户**：需要语音交互场景的用户（无障碍访问、车载、语音播报等）
- **价值**：Agent 回复可听觉化，扩展交互模态

---

## 2. 用户故事（User Stories）

> **已归档**：以下为历史候选故事，**不是**待实现 backlog。禁止按本清单开工。独立 TTS 不落地；播报走 M74 Voice。

### 2.1 P2（候选）

**US-01 语音回复**
> 作为用户，我想在 Chat 中点击「朗读」按钮，听到 Agent 文本回复的语音播报。

要点：
- 朗读按钮位于消息操作区；
- 播放期间按钮变为「停止」；
- 播放结束自动恢复。

**US-02 TTS 配置**
> 作为运维，我想在 Agent 设置中配置 TTS Provider、音色、密钥，使该 Agent 的回复可语音化。

要点：
- Agent 设置 Tab「Agent」中提供 TTS 配置入口（交互规格见 [`5-agent-setting.md` §8](./5-agent-setting.md#8-tabagent-语音合成tts)）；
- 空态：静音图标 +「未配置 TTS」+ 说明文案；
- 配置对话框：选择 TTS Provider、音色、密钥等。

**US-03 工具化调用**
> 作为 Agent，我想通过 `tts` 工具主动将一段文本转成语音文件，供后续播放或下载。

要点：
- `tts` 作为 media 类工具，默认停用，按需 opt-in（与 `create_image` 同策略）；
- 入参：`text`（必填）、`voice`（可选）；
- 出参：语音文件句柄或 URL。

---

## 3. 功能需求清单

> **已归档**：以下为历史候选需求，**不是**待办。禁止按本清单开工。

| # | 需求 | 优先级 | 备注 |
|---|------|--------|------|
| FR-1 | Agent 回复可转换为语音 | P2 | US-01 |
| FR-2 | 前端可播放语音回复 | P2 | US-01 |
| FR-3 | Agent 设置中可配置 TTS | P2 | US-02；UI 见 5-agent-setting §8 |
| FR-4 | `tts` 工具可被 Agent 调用 | P2 | US-03；工具注册已存在（停用态） |
| FR-5 | 多语音模型支持 | P3 | OpenAI / 其他 Provider |

---

## 4. 非功能需求

| # | 项 | 要求 |
|---|----|------|
| NFR-1 | 延迟 | 首字语音延迟 ≤ 2s（流式 TTS） |
| NFR-2 | 成本 | TTS API 调用产生额外费用，需可配额控制 |
| NFR-3 | 存储 | 语音文件存储需考虑空间与清理策略 |
| NFR-4 | 兼容 | 浏览器音频播放兼容性（Web Audio API） |

---

## 5. 验收标准

> **已归档**：进度不以本节为准。语音合成验收见 [`74-voice-companion.development.md`](./74-voice-companion.development.md)。禁止把下列 checkbox 当待办。

- 📦 已归档：Agent 回复可转换为语音（FR-1）→ 已由 M74 播报管线覆盖
- 📦 已归档：前端可播放语音回复（FR-2）→ 已由 M74 Companion 覆盖
- 📦 已归档：Agent 设置中可配置 TTS Provider / 音色 / 密钥（FR-3）→ 见 M74 System Settings speech 分组
- 📦 已归档：`tts` 工具启用后可被 Agent 调用并返回语音文件（FR-4）→ **不实现**；catalog 种子不是入口

---

## 6. 显式不做（OUT-OF-SCOPE）

| # | 项 | 理由 |
|---|----|------|
| N1 | 独立 TTS 服务 proto / biz / data | **SUPERSEDED**；禁止创建。播报走 M74 |
| N2 | 实时语音对话（STT + TTS 全双工） | 超出 63 范围；语音闭环在 M74 |
| N3 | 补齐 catalog `tts` 工具执行逻辑 | **禁止**；种子不是实现入口 |

---

## 7. 与其他模块的关系

| 模块 | 关系 |
|------|------|
| [`5-agent-setting`](./5-agent-setting.md) §8 | 历史交互规格；语音配置以 M74 为准 |
| [`23-tools`](./23-tools.md) | catalog `tts` 种子（停用）；**不是**实现入口 |
| [`74-voice-companion`](./74-voice-companion.md) | **现网 TTS**：流式播报管线 |

---

*文档版本：2026-08-15 — SUPERSEDED 归档；能力在 [`74-voice-companion.md`](./74-voice-companion.md)。文件保留以免断链。*
