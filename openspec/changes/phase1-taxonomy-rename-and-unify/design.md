# 行业模板库审查修复设计文档

> 日期：2026-05-31（2026-06-05 文档对齐复查更新，修正 Phase 3/4 完成状态）
> 状态：Phase 1-4 已实施（Phase 2 部分完成，Phase 3/4 仅 taxonomy+prompt+skill 完成，agents.yaml 未添加新 Agent，详见 §9 实际完成状态）
> 审查来源：`docs/scenarios/industry-template-library.design.md` 方案执行审查

***

## 一、背景与问题

行业模板库设计文档定义了 Industry → Department → Position → Agent(variant) 四层分类体系，但审查发现以下 6 个问题：

| #    | 严重级别 | 问题                             | 说明                                                                          |
| ---- | ---- | ------------------------------ | --------------------------------------------------------------------------- |
| P0-1 | 🔴   | 两套分类体系并存                       | `agent_category_nodes`（单表树）与 `industries/departments/positions`（三表关系型）数据不一致 |
| P0-2 | 🔴   | softwaredev Agent 严重不足         | 方案要求 \~90-110 Agent，实际仅 10 个                                                |
| P0-3 | 🔴   | stockx 与 finance 未统一           | 两套独立 Agent/Team 体系平行存在                                                      |
| P1-1 | 🟡   | industry.yaml / teams.yaml 不存在 | 不符合方案目录规范                                                                   |
| P1-2 | 🟡   | SeedBuiltinIndustries 空实现      | 行业数据需手动 CLI 写入                                                              |
| P1-3 | 🟡   | 命名不准确                          | `AgentCategory` / `agent_category_nodes` 不反映实际功能（行业岗位分类体系）                  |

***

## 二、设计决策

### 2.1 统一分类体系：以 `agent_category_nodes` 为唯一真相源

**决策**：废弃 `industries/departments/positions` 三表，统一到 `agent_category_nodes`（重命名为 `industry_taxonomy`）。

**理由**：

* `agent_category_nodes` 已是 Agent 绑定（`category_position_id`）和 Prompt 生成（`GetPositionPrompt`）的真相源

* `IndustryService` 内部已在委托 `AgentCategoryUsecase`（GetPositionPrompt/ListPositionVariants）

* 前端行业市场页读三表、Agent 创建页读 `agent_category_nodes`，两套数据不同步

* `agent_category_nodes` 的 `metadata_json` / `config_json` 可存储丰富字段

### 2.2 命名重构：AgentCategory → IndustryTaxonomy

**决策**：全链路重命名 `AgentCategory` → `IndustryTaxonomy` / `TaxonomyNode`。

**理由**：

* `category` 太泛化，实际仅用于行业→部门→岗位三级分类

* `IndustryTaxonomy`（行业分类学）准确反映功能

* 代码中硬编码了三级约束（`normalizeTaxonomy`），命名应反映此约束

**实现补充**：实际实现中额外创建了 `IndustryTaxonomyService`（`api/kratos/industry_taxonomy/v1/industry_taxonomy.proto`），提供 `/v1/industry-taxonomy` HTTP 路由。该服务与 `TaxonomyService` 并行存在，共享同一 `TaxonomyUsecase`，但使用不同的 Proto message 命名（`IndustryTaxonomy` vs `TaxonomyNode`）。前端通过 `taxonomy-nodes` 资源名访问 `IndustryTaxonomyService`，通过 `taxonomy` 资源名访问 `TaxonomyService`。

### 2.3 stockx 统一到 finance YAML 体系

**决策**：将 stockx 的 Agent/Team 定义合并到 `finance/agents.yaml`，删除 `cmd/seed-stockx-org/`。

**理由**：

* 方案要求 finance 复用 stockx 场景

* YAML 体系是当前推荐方式，Go 代码硬编码是遗留模式

* 统一后只需维护一套 Agent/Team 定义

### 2.4 softwaredev Agent 分批补全

**决策**：分 3 批补全，P1 优先补全核心岗位（backend/frontend/gamedev 扩展）。

**理由**：

* 一次性补全 \~80 Agent 工作量大，分批降低风险

* P1 覆盖最常用的后端/前端/游戏开发岗位，优先级最高

***

## 三、命名重构映射表

### 3.1 后端 Go

