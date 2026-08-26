# 16 Agent 设置

## 功能

每个 Agent 拥有 **50+ 运行时参数**，按配置域分组独立调优——术业有专攻，每个岗位上的 Agent 都可以有自己的"性格"与"工作方式"。

## 参数配置域

| 配置域 | 关键参数 | 说明 |
|--------|----------|------|
| **Identity** | 路由身份、Agent Kind（llm/a2a_proxy）、Source（user/system/builtin/industry/marketplace） | Agent 是谁 |
| **Reasoning** | 推理模式、推理深度 | 思考方式 |
| **Memory** | L0~L4 每层独立开关与参数、记忆模式（Agentic/Auto） | 记什么、怎么记 |
| **Tools** | 工具执行策略、重试次数、并行度、流式、熔断器 | 怎么用工具 |
| **Skills** | Skill 加载模式（once/turn/session/progressive）、意图传递 | 怎么加载技能 |
| **Evolution** | 自进化开关、子 Agent、护栏参数 | 是否参与自动进化 |
| **Context** | 上下文压缩、输出 schema、压缩缓存 | 上下文管理 |
| **CodeExecutor** | 代码执行后端选择 | 沙箱执行 |
| **轮数闸** | `max_tool_iterations` / `max_llm_calls` | 单成员 ReAct 轮数上限（llm ≥ tool + 2） |

## 原理

### 生效链路

```text
agent_runtime_settings（DB）
  → effective_tools / effective_config 组装
  → biz.CoupledSafetyLimits（轮数闸耦合校验：llm ≥ tool + 2）
  → agent.SafetyLimitAdapter → 框架 WithMaxToolIterations
  → 运行时生效（热切换，无需重启）
```

### 工具生效规则（effective_tools）

- `tools_enabled` 总开关 + 具体 tool_key 开关；
- 高危工具（`requires_confirmation=true`）触发 HITL 确认门禁；
- 专项 Agent 工具画像：ToolsProfile（分组画像）+ Allow/Deny 清单 + Opt-in 工具——**不继承精灵工具箱**；
- profile 与策略代码（`agent_tool_policy.go`）一一对应，审计工具可线下复核。

### 轮数闸三档基线

| Agent 类型 | max_tool_iterations / max_llm_calls |
|-----------|-------------------------------------|
| eval 探针 | 60 / 62 |
| ops_* 运维岗 | 30 / 32 |
| 其余默认 | 50 / 52 |

## 界面配置

Agent 页 → 编辑任一 Agent → 设置面板按域分组：

- **模型**：选择 provider/model（如 `deepseek/deepseek-v4-flash`）；
- **记忆**：L0~L4 独立开关 + 各层参数（预算、召回条数、scope）；
- **工具**：总开关 + 按 tool_key 精细开关；高危工具带确认标记；
- **技能**：加载模式 + 意图路由开关；
- **进化**：是否参与技能/Agent 进化 + 护栏参数；
- **安全**：轮数闸、输出策略、熔断器。

修改即时生效（热切换），在聊天页可直接观察行为变化。

## 设计要点

- **0 = unlimited 慎用**：轮数闸为 0 表示不限制，513 万 token 事故后已全量配置三档基线，不建议回退；
- **配置即审计**：Agent 配置全面审计工具（`test/agent-audit/audit.py`）复刻运行时 effective_tools 逻辑，可离线全量复核 115 个 Agent 的配置问题；
- **设计豁免名单**：`__voice_butler__`（语音实时需求）与 `eval_probe` 前缀 Agent 允许 eff=0，已编码进审计工具。

## 深入阅读

- [65 模块交叉引用 · agent settings 章节](../../docs/development/65-module-cross-reference-full.md)
- [06 组织架构](06-organization.md)（专项 Agent 工具画像）
