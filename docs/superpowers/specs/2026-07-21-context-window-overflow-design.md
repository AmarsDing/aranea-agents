# 上下文窗口显示与单轮超限治理设计

> **设计稿** · 2026-07-21 · 状态：待评审
>
> **范围**：修复会话上下文使用率 100% 误显示；对齐观测/执行链路模型解析；单轮内容超窗口治理（文件落地分片读取 + 预检截断兜底）

***

## 一、背景与问题

### 1.1 用户反馈

Chat 头部用量环显示 100%（exceeded），但会话并未触发压缩，用户质疑压缩机制的百分比显示。

### 1.2 现场数据（问题会话）

| 字段                           | 值                   | 说明                               |
| ---------------------------- | ------------------- | -------------------------------- |
| `context_used_tokens`        | 69825               | 最近一轮真实 prompt tokens             |
| `last_context_window_tokens` | 64000               | 观测链路解析出的窗口（错误）                   |
| `context_used_ratio`         | 1.0                 | 69825/64000=1.09 被钳制             |
| agent 配置 model               | `deepseek-chat`     | **不在 provider\_models 目录中**      |
| 执行实际模型                       | `deepseek-v4-flash` | 目录配置 `context_window_k=1000`（1M） |

### 1.3 根因（代码证据）

#### 根因 A：观测链路与执行链路模型解析不一致（窗口分母错误）

**观测链路**（计算 ratio 分母）：

1. [`internal/service/chat_orchestrator_turn.go:190-192`](../../internal/service/chat_orchestrator_turn.go)：`prov/mod = FirstNonEmpty(请求, 会话默认, agent配置)` → `resolveProviderModelFallback`
2. [`internal/biz/chat_provider_model.go:28-30`](../../internal/biz/chat_provider_model.go) `ResolveProviderModel`：prov/mod 均非空时**直接返回，不校验目录存在性** → `admit.provider/model = deepseek/deepseek-chat`
3. [`internal/session/context_update.go`](../../internal/session/context_update.go) `ResolveContextWindowTokens` → `catalog.GetModelConfigJSON(deepseek, deepseek-chat)` 查无此模型 → `cfgJSON=""`
4. [`internal/llmcontext/window.go`](../../internal/llmcontext/window.go) `ResolveWindow`：catalog 无 → session 默认无 → **回退** **`ag.ContextWindow=64000`**

**执行链路**（实际调用的模型）：

1. [`internal/agent/trpc_build.go:74-93`](../../internal/agent/trpc_build.go)：`TRPCModelForProviderModel(deepseek-chat)` → `ErrProviderModelNotFound` → **回退系统默认** **`deepseek-v4-flash`**（1M 窗口）
2. [`internal/service/chat_orchestrator_turn_phases.go:470`](../../internal/service/chat_orchestrator_turn_phases.go)：`prov == agentDefaultProv && mod == agentDefaultMod` → **不注入** **`WithModel`** **RunOption** → 运行时确为构建期的 `deepseek-v4-flash`

**结果**：69825 tokens 在 1M 窗口下仅占 7%，但观测分母用了 64000 → ratio 1.09。

#### 根因 B：比率钳制掩盖真实状态

* 后端 [`internal/llmcontext/metrics.go:16-18`](../../internal/llmcontext/metrics.go)：`ratio > 1 → 1`

* 前端 [`web/src/features/session/contextMetrics.ts:17`](../../web/src/features/session/contextMetrics.ts)：`Math.min(1, ...)`

* 前端 [`web/src/components/sessions/sessionUi.ts`](../../web/src/components/sessions/sessionUi.ts) `ratioValue` 钳制 0-1

* 前端 [`web/src/components/chat/ChatHeaderUsagePanel.vue`](../../web/src/components/chat/ChatHeaderUsagePanel.vue) `clampedRatio` 钳制后算 `pctLabel`

即使根因 A 存在，若显示 109% + exceeded 状态，用户可立即识别异常，而非误以为"压缩到 100%"。

#### 根因 C：单轮巨型内容无治理手段

* 会话压缩（`Compressor.AfterNativeTurn`）面向**多轮历史**，异步触发于 turn 完成后；单轮 prompt 69825 tokens 无法被预防

* `ToolResultGate`（[`internal/biz/tool_result_gate.go`](../../internal/biz/tool_result_gate.go)）+ `read_tool_result` 分片工具（[`internal/tools/custom/read_tool_result.go`](../../internal/tools/custom/read_tool_result.go)）**只覆盖工具结果**