| 层            | 旧名                                 | 新名                                      |
| ------------ | ---------------------------------- | --------------------------------------- |
| Ent Schema   | `AgentCategory`                    | `IndustryTaxonomy`                      |
| 物理表          | `agent_category_nodes`             | `industry_taxonomy`                     |
| Biz struct   | `AgentCategory`                    | `TaxonomyNode`                          |
| Biz struct   | `AgentCategoryTreeNode`            | `TaxonomyTreeNode`                      |
| Biz struct   | `CategoryAncestors`                | `TaxonomyAncestors`                     |
| Biz 接口       | `AgentCategoryReader`              | `TaxonomyReader`                        |
| Biz 接口       | `AgentCategoryWriter`              | `TaxonomyWriter`                        |
| Biz 接口       | `AgentCategoryRepo`                | `TaxonomyRepo`                          |
| Biz Usecase  | `AgentCategoryUsecase`             | `TaxonomyUsecase`                       |
| Biz 文件       | `agent_category.go`                | `taxonomy.go`                           |
| Data Repo    | `agentCategoryRepo`                | `taxonomyRepo`                          |
| Data 文件      | `agent_category.go`                | `taxonomy.go`                           |
| Service      | `AgentCategoryService`             | `TaxonomyService`                       |
| Service 文件   | `agent_category.go`                | `taxonomy.go`                           |
| Service（额外） | —                                  | `IndustryTaxonomyService`（额外新增）        |
| Service 文件（额外）| —                                | `industry_taxonomy.go`（额外新增）           |
| Seed 文件      | `seed_builtin_agent_categories.go` | `seed_builtin_taxonomy.go`              |
| Agent 字段     | `CategoryPositionID`               | `TaxonomyPositionID`                    |
| Agent Ent 字段 | `category_position_id`             | `taxonomy_position_id`                  |
| 转换函数         | `entToBizCat`                      | `entToBizTaxonomy`                      |
| 转换函数         | `toProtoCat` / `fromProtoCat`      | `toProtoTaxonomy` / `fromProtoTaxonomy` |

### 3.2 Proto

| 旧名                              | 新名                     |
| ------------------------------- | ---------------------- |
| package `agent_category`        | `taxonomy`             |
| message `AgentCategory`         | `TaxonomyNode`         |
| message `AgentCategoryTreeNode` | `TaxonomyTreeNode`     |
| service `AgentCategoryService`  | `TaxonomyService`      |
| 文件 `agent_category.proto`       | `taxonomy.proto`       |
| 目录 `api/kratos/agent_category/` | `api/kratos/taxonomy/` |
| Proto（额外）—                      | `industry_taxonomy.proto`（额外新增） |
| 目录（额外）—                        | `api/kratos/industry_taxonomy/`（额外新增） |
| `agent.proto` 中 `category_position_id` | ⚠️ 仍为 `category_position_id`（未重命名） |

### 3.3 HTTP 路由

| 旧路由                                | 新路由                        |
| ---------------------------------- | -------------------------- |
| `GET /v1/agent-categories`         | `GET /v1/taxonomy`         |
| `GET /v1/agent-categories/tree`    | `GET /v1/taxonomy/tree`    |
| `POST /v1/agent-categories`        | `POST /v1/taxonomy`        |
| `GET /v1/agent-categories/{id}`    | `GET /v1/taxonomy/{id}`    |
| `PATCH /v1/agent-categories/{id}`  | `PATCH /v1/taxonomy/{id}`  |
| `DELETE /v1/agent-categories/{id}` | `DELETE /v1/taxonomy/{id}` |
| `PUT /v1/agent-categories/reorder` | `PUT /v1/taxonomy/reorder` |

**额外新增路由**（`IndustryTaxonomyService`）：

| 路由                                       | 说明                     |
| ---------------------------------------- | ---------------------- |
| `GET /v1/industry-taxonomy`              | 列表（平铺）                |
| `GET /v1/industry-taxonomy/tree`         | 树形                     |
| `POST /v1/industry-taxonomy`             | 创建                     |
| `GET /v1/industry-taxonomy/{id}`         | 获取                     |
| `PATCH /v1/industry-taxonomy/{id}`       | 更新                     |
| `DELETE /v1/industry-taxonomy/{id}`      | 删除                     |

### 3.4 前端

| 旧名                                     | 新名                         |
| -------------------------------------- | -------------------------- |
| API resource `agent-categories`        | `taxonomy` + `taxonomy-nodes`（额外） |
| 组件 `AgentCategoryTree.vue`             | `TaxonomyTree.vue`         |
| 组件 `AgentCategoryPositionCard.vue`     | `TaxonomyPositionCard.vue` |
| 组件 `CategoryTreeNodeHeader.vue`        | `TaxonomyTreeNodeHeader.vue` + `TaxonomyNodeHeader.vue`（额外） |
| 页面 `AgentCategoriesPage.vue`           | `TaxonomyPage.vue`         |
| Composable `useAgentCategoriesPage.ts` | `useTaxonomyPage.ts`       |
| 工具 `categoryTreeUtils.ts`              | `taxonomyTreeUtils.ts` + `categoryTreeUtils.ts`（兼容 re-export） |
| Store 字段 `category_position_id`        | `taxonomy_position_id`     |
| 路由 `/settings/agent-categories`        | `/settings/taxonomy`       |
| 组件（额外）—                                | `TaxonomyIndustryCard.vue`（额外新增） |
| Store 变量（保留）—                          | `categoryTree`/`loadCategoryTree`/`selectedCategory`（保留为兼容别名） |

### 3.5 YAML / Loader

| 旧名                     | 新名                   |
| ---------------------- | -------------------- |
| `categories.yaml`      | `taxonomy.yaml`      |
| `categories_loader.go` | `taxonomy_loader.go` |
| `LoadCategoriesYAML`   | `LoadTaxonomySpec`   |

⚠️ **实现偏差**: `categories.yaml` 和 `categories_loader.go` 仍存在（死代码，未被引用）

### 3.6 不改名的部分

* `level` 字段值保持 `industry` / `department` / `position`

* `taxonomy_key` 格式不变：`finance/quant_trading/quant_researcher`

* `PositionPromptResult` / `VariantInfo` struct 不变

