# 行业管理界面优化（市场页）

## Why

[IndustryMarketPage.vue](file:///f:/aranea-agents/web/src/pages/industries/IndustryMarketPage.vue) 当前实现过于朴素，仅 50 行业卡（emoji + 名称 + 一行描述），完全没用上项目已有设计系统（glass / hero / metric 体系）。同时 [IndustryCard.vue](file:///f:/aranea-agents/web/src/components/industries/IndustryCard.vue) 也是 13 行极简卡。三大问题：

1. **无数据密度**：用户看不到每个行业的部门/岗位/Agent 数量，必须点进详情才知道
2. **无视觉识别度**：emoji 当 hero icon，AI slop 重灾区
3. **无对比/筛选/视图切换**：3 个行业还行，未来到 8-12 行业时无法比较

参考兄弟组件 [TaxonomyIndustryCard.vue](file:///f:/aranea-agents/web/src/components/agents/TaxonomyIndustryCard.vue) 已有的 deptCount/posCount/agentCount 字段可知，数据已具备，缺的只是前端表达。

设计探索产物：3 个 hi-fi demo 在 [docs/design-experiments/industries/](../../design-experiments/industries/)（A 信息建筑派已确认采用）。

## Goals

- 引入 4 metric 卡（部门/岗位/Agent/已部署）作为行业卡的必备信息密度
- 引入顶部 metrics strip（KPI 卡）+ toolbar（搜索 + 状态/来源 chips + 视图切换）让多行业可比较
- monogram 替代 emoji 作为行业"icon"，杜绝 AI slop
- 引入侧滑 drawer：点击行业卡 → 滑出部门+岗位列表（替代直接跳详情页的硬切换）
- 列表/网格双视图：网格为默认，列表为密集对比
- 沿用项目 glass / cream / amber 设计 token，**0 新增依赖**
- 引入 "申请新行业" CTA 卡（dashed 边框 + 占位语义）

## Non-goals

- 不改 IndustryDetailPage（本次只优化市场列表页）
- 不动 IndustryWizard / 申请流程
- 不改 backend / API / store 数据流
- 不改 Industry、Department、Position 数据模型
- 不引入 dark 主题（沿用项目 cream light）
- 不做 B（运动诗学派）/ C（东方极简）方向（如需，分别独立 change）
- 不动 `i18n/locales/*.ts` 主入口结构（仅补 key）
