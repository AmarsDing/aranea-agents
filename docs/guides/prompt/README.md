# Prompt 组装指南

> **定位**：说明 Aranea-Agents 在 Agent 构建期与每次 LLM 调用时，如何将各类内容拼接成最终 Prompt。面向 Agent 配置、记忆注入、Skill / 工具策略与 Chat Turn 排障。

---

## 文档索引

| 文档 | 说明 |
|------|------|
| [**assembly.md**](./assembly.md) | ★ Prompt 拼接全流程：构建期 System Instruction、运行时 Processor 链、BeforeModel Hook、User Message 侧扩展 |
| [../trpc-agent-go-framework.md](../trpc-agent-go-framework.md) | trpc-agent-go 框架接口与项目桥接（Runner / LLMAgent / Processor） |
| [../../需求/5 agent-setting.md](../../需求/5%20agent-setting.md) | Agent 运行时设置（工具、记忆、Intent Pass 等开关） |
| [../../需求/6 agent-setting-file.md](../../需求/6%20agent-setting-file.md) | Agent Prompt 文件（`AGENTS_*.md` 等） |
| [../../需求/memory/README.md](../../需求/memory/README.md) | L0–L4 记忆体系与注入策略 |

---

## 何时阅读

| 场景 | 阅读 |
|------|------|
| 配置 Agent system prompt / prompt 文件 | [assembly.md §一、构建期](./assembly.md#一构建期-system-instruction-拼接) |
| 排查「模型为何看到某段工具/记忆/技能说明」 | [assembly.md §二、运行时](./assembly.md#二运行时每次-llm-调用的拼接流水线) |
| 调整 Intent Pass 或附件如何进入 User Message | [assembly.md §三、User Message](./assembly.md#三user-message-侧turn-开始前) |
| 对照代码改 Prompt 逻辑 | [assembly.md §四、源码入口](./assembly.md#四源码入口速查) |

---

## 关键结论（速览）

1. **构建期**：`BuildSystemPrompt` + `RuntimeCapabilityCue` → `WithInstruction(sys)`；L4 **不在**构建期注入。
2. **运行时**：Processor 链 + BeforeModel 注入 **L2 / L3 / L4** 记忆。
3. **Intent Pass**（`IntentPassEnabled`，默认 false）将结构化意图 JSON 注入 **System 上下文**（`InjectedContextMessages`），User Message 保持原文；`intent_artifact` 仍写入 options JSON 供审计。
4. **BeforeModel 快照**：每次 LLM 调用前输出 `chat.prompt.compose` FlowLog（各段 est_tokens），可用 `ARANEA_PROMPT_SNAPSHOT=0` 关闭。

详细流程与对照表见 [assembly.md](./assembly.md)。
