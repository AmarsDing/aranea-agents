# Provider — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[9 provider.md](./9%20provider.md) · **设计**：[9 provider.design.md](./9%20provider.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Provider 管理：管理 LLM Provider（OpenAI / Anthropic / Azure / 自定义）的配置、凭据、模型列表和可用性检测。

**代码锚点**：
- `api/kratos/llm_provider/v1/` — LLMProvider CRUD + ListModels
- `internal/service/llm_provider.go` — LLMProviderService
- `internal/biz/llm_provider.go` — LLMProviderUsecase
- `internal/data/llm_provider.go` — LLMProviderRepo
- `internal/agent/trpc_build.go` — Provider → trpc LLM 装配

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Provider CRUD | ✅ | Create/Update/Delete/Get/List |
| 模型列表 | ✅ | `ListModels` RPC（从 Provider API 拉取） |
| 凭据加密 | ✅ | `api_key` 字段存储 |
| Provider 类型 | ✅ | openai / anthropic / azure / custom |
- | 可用性检测 | ✅ | `CheckProvider` RPC |
| 前端管理 | ✅ | Provider 设置页 |

---

## 3. 差距与优化

1. **P2**：Provider 凭据未加密存储（`api_key` 明文存 SQLite），需加密或使用 vault。
2. **P3**：Provider 速率限制（rate_limit）配置未实现，无法在 Agent 运行时限制调用频率。
3. **P3**：Provider 健康检查定时任务未实现，仅手动触发 `CheckProvider`。

---

## 4. 开发阶段

- **Phase 1**：Provider 凭据加密存储（AES-256-GCM）
- **Phase 2**：Provider 速率限制配置
- **Phase 3**：Provider 健康检查定时任务

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | 凭据加密：AES-256-GCM 加密 api_key | P2 | — |
| 2 | 速率限制：provider 表增加 rate_limit 字段 + 运行时限流 | P3 | — |
| 3 | 健康检查：5min ticker + safego | P3 | — |

---

## 6. 验收标准

- [ ] api_key 在数据库中为密文
- [ ] 速率限制在 Agent 运行时生效
- [ ] Provider 异常时自动标记状态

---

## 7. 依赖与风险

- 凭据加密需迁移现有明文数据
- 速率限制需与 trpc-agent-go LLM 调用链集成