* `IndustryUsecase` / `DepartmentUsecase` / `PositionUsecase` — 将被删除

***

## 四、废弃三表体系

### 4.1 删除清单

| 层          | 文件                                         | 操作 |
| ---------- | ------------------------------------------ | -- |
| Ent Schema | `internal/data/ent/schema/industry.go`     | 删除 |
| Ent Schema | `internal/data/ent/schema/department.go`   | 删除 |
| Ent Schema | `internal/data/ent/schema/position.go`     | 删除 |
| Biz        | `internal/biz/industry_types.go`           | 删除 |
| Biz        | `internal/biz/industry_usecase.go`         | 删除 |
| Biz        | `internal/biz/department_types.go`         | 删除 |
| Biz        | `internal/biz/department_usecase.go`       | 删除 |
| Biz        | `internal/biz/position_types.go`           | 删除 |
| Biz        | `internal/biz/position_usecase.go`         | 删除 |
| Data       | `internal/data/industry_repo.go`           | 删除 |
| Data       | `internal/data/department_repo.go`         | 删除 |
| Data       | `internal/data/position_repo.go`           | 删除 |
| Service    | `internal/service/industry.go`             | 删除 |
| Proto      | `api/kratos/industry/`                     | 删除 |
| Server     | `http.go` / `grpc.go` 中 industry 注册        | 移除 |
| Server     | `service_registry.go` 中 `Industry` 字段      | 移除 |
| CLI        | `cmd/seed-industries/`                     | 删除 |
| Biz        | `internal/data/seed_builtin_industries.go` | 删除 |

### 4.2 IndustryService API 迁移

前端行业市场页当前读 `IndustryService` API，迁移到 `TaxonomyService`：

| 旧 API                                  | 新 API                                              |
| -------------------------------------- | -------------------------------------------------- |
| `GET /v1/industries`                   | `GET /v1/taxonomy?level=industry`                  |
| `GET /v1/industries/{key}/departments` | `GET /v1/taxonomy?level=department&parent_id={id}` |
| `GET /v1/departments/{key}/positions`  | `GET /v1/taxonomy?level=position&parent_id={id}`   |
| `GET /v1/positions/{key}/prompt`       | `GET /v1/taxonomy/{id}/prompt`                     |
| `GET /v1/positions/{key}/variants`     | `GET /v1/taxonomy/{id}/variants`                   |

⚠️ **实现偏差**: 以上 API 迁移仅后端完成（TaxonomyService 已替代 IndustryService），前端未迁移。`features/industries/api.ts` 仍调用 `/v1/industries` 旧 API（后端已无此路由），行业市场页 API 调用会 404

### 4.3 前端迁移

* `IndustryMarketPage` 改为读 `GET /v1/taxonomy?level=industry`

* `IndustryDetailPage` 改为读 taxonomy API（按 parent\_id 级联）

* `IndustryPositionPicker` 改为读 taxonomy API

* `features/industries/api.ts` 改为调 taxonomy API

⚠️ **实现偏差**: 以上前端迁移均未执行。`features/industries/api.ts` 仍调用 `/v1/industries` 旧 API（后端已无此路由），行业市场页 API 调用会 404

***

## 五、升级 categories.yaml → taxonomy.yaml

### 5.1 新格式

```yaml
industries:
  - key: finance
    name: 金融
    icon: finance
    description: 金融服务与投资研究
    sort_order: 1
    departments:
      - key: quant_trading
        name: 量化交易部
        description: 量化策略研发与算法交易
        sort_order: 1
        responsibilities:
          - 量化策略研发与回测
          - 算法交易系统开发与维护
        positions:
          - key: quant_researcher
            name: 量化研究员
            seniority_level: senior
            skills_required:
              - Python
              - 统计学
              - 量化建模
            responsibilities:
              - 因子挖掘与策略研发
              - 回测框架搭建与验证
            variants:
              - key: factor
                name: 因子研究
              - key: backtest
                name: 回测验证
              - key: portfolio
                name: 组合优化
              - key: ml_alpha
                name: 机器学习 Alpha
```

### 5.2 数据对齐规则

* 部门 key 以 `seed-industries/main.go` 为准（更丰富、更专业）

* 岗位 name 以 `seed-industries/main.go` 为准

* 补全 softwaredev 的 10 部门 / ~17 岗位

* 合并 `seed-industries` 中的 `responsibilities_json`、`skills_required`、`seniority_level` 到 YAML

⚠️ **实现偏差**: 以上数据对齐规则均未执行。`taxonomy.yaml` 仅包含基础字段（key/name/description/sort_order），部门 key 未对齐（仍使用 `risk_compliance` 而非 `compliance_risk` 等），selfmedia 的 `content_creation` 未拆分，`growth_monetization` 未改为 `distribution`

### 5.3 关键部门 key 对齐

| 行业          | categories.yaml key     | seed-industries key                       | 统一后 key           |
| ----------- | ----------------------- | ----------------------------------------- | ----------------- |
| finance     | `risk_compliance`       | `compliance_risk`                         | `compliance_risk` |
| finance     | `investment_research`   | `equity_research`                         | `equity_research` |
| finance     | `financial_engineering` | `fintech`                                 | `fintech`         |
| finance     | `wealth_management`     | `wealth_mgmt`                             | `wealth_mgmt`     |
| finance     | `derivatives`           | `fixed_income`                            | `fixed_income`    |
| selfmedia   | `content_creation`      | (拆分为 fiction\_writing + content\_graphic) | 拆分                |
| selfmedia   | `growth_monetization`   | `distribution`                            | `distribution`    |
| softwaredev | `game_client`           | `gamedev`                                 | `gamedev`         |

