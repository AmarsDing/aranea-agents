# 09 Provider Review

> **评分**：81 / 100 | **风险等级**：P1  
> **文档**：[9-provider-development.md](../需求/9-provider-development.md)  
> **代码锚点**：`internal/provider/` · `internal/provider/trpc_llm.go` · `internal/biz/llm_provider_model.go` · `web/src/pages/ResourceManagerPage.vue`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | Provider 目录、模型配置、启用/禁用、定价规则、Prometheus 指标均已落地 |
| 架构一致性 | 22 | 25 | `internal/provider` 独立适配层；`TRPCModelForProviderModel` 接口对齐 ✅；biz ↔ provider 双向依赖有改进空间 |
| 后端实现质量 | 18 | 20 | HA / 多模型切换、Provider 指标、`llminspect` 已拆出 |
| 前端实现质量 | 12 | 15 | `ResourceManagerPage` + 模型趋势对话框 ✅；密钥不回显 ✅ |
| 测试与验证 | 6 | 10 | 基础 Provider 测试；`trpc_llm.go` 适配路径测试待补 |
| 文档一致性 | 6 | 10 | `9-provider-development.md` 与现状基本对齐 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| Provider 列表 + 模型行 | ✅ |
| 模型启用/禁用 | ✅ |
| API Key + Base URL 配置（Key 不回显） | ✅ |
| 模型能力分类（vision/tool_call/embed 等） | ✅ |
| `model_pricing_rules` 定价配置 | ✅ |
| Provider Prometheus 指标 | ✅ M4 |
| 模型趋势对话框 | ✅ |
| `llminspect` 拆环 | ✅ M2 |
| HA / 多模型轮询 | ✅ |
| `TRPCModelForProviderModel` | ✅ |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| PROV-P1-01 | `biz <-> provider` 仍存在轻度双向依赖（模型 inspect 使用了 biz 类型）| 通过 `internal/llminspect` 端口完全隔离 |
| PROV-P1-02 | 未配置定价规则时 `total_cost_micro_usd=0`，配额 SUM 失效；需要明显的用户提示 | 在 `/models` 配置页加警告横幅 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| PROV-P2-01 | `trpc_llm.go` Provider 适配路径缺乏单测 | 补 Provider 适配层测试 |
| PROV-P2-02 | 多 Provider 故障转移（HA）行为未在 Monitor 中可视化 | 在 Monitor Events 中添加 provider failover 事件 |

---

## 建议优化路径

1. 完全隔离 `biz <-> provider` 双向依赖。
2. 在未配置定价规则时给用户明显提示。
3. 补 Provider 适配层测试。
