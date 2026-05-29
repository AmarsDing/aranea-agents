# 09 Provider Review

> **评分**：88 / 100 | **风险等级**：P2
> **文档**：[9-provider-development.md](../需求/9-provider-development.md)
> **代码锚点**：`internal/provider/` · `internal/provider/trpc_llm.go` · `internal/biz/llm_provider_model.go` · `web/src/features/platform/`
> **审查时间**：2026-05-29（第二轮）

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 18 | 20 | Provider 目录、模型配置、启用/禁用、定价规则、Prometheus 指标均已落地 |
| 架构一致性 | 23 | 25 | Repo 接口拆分合规；ModelCapabilities 统一到 biz；HA 候选补齐预检+指标；SetInspector 全局注入仍有改进空间 |
| 后端实现质量 | 19 | 20 | HA 候选模型预检+指标装饰；死代码清理；wrapHA 签名简化 |
| 前端实现质量 | 14 | 15 | composable 拆分（1392→215+168+1123）；类型统一；数据流合规；Store 路由改造 |
| 测试与验证 | 7 | 10 | 基础 Provider 测试；`trpc_llm.go` 适配路径测试待补 |
| 文档一致性 | 7 | 10 | `9-provider-development.md` 已更新至 Phase 6 |

---

## 本轮优化变更

| # | 变更 | 端 | 影响文件 |
|---|------|----|----------|
| O1 | Repo 接口拆分（8方法→4子接口+1组合） | 后端 | `biz/llm_provider_model.go`, `cmd/admin/wire.go` |
| O2 | ModelCapabilities 统一到 biz 层 | 后端 | `biz/llm_provider_model.go`, `provider/catalog.go`, `provider/trpc_llm.go` |
| O3 | HA 候选模型预检+指标装饰 | 后端 | `provider/trpc_llm.go` |
| O4 | 清理 buildHuggingFaceSpecificOptions 空壳 | 后端 | `provider/trpc_llm.go` |
| O5 | wrapHA 移除无用 ctx 参数 | 后端 | `provider/trpc_llm.go` |
| O6 | ProviderConfig/ModelCategory/CapabilityChip 统一到 types.ts | 前端 | `features/platform/types.ts`, `providerModelUi.ts`, `ProviderTrendDialog.vue` |
| O7 | useResourceManagerPage composable 拆分 | 前端 | 新建 `useProviderList.ts` + `useProviderWizard.ts`，原文件 1392→215 行 |
| O8 | 工具函数提取到 providerUtils.ts | 前端 | 新建 `providerUtils.ts`，5 个文件去重 |
| O9 | 数据流合规改造 | 前端 | `useAgentProviderModelPicker.ts`, `useProviderWizard.ts` 改走 Store |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 | 状态 |
|----|------|---------|------|
| PROV-P1-01 | `biz <-> provider` 双向依赖：SetInspector/SetCredentialKeyResolver 全局注入 | 改为 Wire 构造函数注入 | ⏳ 待解决 |
| PROV-P1-02 | 未配置定价规则时配额 SUM 失效 | 后端 Monitor 提示 | ⏳ 待解决 |

### P2

| ID | 问题 | 建议修复 | 状态 |
|----|------|---------|------|
| PROV-P2-01 | `trpc_llm.go` Provider 适配路径缺乏单测 | 补 Provider 适配层测试 | ⏳ 待解决 |
| PROV-P2-02 | HA 故障转移行为未在 Monitor 中可视化 | 在 Monitor Events 中添加 provider failover 事件 | ⏳ 待解决 |
| PROV-P2-03 | UpsertModelPricingRule 先查后写不在同一事务 | 改为 Ent 事务 | ⏳ 待解决 |
| PROV-P2-04 | RunHealthChecks 直接修改快照行状态 | 改为仅标记需更新的 ID，批量事务更新 | ⏳ 待解决 |
| PROV-P2-05 | useProviderWizard 仍 1123 行 | 进一步拆分（目录选择/凭据/Inspect） | ⏳ 待解决 |

### P3

| ID | 问题 | 建议修复 | 状态 |
|----|------|---------|------|
| PROV-P3-01 | 凭据加密降级风险 | ARANEA_CREDENTIAL_KEY 未配置时强制提示 | ⏳ 待解决 |

---

## 建议优化路径

1. **P1**：SetInspector/SetCredentialKeyResolver 改为 Wire 构造函数注入（需解决 biz ↔ llminspect 循环依赖）
2. **P1**：后端 Monitor 中增加未配置定价规则的提示
3. **P2**：补 Provider 适配层测试（各厂商 buildXxxSpecificOptions）
4. **P2**：UpsertModelPricingRule 改为 Ent 事务
5. **P2**：useProviderWizard 进一步拆分