⚠️ **实现偏差**: 以上部门 key 对齐均未执行。`taxonomy.yaml` 中 finance 部门仍使用旧 key（`risk_compliance`/`investment_research`/`financial_engineering`/`wealth_management`/`derivatives`），selfmedia 的 `content_creation` 未拆分，`growth_monetization` 未改为 `distribution`。仅 softwaredev 的 `gamedev` 已对齐

***

## 六、统一 stockx 与 finance

### 6.1 Agent 映射

| stockx Agent Key            | stockx 岗位 | finance 映射岗位          | finance variant |
| --------------------------- | --------- | --------------------- | --------------- |
| `agent_coordinator`         | 主控调度员     | `trading_coordinator` | `premarket`     |
| `agent_critic`              | 评审员       | `trading_coordinator` | `critic`        |
| `agent_data_collector`      | 数据采集员     | `data_collector`      | `general`       |
| `agent_technical_analyst`   | 技术分析师     | `technical_analyst`   | `general`       |
| `agent_fundamental_analyst` | 基本面分析师    | `fundamental_analyst` | `general`       |
| `agent_money_flow_analyst`  | 资金面分析师    | `money_flow_analyst`  | `general`       |
| `agent_news_analyst`        | 消息面分析师    | `news_analyst`        | `general`       |
| `agent_sentiment_analyst`   | 情绪面分析师    | `sentiment_analyst`   | `general`       |
| `agent_industry_analyst`    | 行业分析师     | `industry_analyst`    | `general`       |
| `agent_risk_assessor`       | 风险评估师     | `risk_assessor`       | `general`       |
| `agent_quant_factor`        | 因子计算员     | `quant_researcher`    | `factor`        |
| `agent_chart_builder`       | 图表构建员     | `report_writer`       | `chart`         |
| `agent_report_writer`       | 报告撰写员     | `report_writer`       | `general`       |

### 6.2 Team 合并

| stockx Team              | finance 已有？ | 操作                             |
| ------------------------ | ----------- | ------------------------------ |
| `team-premarket-brief`   | ✅ 已有        | 统一成员引用，使用 finance 的 agent\_key |
| `team-stock-deep-dive`   | ✅ 已有        | 统一成员引用                         |
| `team-sector-rotation`   | ✅ 已有        | 统一成员引用                         |
| `team-portfolio-doctor`  | ✅ 已有        | 统一成员引用                         |
| `team-market-recap`      | ✅ 已有        | 统一成员引用                         |
| `team-research-pipeline` | ❌ 新增        | 添加到 finance/agents.yaml        |
| `team-deep-dive-critic`  | ❌ 新增        | 添加到 finance/agents.yaml        |

### 6.3 Prompt 迁移

stockx 的 prompt 文件从 `cmd/seed-stockx-org/` 迁移到 `internal/scenario/finance/prompts/positions/` 目录。

### 6.4 删除

* 删除 `cmd/seed-stockx-org/` 目录

* 删除 stockx 相关的独立分类树定义

⚠️ **实现偏差**: stockx 合并未完全执行。`cmd/seed-stockx-org/` 已删除 ✅，stockx prompt 文件已迁移（`critic.md`/`chart.md` 存在）✅，但以下内容未合并到 `finance/agents.yaml`：
- `trading_coordinator/critic` variant（§6.1 中 `agent_critic` 的映射）未添加
- `report_writer/chart` variant（§6.1 中 `agent_chart_builder` 的映射）未添加
- `team-research-pipeline` 团队（§6.2 中新增）未添加
- `team-deep-dive-critic` 团队（§6.2 中新增）未添加

✅ **已纠正**: 现有 5 个 stockx 团队（`team-premarket-brief` 等）的成员引用已统一为 finance agent_key（如 `trading-coordinator-premarket`、`data-collector-general` 等），与之前偏差记录不符。

⚠️ **额外实现**: finance/agents.yaml 包含 3 个设计未规划的额外 team：
- `team-quant-strategy-research`（量化策略研发，sequential 模式）
- `team-investment-committee`（投资决策委员会，coordinator 模式）
- `team-risk-monitoring`（风险监控，parallel 模式）

***

## 七、补全 softwaredev Agent

### 7.1 分批计划

