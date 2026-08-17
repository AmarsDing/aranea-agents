# PERF-S1 抓包分析报告：Spirit 24k token/轮

> 抓包时间：2026-08-17 16:49:46
> 会话：85875cbe-817a-4b09-b8a7-cac221030937（Spirit "Hello" 平凡问答）
> 工具：llm_relay.py（:8899 → deepseek）→ analyze_capture.py

---

## 一、Token 构成总览

| 层级 | 消息数 | 字符数 | 估算 tokens | 占比 |
|------|--------|--------|-------------|------|
| **System Prompt** | 8 | 24,970 | ~7,134 | ~29% |
| **User** | 13 | 3,704 | ~1,058 | ~4% |
| **Assistant** | 303 | 0 | 0 | 0% |
| **Tool Results** | 319 | 388,911 | ~111,117 | ~67% |
| **合计** | 643 | 417,585 | ~119,310 | 100% |

**关键发现**：24k token_in 的实际构成中，system prompt 仅 ~7k（29%），**历史 tool 结果累积 ~111k（67%）** 才是大头。

---

## 二、System Prompt 分解（8 条消息，24,970 chars）

| # | 模块 | 字符数 | tokens | 占比 | 来源 |
|---|------|--------|--------|------|------|
| 0 | **AGENTS_CORE.md**（internal_config） | 12,627 | ~3,608 | **50.6%** | 核心系统提示（语言/文件/工具约束） |
| 1 | Runtime capability policy | 999 | ~285 | 4.0% | 工作区根目录/权限说明 |
| 2 | Tool catalog instructions | 426 | ~122 | 1.7% | CAPABILITIES.md 引用 |
| 3 | **Available Tools Catalog** | 3,813 | ~1,089 | **15.3%** | tool_load 可用工具列表 |
| 4 | Tool reminder（短） | 98 | ~28 | 0.4% | 工具调用后输出简短回复提醒 |
| 5 | **Available Skills** | 2,871 | ~820 | **11.5%** | skill_run 可用技能列表 |
| 6 | **Memory inject**（用户偏好） | 3,299 | ~943 | **13.2%** | `aranea:memory_inject` 长期记忆 |
| 7 | Available Knowledge Bases | 837 | ~239 | 3.4% | knowledge_search 可用知识库 |

**System prompt 三大头**：
1. **AGENTS_CORE.md**（3,608 tokens / 50.6%）—— 静态，不可裁剪
2. **Available Tools Catalog**（1,089 tokens / 15.3%）—— 可分级：平凡对话不注入
3. **Memory inject**（943 tokens / 13.2%）—— 可分级：无记忆命中时不注入

---

## 三、Tool 历史累积分析

- **319 条 tool 结果**（388,911 chars ≈ 111k tokens）
- 主要工具调用：`knowledge_search`（高频重复）、`file_search_content`、`file_read_file`、`skill_run`、`tool_load` 等
- 特征：**同一 tool 被反复调用**，历史结果无截断/压缩，全量累积

---

## 四、根因结论

PERF-S1 的 24k token_in 并非单一 system prompt 过大，而是 **"system prompt（7k）+ 历史 tool 结果（111k）"** 的叠加：

1. **主因（67%）**：Tool 历史无压缩累积 —— 303 轮 assistant + 319 条 tool 结果全量保留
2. **次因（29%）**：System prompt 7k tokens —— AGENTS_CORE.md 占 50%，Tools/Skills/Memory 注入占 42%

---

## 五、优化建议（按收益排序）

| 优先级 | 方案 | 预期收益 | 实施位置 |
|--------|------|----------|----------|
| **P0** | **Tool 历史压缩/截断**：限制保留最近 N 轮 tool 结果，或对 tool 结果做摘要压缩 | 降 67% → 24k → ~8k | `internal/agent/context/` 或 framework runner |
| **P1** | **System prompt 分级装载**：平凡对话（无 plan_and_execute 意图）不注入 Tools Catalog + Skills + Memory | 降 29% → 7k → ~4k | `internal/agent/prompt/` 或 `agent_defaults.go` |
| **P2** | **Prompt cache**：对 AGENTS_CORE.md 等静态部分启用 DeepSeek prompt cache | 降首 token 延迟 ~50% | `internal/agent/model/` |

---

## 六、验证建议

1. 实施 P0 后，复测 Spirit "Hello" 平凡问答，token_in 应从 24k 降至 ~8k
2. 实施 P1 后，平凡对话 system prompt 应从 7k 降至 ~4k
3. 实施 P2 后，首 token 延迟应从 9.9s 降至 ~5s