* 用户输入与文本附件全文直接进 prompt：[`internal/agent/attachments.go:34-58`](../../internal/agent/attachments.go) `Load` 后原样放入 `ContentParts`

***

## 二、设计目标

1. **窗口解析对齐（方向1B）**：观测链路窗口 = 执行链路实际模型的窗口，消除分母错误
2. **真实比率显示（方向2）**：超限时显示真实百分比（如 109%）+ exceeded 状态，不再钳制为 100%
3. **单轮超限治理 A**：巨型用户输入/文本附件落地 blob，prompt 中放 preview + 指针，agent 经 `read_tool_result` 分片读取
4. **单轮超限治理 B**：LLM 调用前预检截断兜底（框架 token tailoring 默认启用）
5. **数据修复（方向1A）**：修正 agent 配置模型为目录内存在的模型（运维操作）

## 三、非目标

* 不修改压缩触发阈值与压缩策略本身

* 不重构 `ToolResultGate`（复用其 blob/replacement 基础设施）

* 不为 system prompt / memory 注入超限做专门治理（罕见场景，YAGNI）

* 不改动 `read_tool_result` 工具本身（已满足分片读取 + session 归属校验）

* 方向1A 为数据运维，不含代码变更

***

## 四、设计方案

### 4.1 方向1B：模型解析对齐（观测 = 执行）

**修改** [`internal/biz/chat_provider_model.go`](../../internal/biz/chat_provider_model.go) `ResolveProviderModel`：

prov/mod 非空时，先经 `modelLister.GetByProviderAndModel` 校验目录存在性；若返回 `ErrProviderModelNotFound`，**不直接返回**，继续落入既有回退链（RefineLLM → 目录首个 enabled 模型），并记录 Warn 日志（结构化字段：原 provider/model、回退后 provider/model）。

```go
// 现状：非空即返回
if provider != "" && model != "" {
    return provider, model, nil
}

// 改为：非空但目录缺失 → 落入回退链
if provider != "" && model != "" {
    if uc.modelLister == nil {
        return provider, model, nil // 无目录可查，保持原值（nil-safe）
    }
    if _, err := uc.modelLister.GetByProviderAndModel(ctx, provider, model); err == nil {
        return provider, model, nil // 目录内有效
    } else if !errors.Is(err, ErrProviderModelNotFound) {
        return provider, model, nil // 目录查询本身失败（DB 错误），保持原值不阻断
    }
    uc.lg.Warn("配置模型不在目录中，回退解析", ...)
    // 继续向下走 RefineLLM → first-enabled 回退链
}
```

**对齐效果**：

| 环节                     | 修复前                              | 修复后                                                                        |
| ---------------------- | -------------------------------- | -------------------------------------------------------------------------- |
| `admit.provider/model` | `deepseek-chat`（目录外）             | `deepseek-v4-flash`（目录内）                                                   |
| 观测窗口                   | 回退 `ag.ContextWindow=64000`      | catalog `context_window_k=1000` → 1M                                       |
| 执行模型                   | 构建期 fallback `deepseek-v4-flash` | `turn_phases.go:470` 条件满足（prov≠agent配置）→ 注入 `WithModel(deepseek-v4-flash)` |
| 会话自愈                   | 无                                | `SyncSessionProviderModel` 同步 session 默认值为有效模型                             |

**保留**：`trpc_build.go:81-93` 构建期 fallback 作为兜底防线（agent 配置模型失效时构建不失败）。

**team 链路核查**：[`internal/team/runner_team_trpc.go`](../../internal/team/runner_team_trpc.go) 亦调用 `PatchContextFromLLMUsage`；实施时确认 team 的 prov/mod 解析是否经过 `ResolveProviderModel`，若为独立解析路径需同步目录校验（任务清单含核查项）。

### 4.2 方向2：真实比率显示（去钳制）

**后端**：

* [`internal/llmcontext/metrics.go`](../../internal/llmcontext/metrics.go) `ContextRatio`：删除 `ratio > 1 → 1` 钳制（保留 `<= 0 → 0`）

  * 调用方影响评估：`compressor.go:731`、`context_compression_inject.go:84`、`session_repo.go:614/681` 均为阈值比较或直接持久化，ratio>1 语义正确（超限就是超限），无破坏

  * `ContextStatusForRatio` 不变：`>= 0.95 → exceeded`（1.09 自然落入 exceeded）

  * `max_context_used_ratio`（`Greatest` 水印）可能 >1，属真实峰值，无害

**前端**：