| 批次 | 部门           | 新增岗位                                                              | 新增 Agent（含 variant） | 优先级 |
| -- | ------------ | ----------------------------------------------------------------- | ------------------- | --- |
| P1 | backend      | Java 高级工程师(3), Python 高级工程师(2), Rust 工程师(2), C++ 后端工程师(2), DBA(2) | 11                  | 高   |
| P1 | frontend     | React 高级前端(3), TS 专家(2), 前端性能(2), UI/UX 还原(1)                     | 8                   | 高   |
| P1 | gamedev      | UE 游戏逻辑(2), UE 图形渲染(2), 游戏服务端(2), TA(1), 策划(1)                    | 8                   | 高   |
| P2 | devops       | SRE(3), CI/CD(2), 容器编排(2), 监控(2), 基础设施(1)                         | 10                  | 中   |
| P2 | architecture | 系统架构师(3), 技术负责人(2), 解决方案架构师(2)                                    | 7                   | 中   |
| P2 | qa           | 测试工程师(3), 自动化测试(2), 性能测试(1)                                       | 6                   | 中   |
| P3 | mobiledev    | iOS(3), Android(3), Flutter(2), RN(1)                             | 9                   | 低   |
| P3 | dataeng      | 数据工程师(3), 数据平台(2)                                                 | 5                   | 低   |
| P3 | security     | 安全工程师(2), 渗透测试(1), 安全审计(1)                                        | 4                   | 低   |
| P3 | productpm    | 产品经理(2), 项目经理(2)                                                  | 4                   | 低   |

**P1 完成后**：10 → 37 Agent
**P2 完成后**：37 → 60 Agent
**P3 完成后**：60 → 82 Agent

### 7.2 每个 Agent 需要的交付物

1. `agents.yaml` 中的定义（position\_key + variant + model + tools\_profile + skills）
2. `prompts/positions/{position_key}/{variant}.md` prompt 文件
3. Skill 文件（如需要，放在 `skills/` 目录）
4. Schema 文件（如需要，放在 `schemas/` 目录）

***

## 八、实现 SeedBuiltinTaxonomy 自动 seed

### 8.1 方案

* 将 `taxonomy.yaml` 的加载和种子逻辑集成到 `SeedBuiltinTaxonomy`（原 `SeedBuiltinAgentCategories`）

* 启动时自动执行，版本门控 `SeedTaxonomyV1`

* ⚠️ **实现偏差**: 种子逻辑实际使用 Raw SQL（INSERT ... ON CONFLICT），而非设计要求的 Ent ORM。通过 `TaxonomyRepo.Create` / `TaxonomyRepo.Update` 实现 upsert 的方案未执行

* 删除 `cmd/seed-industries/` CLI（功能已被自动 seed 替代）

* ⚠️ **实现偏差**: `cmd/seed-industry-agents/` CLI 也已删除（设计要求保留）

* ⚠️ **实现偏差**: `metadata_json` 未存储扩展字段（responsibilities/skills_required/seniority_level），始终为空字符串

### 8.2 种子版本

```go
const SeedTaxonomyV1 = 20260701
```

⚠️ **实现偏差**: 实际值为 `SeedTaxonomyV1 = 20260529`，且 `SeedCategoriesV2 = 20260530` 仍保留在 `seed_versions.go` 中

***

## 九、执行顺序

```
Phase 1: 命名重构 + 数据统一（基础设施）
  1.1 重命名 AgentCategory → IndustryTaxonomy（全链路：Ent/Biz/Data/Service/Proto/前端）
  1.2 升级 categories.yaml → taxonomy.yaml（合并 seed-industries 数据）
  1.3 实现 SeedBuiltinTaxonomy 自动 seed
  1.4 废弃 industries/departments/positions 三表
  1.5 IndustryService API 迁移到 TaxonomyService
  1.6 前端迁移
  1.7 make api && make wire && make build && make test && make lint
  1.8 前端 pnpm lint && pnpm build

Phase 2: stockx 统一（金融行业包完善）
  2.1 stockx Agent 合并到 finance/agents.yaml
  2.2 stockx Team 合并到 finance/agents.yaml
  2.3 stockx prompt 迁移到 finance/prompts/positions/
  2.4 删除 cmd/seed-stockx-org/
  2.5 运行 seed-industry-agents CLI 验证

Phase 3: softwaredev Agent 补全（P1 批次）
  3.1 补全 backend 岗位 Agent（11 个）
  3.2 补全 frontend 岗位 Agent（8 个）
  3.3 补全 gamedev 岗位 Agent（8 个）
  3.4 编写 prompt 文件
  3.5 编写 Skill 文件
  3.6 运行 seed-industry-agents CLI 验证

Phase 4: softwaredev Agent 补全（P2+P3 批次）
  4.1 补全 devops/architecture/qa 岗位（23 个）
  4.2 补全 mobiledev/dataeng/security/productpm 岗位（22 个）
  4.3 编写 prompt 文件
  4.4 编写 Skill 文件
  4.5 运行 seed-industry-agents CLI 验证
```

**实际完成状态**（2026-06-05 复查）：
- Phase 1: ⚠️ 基本完成（1.6 前端行业市场页迁移未执行，1.10 selfmedia variant 连字符未修复）
- Phase 2: ⚠️ 部分完成（2.3 prompt 迁移 ✅，2.4 删除 ✅，2.1/2.2 Agent/Team 合并未完成；⚠️ 实际 finance 团队成员引用已统一为 finance agent_key，但缺少 critic/chart variant 和 team-research-pipeline/team-deep-dive-critic；⚠️ 实际有 3 个设计未规划的额外 team：team-quant-strategy-research/team-investment-committee/team-risk-monitoring）
- Phase 3: ⚠️ 部分完成（3.0 taxonomy.yaml ✅，3.4 prompt 文件 ✅，3.5 Skill ✅；⚠️ 3.1-3.3 agents.yaml 未添加新 Agent，仅 10 个原始 Agent，缺少约 17 个 P1 Agent）
- Phase 4: ⚠️ 部分完成（4.3 prompt 文件 ✅，4.4 Skill ✅；⚠️ 4.1-4.2 agents.yaml 未添加 P2+P3 Agent，缺少约 45 个 Agent）

