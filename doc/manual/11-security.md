# 11 安全与插件

## 功能

**五重安全防护 + 11 个内置插件 + 事件驱动钩子系统**：让 Agent 的每次工具调用、每分 Token 消耗、每次外部通知都在管控之内。

## 原理

### 11.1 11 个内置插件

| 插件 | 功能 |
|------|------|
| **identity** | 身份注入，自动将 Agent 身份信息注入上下文 |
| **guardrail** | 安全护栏，防止输出违规内容 |
| **toolcallid** | 工具调用 ID 追踪，确保调用链完整 |
| **messagemerger** | 消息合并，优化流式输出 |
| **confirmation_guard** | 工具调用确认，**高危操作需人工审批**（HITL 门禁） |
| **permission_guard** | 工具权限控制，deny_list 机制 |
| **cost_guard** | 成本预算守卫，按 scope 限流 |
| **model_router** | 模型路由，按规则自动切换模型 |
| **output_policy** | 输出策略控制，限制输出格式和内容 |
| **sensitive_mask** | 敏感信息脱敏，防止泄露隐私数据 |
| **skill_tracker** | Skill 调用追踪，记录技能使用情况 |

### 回调编排边界（三层）

| 层 | 挂载的插件 |
|----|-----------|
| Runner 层 | DB-backed 插件 + 框架插件 |
| LLMAgent 层 | 产品指标 + 工具计时/记录 + Hook 规则 |
| ModelSelector 层 | model_router / cost_guard |

三层回调链职责清晰，互不干扰。

### 五重安全防护

```text
confirmation_guard（人工确认）
  + permission_guard（权限白名单/黑名单）
  + sensitive_mask（脱敏）
  + output_policy（输出策略）
  + cost_guard（成本限流）
```

**高危工具确认门禁**：工具在 DB 中 `requires_confirmation=true` 时触发 HITL 审批——工具 declaration 的 `Name` 必须与 DB `tool_key` 完全一致才能正确匹配门禁。

### 11.2 钩子系统（Hook）

- **三维过滤**：HookCondition 按 Agent + Tool + Event 过滤，精准触达；
- **动作执行**：HookAction 支持 Webhook 回调、日志记录等；
- **Webhook 出站**：run.completed / run.failed / run.cancelled 等事件触发外部回调；
- **投递保证**：HookDelivery 记录每次投递状态（pending/success/failed）+ 重试机制；
- **出站安全**：outboundguard SSRF 防护，精确主机放行（无通配，新增主机须逐个登记）。

### 循环守卫

同一工具连续同参数调用达 2 次后，第 3 次起拦截（按节点隔离计数，key 加 node_id 维度；参数签名经 trim + NFC + 全角折叠归一化哈希，不改写真实下发参数）。

## 设计要点

- **工具白名单服务端化**：白名单校验在服务端实现，不依赖模型自觉；
- **方案 C 拦截处理**：工具调用被拦截后，模型应直接发起故障清除类调用，而非重复原调用；
- **凭证最小暴露**：所有密钥加密存储 + masked preview，界面永不回显明文。

## 界面配置

- **插件页**：查看 11 个内置插件的启用状态与配置（如 cost_guard 的 scope 限流阈值、model_router 的路由规则）；
- **Hooks 页**：创建 Hook → 设三维过滤条件 → 配置 Webhook 动作 → 查看投递记录与重试；
- **Webhooks 页**：管理出站回调端点与签名密钥；
- **工具页**：查看工具清单，高危工具标记确认门禁状态。

## 深入阅读

- [65 模块交叉引用 · plugin / hook 章节](../../docs/development/65-module-cross-reference-full.md)
- [23 工具开发计划](../../docs/development/23-tools.development.md)
