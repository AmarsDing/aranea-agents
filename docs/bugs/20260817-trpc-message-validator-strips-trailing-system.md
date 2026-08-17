# 框架 Bug 报告：`message_validator.ensureLastMessageIsUserOrTool` 无条件剥离尾部 system 消息

## 基本信息

| 项 | 值 |
|---|---|
| 发现日期 | 2026-08-17 |
| 严重级别 | **高**（ silently 丢弃所有 hook 注入的尾部动态 cue） |
| 影响范围 | trpc-agent-go 所有使用 `enable_token_tailoring=true` 的模型 + 在 `BeforeModel` hook 尾部追加 system 消息的消费者 |
| 涉及文件 | `pkg/trpc-agent-go/model/message_validator.go` |
| 触发链 | `openai.prepareChatRequest` → `applyTokenTailoring` → `token_tailor.TailorMessages` → `validateAndFixMessageSequence` → `ensureLastMessageIsUserOrTool` |

## 现象

Aranea 的 memory/knowledge/tool-catalog 注入 hook 在 `BeforeModel` 阶段将动态 cue 以 `<!-- aranea:memory_inject -->` 等标记的 system 消息**追加到消息列表末尾**（设计意图：保持 [system block + history + user] 前缀单调递增，命中 provider 侧 prompt cache）。

- 框架 debug 日志 `afterBeforeModelCallbacks` 确认消息数为 **6**（含尾部 3 条 system cue）
- 但 LLM relay 抓包（`test/ts10-gns3/llm_relay.py`）确认实际发送到上游的请求只有 **3** 条消息（`sysLens=[9503,815]`），尾部 cue 全部消失
- 结果：召回命中的记忆事实（`composite_recall.stages hits=8~11`）无法到达 LLM，表现为"记忆召回全空"

## 根因

[message_validator.go:227-238](file:///f:/myproject/aranea-agents/pkg/trpc-agent-go/model/message_validator.go#L227-L238)：

```go
// ensureLastMessageIsUserOrTool ensures the last message is a user or tool message.
// This ensures the last message is a strict user/tool message when possible.
func ensureLastMessageIsUserOrTool(messages []Message) []Message {
    // Remove trailing system messages.
    end := len(messages)
    for end > 0 && messages[end-1].Role == RoleSystem {
        end--
    }
    messages = messages[:end]
    ...
}
```

该函数在 `validateAndFixMessageSequence` 中被**无条件调用**（token_tailor.go 多处：summarize/rebuild 前、无操作路径 line 537-538 等），不论是否真的发生了裁剪。设计假设是"尾部 system 消息是异常"，但 trpc-agent-go 的 `BeforeModel` hook 契约允许向 `args.Request.Messages` 追加消息，两者冲突。

## 证据链

1. **hook 注入成功**：容器日志 `agent.memory_cue.build` cue_chars=1530~3841、recall_hits=8、empty=false
2. **callbacks 后消息在位**：`llmflow` debug `afterBeforeModelCallbacks` 显示 `[0]system [1]system [2]user [3]system [4]system [5]system`（6 条）
3. **上链缺失**：relay 抓包同 ts 请求仅 `[0]system(9503) [1]system(815) [2]user`（3 条）
4. **DB 配置确认**：`llm_provider_models` 中 `deepseek:deepseek-v4-flash` / `deepseek:deepseek-chat` 的 `config_json.enable_token_tailoring = true`（context_window_k=1000）
5. **修复验证**：DB 将两模型 `enable_token_tailoring` 置 false 后（免重启生效），同一会话重发提问，抓包 idx=722 显示 msgs=6、sysLens=[9503,815,2871,1577,484]、`memInject=True`，agent 正确回答"您叫张伟"（证据 `evidence/v2-ask2.json`）

## 修复方案（供裁定）

| 方案 | 内容 | 评价 |
|---|---|---|
| A（已落地） | DB 关闭 deepseek 两模型 `enable_token_tailoring` | 零代码、立即可用；但属绕过，其他模型仍中招；失去超窗保护（当前 prompt ~15k vs context 1M，风险低） |
| B | Aranea 改注入位置：cue 插到静态 system 块之后 | 需改多个 hook + 重建；破坏前缀缓存单调递增设计（每轮 cue 变化导致缓存失效） |
| C（建议根治） | 框架内修改 `ensureLastMessageIsUserOrTool`：尾部 system 不删除，改为**重锚定**到最后一条 user/tool 之前；或仅在真实发生裁剪时才调用序列校验 | 需 FW 例外流程；一劳永逸，所有 hook 受益 |

## 上游比对

需确认上游 trpc-agent-go 最新版本是否已修复此问题（本仓库为 vendored 副本）。若上游未修，可考虑提 issue/PR。

## 关联

- 本项目规则：`aranea-agentstrae rulesroject_rules.md`「框架源码约束 FW-R1~R3 + 例外流程」
- 同源 bug：`scanFactRowJSON` 缺 `embedding_blob`（已于 2026-08-17 修复并部署）——两 bug 叠加导致"记忆召回全空"表象