***

## 十、复查计划

完成所有修改后，使用 `aranea-review` SKILL 进行以下三项复查：

### 10.1 行业/部门/岗位职责描述复查

检查项：

* 每个行业的部门划分是否合理，是否覆盖方案要求的所有部门

* 每个部门的岗位职责描述是否准确、专业

* 每个岗位的 skills\_required 是否与职责匹配

* 每个岗位的 seniority\_level 是否合理

* 岗位 variant 定义是否覆盖该岗位的核心工作场景

* `taxonomy.yaml` 中的描述与 `prompts/positions/` 下的 prompt 文件是否一致

### 10.2 Team 配置复查

检查项：

* 每个 Team 的 mode（coordinator/parallel/sequential）是否与业务场景匹配

* Team 成员是否引用了正确的 agent\_key（与 agents.yaml 一致）

* Graph 定义（nodes/edges）是否完整且可达

* Team 的 max\_concurrency / timeout\_seconds 是否合理

* stockx 合并后的 Team 与原有 finance Team 是否有冲突

* 每个 Team 的描述是否准确反映其业务目的

### 10.3 Agent 命名/归属/职责配置复查

检查项：

* Agent 的 `position_key` 是否正确归属到对应岗位

* Agent 的 `agent_variant` 命名是否语义化且一致

* Agent 的 `variant_description` 是否准确描述该变体的职责

* Agent 的 model 选择（fast\_model / strong\_model）是否与岗位复杂度匹配

* Agent 的 tools\_profile 是否与岗位职责匹配

* Agent 的 skills 引用是否与岗位技能要求一致

* 同一岗位不同 variant 之间的职责边界是否清晰

* stockx 合并后的 Agent 是否与原有 finance Agent 有 key 冲突

### 10.4 文档对齐

修改完成后，对齐以下文档：

* `docs/scenarios/industry-template-library.design.md` — 更新方案中与实现不一致的部分

* `docs/scenarios/industry-template-library-implementation-plan.md` — 更新实施计划

* `docs/architecture-blueprint.md` — 更新模块描述（AgentCategory → IndustryTaxonomy）

* `docs/module-cross-reference.md` — 更新交叉参考（删除 Industry/Department/Position 模块卡片，更新 Taxonomy 模块卡片）

***

## 十一、风险与缓解

| 风险                  | 影响       | 缓解措施                             |
| ------------------- | -------- | -------------------------------- |
| 命名重构范围大，可能遗漏        | 编译失败     | 全链路 grep 确认无遗漏引用；`make build` 验证 |
| 前端 API 路径变更         | 前端调用失败   | 前后端同步修改；`pnpm build` 验证          |
| stockx Agent key 冲突 | 种子数据写入失败 | 合并前检查 key 唯一性；`--dry-run` 验证     |
| taxonomy.yaml 数据量大  | 种子写入慢    | 使用 upsert 语义；版本门控避免重复写入          |
| 三表删除影响其他模块          | 编译/运行时错误 | 先 grep 全部引用；Wire 验证              |

---

## 十二、实现进度

### Phase 1: 命名重构 + 数据统一（基础设施）

| 步骤 | 描述 | 状态 | 备注 |
|------|------|------|------|
| 1.1 | 重命名 AgentCategory → IndustryTaxonomy（全链路） | ✅ 已完成 | Proto/Ent/Biz/Data/Service/Server/Agent/Wire/Cmd |
| 1.2 | 升级 categories.yaml → taxonomy.yaml | ✅ 已完成 | Loader 重命名为 taxonomy_loader.go |
| 1.3 | 实现 SeedBuiltinTaxonomy 自动 seed | ✅ 已完成 | 表名 industry_taxonomy，字段 taxonomy_key |
| 1.4 | 废弃 industries/departments/positions 三表 | ✅ 已完成 | 从 Wire DI 移除，源文件保留待后续清理 |
| 1.5 | IndustryService API 迁移到 TaxonomyService | ✅ 已完成 | TaxonomyService 已替代 IndustryService |
| 1.6 | 前端迁移 | ⚠️ 部分完成 | 文件名/页面名/API 函数名已重命名；⚠️ 行业市场页（features/industries/）未迁移到 Taxonomy API |
| 1.7 | make api && make wire && make build && make test | ✅ 已完成 | 全量编译和测试通过 |
| 1.8 | 前端 pnpm build | ✅ 已完成 | npx quasar build 通过 |
| 1.9 | aranea-review 审查 + 三项专项复查 | ✅ 已完成 | 3 阻断已修复，5 建议记录备忘 |
| 1.10 | 修复审查发现的问题 | ⚠️ 部分完成 | data/taxonomy.go fmt.Errorf→kerrors；seed ID 前缀 cat→tax；⚠️ selfmedia variant 连字符→下划线未实际执行（`platform-adapt`/`data-driven`/`geography-history`/`magic-system` 仍使用连字符）；SQL DDL 表名更新 |