| 文件                                                                                                           | 修改                                                             |
| ------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------- |
| [`web/src/features/session/contextMetrics.ts`](../../web/src/features/session/contextMetrics.ts)             | `contextRatioFromPrompt` 删除 `Math.min(1, ...)`                 |
| [`web/src/components/sessions/sessionUi.ts`](../../web/src/components/sessions/sessionUi.ts)                 | `formatPercent` 改用真实值（仅保留下限 0）；`ratioValue` 保留钳制专供进度条          |
| [`web/src/components/chat/ChatHeaderUsagePanel.vue`](../../web/src/components/chat/ChatHeaderUsagePanel.vue) | 环形进度 `value` 仍用钳制值（组件要求 0-100）；`pctLabel` 改用未钳制 ratio，可显示 109% |

i18n 已有 `exceeded → 超限` 映射，无需新增。

### 4.3 单轮超限治理 A：巨型内容落地 + 分片读取

复用 `tool_result_blobs` + `tool_result_replacements` 基础设施与 `read_tool_result` 工具（session 归属校验已内置），镜像 `ToolResultGate` 模式：

**扩展** [`internal/biz/tool_result_gate.go`](../../internal/biz/tool_result_gate.go)：新增方法

```go
// CheckUserInput 对超阈值的用户输入/附件文本落地 blob，返回 preview。
// tool_name 固定为 "user_input" / "attachment_text"，与工具结果区分。
func (g *ToolResultGate) CheckUserInput(ctx context.Context, sessionID, messageID, source, fullContent string) (ToolResultGateResult, error)
```

* 阈值：复用 `ToolResultSizeThreshold = 50000`（字符）

* preview 格式：复用 `freezePreview`（首部 2000 字符 + `[truncated … blob_id=… Use read_tool_result …]`）

* 幂等：同一 `(sessionID, messageID)` 已存在 replacement 时直接复用（既有逻辑）

**挂点 1 — 用户输入**：[`internal/service/chat_orchestrator_turn_phases.go`](../../internal/service/chat_orchestrator_turn_phases.go) `persistTurnUserMessage` 持久化前，对 `input.Content` 调用 `CheckUserInput`；消息表存 preview + 指针。**消息表不再保存巨型全文**（全文在 blob），否则下一轮历史再次带入，问题复发。

**挂点 2 — 文本附件**：[`internal/agent/attachments.go`](../../internal/agent/attachments.go) `BuildUserMessageFromArtifacts` 对非 image 附件 `data` 超阈值时同样落地，prompt 中 `ContentTypeFile` 替换为 preview 文本 part。签名增加 gate 参数（nil-safe：nil 时保持现状不落地）；调用方 [`chat_orchestrator_turn_metrics.go:103`](../../internal/service/chat_orchestrator_turn_metrics.go) 传入 `o.rt().ToolResultGate`。

**读取路径**：agent 工具集已含 `read_tool_result`（[`internal/tools/toolset_assemble.go:362-365`](../../internal/tools/toolset_assemble.go)），LLM 按 preview 指针中的 `blob_id/offset` 自行分页，无需新工具。

**行为契约**：前端消息渲染照旧（markdown 文本，含截断提示）；压缩/记忆读取消息内容时拿到 preview，与工具结果截断语义一致。

### 4.4 单轮超限治理 B：LLM 调用前预检截断兜底

框架能力已存在：[`pkg/trpc-agent-go/model/openai/openai.go:560`](../../pkg/trpc-agent-go/model/openai/openai.go) `applyTokenTailoring` 在每次 `GenerateContent` 前按 `contextWindow`（catalog `context_window_k` 经 `WithContextWindow` 注入）+ 安全边距裁剪 messages（默认 MiddleOut 策略）。**但仅在模型 config\_json 显式** **`enable_token_tailoring=true`** **时启用**。

**代码修改**（默认兜底）：[`internal/provider/catalog.go:169-171`](../../internal/provider/catalog.go) — `enable_token_tailoring` 未显式配置（nil）且 `context_window_k > 0` 时，默认启用：

```go
// 现状
if c.EnableTokenTailoring != nil {
    cfg.EnableTokenTailoring = *c.EnableTokenTailoring
}

// 改为：显式配置优先；未配置但有窗口数据时默认启用（截断优于 API 硬错误）
if c.EnableTokenTailoring != nil {
    cfg.EnableTokenTailoring = *c.EnableTokenTailoring
} else {
    cfg.EnableTokenTailoring = c.ContextWindowK > 0
}
```