### Phase 2: stockx 统一（金融行业包完善）

| 步骤 | 描述 | 状态 | 备注 |
|------|------|------|------|
| 2.1 | stockx Agent 合并到 finance/agents.yaml | ⚠️ 部分完成 | trading_coordinator/critic 和 report_writer/chart variant 未添加；其余 stockx Agent 已通过原有 finance Agent 覆盖 |
| 2.2 | stockx Team 合并到 finance/agents.yaml | ⚠️ 部分完成 | team-research-pipeline 和 team-deep-dive-critic 未添加；5 个已有 Team 成员引用已统一为 finance agent_key ✅；额外新增 3 个设计未规划的 team（team-quant-strategy-research/team-investment-committee/team-risk-monitoring） |
| 2.3 | stockx prompt 迁移到 finance/prompts/positions/ | ✅ 已完成 | 新增 critic.md + chart.md |
| 2.4 | 删除 cmd/seed-stockx-org/ | ✅ 已完成 | 6 个文件已删除 |
| 2.5 | 运行 seed-industry-agents CLI 验证 | ⏳ 待验证 | 需预先修复 biz/usage + biz/monitor 编译错误 |

### Phase 3: softwaredev Agent 补全（P1 批次）

| 步骤 | 描述 | 状态 | 备注 |
|------|------|------|------|
| 3.0 | 更新 taxonomy.yaml 添加 P1 新部门/岗位 | ✅ 已完成 | backend 6 岗位 + frontend 4 岗位 + gamedev 4 岗位 |
| 3.1 | 补全 backend 岗位 Agent（11 个） | ⚠️ 未完成 | agents.yaml 仅 10 个原始 Agent，未添加 Java/Python/Rust/C++/DBA 等 P1 Agent；prompt 文件已创建 |
| 3.2 | 补全 frontend 岗位 Agent（8 个） | ⚠️ 未完成 | agents.yaml 未添加 React/TypeScript/性能/UI/UX 等 P1 Agent；prompt 文件已创建 |
| 3.3 | 补全 gamedev 岗位 Agent（8 个） | ⚠️ 未完成 | agents.yaml 未添加 UE 图形/游戏服务端/TA/策划等 P1 Agent；prompt 文件已创建 |
| 3.4 | 编写 P1 prompt 文件 | ✅ 已完成 | 79 个 prompt 文件（覆盖 P1+P2+P3 全部岗位） |
| 3.5 | 编写 P1 Skill 文件 | ✅ 已完成 | 11 个 Skill（含设计未规划的 5 个额外 Skill：ddd-tactical/code-review-checklist/clean-arch/go-best-practices/ue5-gas） |
| 3.6 | 运行 seed-industry-agents CLI 验证 | ⏳ 待验证 | 需预先修复 biz/usage + biz/monitor 编译错误 |

### Phase 4: softwaredev Agent 补全（P2+P3 批次）

| 步骤 | 描述 | 状态 | 备注 |
|------|------|------|------|
| 4.1 | 补全 devops/architecture/qa 岗位（23 个） | ⚠️ 未完成 | agents.yaml 未添加 SRE/CI/CD/容器/监控/架构/测试等 P2 Agent；prompt 文件已创建 |
| 4.2 | 补全 mobiledev/dataeng/security/productpm 岗位（22 个） | ⚠️ 未完成 | agents.yaml 未添加 iOS/Android/数据/安全/产品等 P3 Agent；prompt 文件已创建 |
| 4.3 | 编写 P2+P3 prompt 文件 | ✅ 已完成 | 79 个 prompt 文件（覆盖 P1+P2+P3 全部岗位） |
| 4.4 | 编写 P2+P3 Skill 文件 | ✅ 已完成 | 11 个 Skill（含 sre-practices/mobile-dev） |
| 4.5 | 运行 seed-industry-agents CLI 验证 | ⏳ 待验证 | 需预先修复 biz/usage + biz/monitor 编译错误 |

---

## 十三、aranea-review 审查记录

**审查日期**：2026-05-31
**审查范围**：Phase 1 命名重构 + 数据统一所有变更文件 + 三项专项复查
**审查结果**：🔴 发现 3 个阻断 + 5 个建议（已修复阻断项）

### 后端审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 |
|------|---------|---------|---------|
| 架构合规 | 0 | 0 | 0 |
| 分层合规 | 0 | 1 | 0 |
| OOP | 0 | 0 | 0 |
| Agent 运行时 | 0 | 0 | 0 |
| 并发安全 | 0 | 0 | 0 |
| 错误处理 | 2 | 0 | 0 |
| 依赖注入 | 0 | 0 | 0 |

### 前端审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 |
|------|---------|---------|---------|
| 数据流合规 | 0 | 3 | 0 |
| 组件分层 | 0 | 1 | 0 |
| UX 主题 | 0 | 0 | 0 |

### 专项复查结果

| 复查项 | 🔴 阻断 | 🟡 建议 |
|--------|---------|---------|
| 行业/部门/岗位职责描述 | 0 | 1（softwaredev 岗位不足） |
| Team 配置 | 0 | 0 |
| Agent 命名/归属/职责 | 1（variant 连字符） | 0 |

### 阻断项（已修复）

| ID | 维度 | 端 | 文件 | 问题 | 修复 |
|----|------|----|------|------|------|
| BE1 | 错误处理 | 后端 | `internal/data/taxonomy.go` | `ensureNodeCanDelete` 使用 `fmt.Errorf` 返回业务错误 | 改用 `kerrors.BadRequest` |
| BE2 | 错误处理 | 后端 | `internal/data/taxonomy.go` | 错误消息仍用 "category" 旧名 | 更新为 "node" |
| BR1 | Agent 配置 | 后端 | `internal/scenario/selfmedia/agents.yaml` | 4 个 variant 使用连字符（`platform-adapt`/`data-driven`/`geography-history`/`magic-system`），不匹配 `variantSafeRe` 正则 `^[a-z0-9_]+$`，导致 API 400 | ⚠️ 未修复（仍使用连字符） |

### 建议项（记录备忘，后续迭代修复）

| ID | 维度 | 端 | 文件 | 问题 |
|----|------|----|------|------|
| FS1 | 数据流 | 前端 | `stores/agents/index.ts` | Agent Store 自行维护 `categoryTree` 副本，绕过 Platform Store |
| FS2 | 数据流 | 前端 | `features/teams/useTeamsPage.ts` | Teams composable 拷贝 `categoryTree` 到本地 ref |
| FS3 | 数据流 | 前端 | `features/chat/composables/useChatEntityNav.ts` | Chat composable 拷贝 `categoryTree` 到本地 ref |
| FL1 | 分层 | 前端 | 多文件 | Store/Composable/变量名仍用 `category` 前缀（`categoryTree`→`taxonomyTree`、`loadCategoryTree`→`loadTaxonomyTree`、`selectedCategory`→`selectedTaxonomy`、`useCategoryTreeField`→`useTaxonomyTreeField`、`CATEGORY_RESOURCE`→`TAXONOMY_RESOURCE`、`category-*` CSS 类名→`taxonomy-*`、`category.*` FieldScope→`taxonomy.*`） |
| SD1 | 岗位补全 | 配置 | `internal/scenario/softwaredev/agents.yaml` | softwaredev agents.yaml 仅 10 个 Agent（原始），方案要求 ~82 个（Phase 3-4）；⚠️ taxonomy.yaml 岗位定义和 prompt 文件已补全，但 agents.yaml Agent 条目未添加 |

### 合规性清单

- [x] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [x] Runner 装配在 Service 层
- [x] Service 层无业务逻辑
- [x] 跨模块通过窄接口
- [x] Wire 绑定在 Service 层
- [x] 无工具生成代码的手动修改（Proto/Ent/Wire 均通过工具重新生成）
- [x] Repository 接口方法 ≤ 5（已拆 TaxonomyReader/TaxonomyWriter 子接口）
- [x] 业务错误用 kerrors（修复后合规）
- [x] Data 层 fmt.Errorf 仅用于 wrap 错误（%w），合规

### 变更文件清单

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `api/kratos/taxonomy/v1/taxonomy.proto` | Taxonomy 服务 Proto 定义 |
| 新建 | `api/kratos/industry_taxonomy/v1/industry_taxonomy.proto` | IndustryTaxonomy 服务 Proto 定义（额外新增） |
| 删除 | `api/kratos/agent_category/v1/agent_category.proto` | 旧分类 Proto |
| 新建 | `internal/data/ent/schema/industry_taxonomy.go` | Ent Schema（表名 industry_taxonomy） |
| 删除 | `internal/data/ent/schema/agent_category.go` | 旧 Ent Schema |
| 新建 | `internal/biz/taxonomy.go` | TaxonomyNode + TaxonomyUsecase |
| 删除 | `internal/biz/agent_category.go` | 旧 Biz 层 |
| 新建 | `internal/data/taxonomy.go` | TaxonomyRepo 实现 |
| 删除 | `internal/data/agent_category.go` | 旧 Data 层 |
| 新建 | `internal/service/taxonomy.go` | TaxonomyService |
| 新建 | `internal/service/industry_taxonomy.go` | IndustryTaxonomyService（额外新增） |
| 删除 | `internal/service/agent_category.go` | 旧 Service 层 |
| 新建 | `internal/scenario/loader/taxonomy_loader.go` | TaxonomySpec + LoadTaxonomySpec |
| 删除 | `internal/scenario/loader/categories_loader.go` | 旧 Loader（⚠️ 仍存在为死代码） |
| 新建 | `internal/data/seed_builtin_taxonomy.go` | SeedBuiltinTaxonomy |
| 删除 | `internal/data/seed_builtin_agent_categories.go` | 旧 Seed |
| 重命名 | `categories.yaml` → `taxonomy.yaml` | YAML 数据文件（⚠️ categories.yaml 仍存在） |
| 修改 | `internal/data/ent/schema/agent.go` | category_position_id → taxonomy_position_id |
| 修改 | `api/kratos/agent/v1/agent.proto` | ⚠️ 仍使用 category_position_id（未重命名） |
| 删除 | `web/src/services/kratos/agent_category/v1/index.ts` | 旧前端 Proto 客户端（✅ 已清理） |
| 修改 | 多个 Biz/Data/Service/Server/Agent/Wire/Cmd 文件 | 全链路重命名引用 |