**理由**：tailoring 仅在超预算时触发；不启用时超预算必然收到 provider 上下文溢出硬错误（turn 失败）。截断丢中间轮次是设计内的降级模式（与会话压缩同语义），优于直接失败。显式 `enable_token_tailoring=false` 的模型行为不变。

**治理分工**：

| 场景          | 治理手段                             |
| ----------- | -------------------------------- |
| 单条巨型用户输入/附件 | 4.3 blob 落地（MiddleOut 无法裁单条消息内部） |
| 多轮累积超窗口     | 4.4 框架 tailoring 兜底 + 既有会话压缩     |
| 工具巨型结果      | 既有 ToolResultGate（已覆盖）           |

### 4.5 方向1A：数据修复（运维操作，随实施一并执行）

1. 修正问题 agent 配置：`agents.model = 'deepseek-v4-flash'`（admin UI 或 SQL）
2. 全量核查：`agents` 表中所有 `(provider, model)` 不在 `provider_models` 的记录，逐一修正
3. 核查目录内模型均配置 `context_window_k`（观测分母与 tailoring 预算的共同数据源）

***

## 五、影响面与兼容性

| 变更                            | 影响                | 评估                                   |
| ----------------------------- | ----------------- | ------------------------------------ |
| `ResolveProviderModel` 增加目录校验 | 目录缺失时 prov/mod 回退 | 语义修正：原行为会产生无效模型调用/错误窗口；DB 错误时保持原值不阻断 |
| `ContextRatio` 去钳制            | ratio 可 >1        | 所有调用方为比较/持久化，无破坏；前端进度条仍钳制            |
| 用户消息存 preview                 | 消息表 content 变化    | 与工具结果截断同语义；全文可从 blob 追溯              |
| tailoring 默认启用                | 超预算时裁剪中间轮次        | 仅替代硬错误；显式配置不受影响                      |

***

## 六、验证计划（TDD）

**后端单测**（先红后绿）：

| 测试                                  | 断言                                                    |
| ----------------------------------- | ----------------------------------------------------- |
| `biz/chat_provider_model_test.go`   | 目录缺失 → 回退 RefineLLM/first-enabled；目录内 → 原值；DB 错误 → 原值 |
| `llmcontext/metrics_test.go`        | `ContextRatio(200000,128000)=1.5625`（改现有 `=1` 断言）     |
| `biz/tool_result_gate_test.go`      | `CheckUserInput` 阈值/幂等/preview 格式/session 归属          |
| `agent/attachments_test.go`         | 大文本附件落地 blob + preview part；小附件不变；nil gate 不变         |
| `provider/trpc_llm_options_test.go` | 未配置 + 有窗口 → tailoring 启用；显式 false → 不启用               |

**前端单测**：

| 测试                                      | 断言                                             |
| --------------------------------------- | ---------------------------------------------- |
| `contextMetrics.test.ts`                | `contextRatioFromPrompt(200000,128000)=1.5625` |
| `sessionUi` / `ChatHeaderUsagePanel` 测试 | `pctLabel` 显示 109%；ring value 钳制 100           |

**运行时验证**（R3 规则）：

1. 复现会话发送消息 → 日志确认 `chat.provider_resolve` 输出目录内模型
2. UI 用量环显示真实百分比（本例修复后应回落为 \~7%）
3. 构造 >50000 字符输入 → 消息显示截断指针，agent 经 `read_tool_result` 分页读取后正常作答

**全量**：`go build ./... && go test ./... && golangci-lint run`；`cd web && pnpm lint && pnpm test && pnpm build`

**文档同步**：`docs/development/10-session.design.md`（窗口解析链、ratio 语义、UserInputGate）；`65-module-cross-reference-full.md`（新增挂点）；本 spec 归档。

***

## 七、实施任务清单

| # | 任务                                             | 类型     | 状态 |
| - | ---------------------------------------------- | ------ | -- |
| 1 | 方向1B：`ResolveProviderModel` 目录校验回退 + team 链路核查 | 代码 TDD | ✅  |
| 2 | 方向2：后端去钳制 + 前端真实百分比                            | 代码 TDD | ✅  |
| 3 | 治理A：`CheckUserInput` + 两个挂点                    | 代码 TDD | ✅  |
| 4 | 治理B：tailoring 默认启用                             | 代码 TDD | ✅  |
| 5 | 方向1A：agent 模型配置修正 + 全量核查                       | 数据运维   | ✅  |
| 6 | 全量验证 + 文档同步                                    | 验证归档   | ✅  |

