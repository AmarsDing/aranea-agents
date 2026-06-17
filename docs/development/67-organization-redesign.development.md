# M67: 公司架构重塑 — 开发计划

> **版本**：2026-06-07 | **状态**：实施中
> **需求**：[67 organization-redesign.md](./67-organization-redesign.md)
> **设计**：[67 organization-redesign.design.md](./67-organization-redesign.design.md)
> **方案报告**：[2026-06-07-proposal-organization-redesign.md](../reports/2026-06-07-proposal-organization-redesign.md)

---

## 1. 模块定位

将"行业分类"重塑为"公司架构"，明确公司架构/Agent/Team/Graph 四者边界，新增部门主管和跨部门协作机制。

**代码锚点**：

| 层级 | 路径 | 阶段 | 状态 |
|------|------|------|------|
| Ent Schema | `internal/data/ent/schema/organization.go` | 1 | ✅ |
| Ent Schema | `internal/data/ent/schema/agent.go` | 1 | ✅ |
| Ent Schema | `internal/data/ent/schema/team.go` | 1 | ✅ |
| Ent Schema | `internal/data/ent/schema/graph_definition.go` | 1 | ✅ |
| Proto | `api/kratos/organization/v1/organization.proto` | 1 | ✅ |
| Biz | `internal/biz/organization.go` | 1 | ✅ |
| Biz | `internal/biz/dept_lead.go` | 2 | ✅ |
| Biz | `internal/biz/deliverable_contract.go` | 3 | ✅ |
| Biz | `internal/biz/verification_gate.go` | 3 | ✅ |
| Biz | `internal/biz/rework.go` | 3 | ✅ |
| Biz | `internal/biz/spirit_team_usecase.go` | 3 | ✅ |
| Biz | `internal/biz/team_usecase.go` | 1 | ✅ |
| Service | `internal/service/organization.go` | 1 | ✅ |
| Service | `internal/service/taxonomy.go`（兼容层） | 6 | ✅ |
| Scenario | `internal/scenario/organization.yaml` | 4 | ✅ |
| Scenario | `internal/scenario/loader/organization_loader.go` | 4 | ✅ |
| Scenario | `internal/scenario/loader/company_loader.go` | 4 | ✅ |
| Scenario | `internal/scenario/loader/spec.go` | 4 | ✅ |
| Scenario | `internal/scenario/system/prompts/dept_lead.md` | 2 | ✅ |
| 前端 | `web/src/features/platform/` | 5 | 🟡 部分完成 |
| 迁移 | `internal/data/sql/migrations/` | 6 | ⏳ 未实施 |

---

## 2. 开发阶段

### Phase 1 — 数据模型 + OrganizationService（P0）

> **目标**：完成核心数据模型变更和 OrganizationService CRUD，为后续阶段打基础。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| ORG-01 | 重命名 Ent Schema: `industry_taxonomy.go` → `organization.go`，字段变更（level 值、新增 dept_lead 字段、taxonomy_key → org_key） | `internal/data/ent/schema/organization.go` | `make api && go build ./...` 通过 | ✅ |
| ORG-02 | 重命名 Agent Schema 字段: `taxonomy_position_id` → `position_id` | `internal/data/ent/schema/agent.go` | `make api && go build ./...` 通过 | ✅ |
| ORG-03 | 修改 Team Schema: `category_industry_id` → `department_id`，新增 `deliverables`/`input_contract`/`dept_lead_agent_id`/`cross_dept_member_ids`/`linked_graph_id` | `internal/data/ent/schema/team.go` | `make api && go build ./...` 通过 | ✅ |
| ORG-04 | 修改 Graph Schema: 新增 `team_id`/`is_template`/`verification_gates` | `internal/data/ent/schema/graph_definition.go` | `make api && go build ./...` 通过 | ✅ |
| ORG-05 | 新增 `api/kratos/organization/v1/organization.proto`，定义 OrganizationService + OrganizationNode message | `api/kratos/organization/v1/organization.proto` | `make api` 通过 | ✅ |
| ORG-06 | 修改 agent.proto: `taxonomy_position_id` → `position_id` | `api/kratos/agent/` | `make api` 通过 | ✅ |
| ORG-06b | 修改 agent.proto: ListAgentsRequest 中 `category_id = 4` rename 为 `org_node_id`，保持编号 4 不变 | `api/kratos/agent/` | `make api` 通过 | 🟡 待核实 |
| ORG-07 | 修改 team.proto: 新增 `department_id`/`deliverables`/`input_contract`/`dept_lead_agent_id` | `api/kratos/team/` | `make api` 通过 | ✅ |
| ORG-08 | 修改 graph.proto: 新增 `team_id`/`is_template`/`verification_gates` | `api/kratos/graph/` | `make api` 通过 | ✅ |
| ORG-09 | Biz 层: `taxonomy.go` → `organization.go`，TaxonomyUsecase → OrganizationUsecase，类型重命名 | `internal/biz/organization.go` | 单测绿 | ✅ |
| ORG-09b | OrganizationRepo 接口拆分为: OrganizationReader + OrganizationWriter + GetOrgNodeByKeyAnyState | `internal/biz/organization.go` | 接口编译通过 | ✅ |
| ORG-10 | Service 层: 新增 `organization.go`，实现 OrganizationService | `internal/service/organization.go` | `make wire && go build ./cmd/admin` 通过 | ✅ |
| ORG-11 | Data 层: OrganizationRepo 实现（原 TaxonomyRepo） | `internal/data/` | 单测绿 | ✅ |
| ORG-11b | 双向引用回写: Team.linked_graph_id 变更时自动回写 Graph.team_id | `internal/biz/team_usecase.go` | ✅ 回写逻辑已实现（syncGraphTeamID） | ✅ |
| ORG-11c | Team 删除时 Graph 清理: 专属 Graph 随 Team 删除，模板 Graph 仅清除 team_id | `internal/biz/team_usecase.go` | ✅ 删除逻辑已实现（Delete 方法中 Graph 清理） | ✅ |
| ORG-12 | Wire 注入更新: biz.ProviderSet、data.ProviderSet、service 层引用、agent.builder_deps.go、chat_orchestrator.go 中所有 TaxonomyUsecase/TaxonomyRepo 引用 | `cmd/admin/wire.go` | `make wire && go build ./cmd/admin` 通过 | ✅ |
| ORG-13 | 更新 Seed SQL: 4 个 agent seed 函数中的 `taxonomy_position_id` 列名改为 `position_id` | `internal/data/seed_system_admin.go` | 种子执行成功 | ✅ |
| ORG-14 | Pack 导入: `SeedPackIndustry` 函数名保留（内部调用 `LoadCompanySpec`），`IndustrySpec` → `CompanySpec`（`IndustryKey` → `CompanyKey`），`LoadOrganizationSpec` + `LoadCompanySpec` 新增加载器 | `internal/data/seed_pack.go`、`internal/scenario/loader/` | Pack 导入成功 | 🟡 部分完成（`pack.ConvertCompanySpecToPack` 未实现） |

**Phase 1 验收**：`make api && make wire && make build && make test` 全部通过

---

### Phase 2 — 部门主管（P0）

> **目标**：实现部门主管 Agent 的自动创建、自动加入 Team、Prompt 模板。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| DL-01 | 实现 `DeptLeadManager`：`CreateDeptLead`/`DeleteDeptLead`/`ReplaceDeptLead` | `internal/biz/dept_lead.go` | 单测绿 | ✅ |
| DL-02 | 部门主管 Prompt 模板: `internal/scenario/system/prompts/dept_lead.md` | `internal/scenario/system/prompts/dept_lead.md` | 文件存在且内容合理 | ✅ |
| DL-02b | 部门主管 tools_profile 定义: 初期为空（纯 LLM 判断），后续可添加 query_team_status/review_deliverable/escalate_to_spirit | `internal/data/seed_system_admin.go` | tools_profile 设置正确 | 🟡 待核实 |
| DL-03 | 部门主管种子数据: 在 `seed_system_admin.go` 中为现有部门生成主管 | `internal/data/seed_system_admin.go` | 种子执行成功 | 🟡 待核实 |
| DL-04 | OrganizationUsecase.Create 集成: 创建部门时自动调用 DeptLeadManager | `internal/biz/organization.go` | 创建部门后 dept_lead_agent_id 非空 | ✅ |
| DL-05 | OrganizationUsecase.Delete 集成: 删除部门时级联删除主管 | `internal/biz/organization.go` | 删除部门后主管 Agent 被清理 | ✅ |
| DL-06 | SpiritTeamUsecase 集成: Team 组建时自动注入部门主管 | `internal/biz/spirit_team_usecase.go` | Team.members 包含部门主管 | 🟡 待核实 |
| DL-07 | 部门主管保护: AgentUsecase 中禁止删除 kind=system_builtin 且 key 以 `__dept_lead_` 开头的 Agent | `internal/biz/agent_usecase.go` | 删除操作被拒绝 | ✅ |
| DL-07b | 部门删除级联: 有活跃 Team 时阻止删除；删除时归档非活跃 Team、解除 Agent 岗位关联、级联删除岗位和主管 | `internal/biz/organization.go` | ✅ 7步级联逻辑已实现（deleteDepartmentWithCascade） | ✅ |
| DL-08 | Team Schema 新增 `cross_dept_member_ids` 字段（text, 默认 "[]"） | `internal/data/ent/schema/team.go` | `make api && go build ./...` 通过 | ✅ |
| DL-09 | 跨部门成员加入审批: 非本部门 Agent 加入 Team 时需其部门主管自动审批 | `internal/biz/dept_lead.go` | 跨部门成员可加入 | ✅ |
| DL-10 | 实现 `BorrowRequest` 类型和 `DeptLeadManager` 借调审批方法: `SubmitBorrowRequest`/`ApproveBorrowRequest`/`RejectBorrowRequest`/`AutoApproveExpiredBorrowRequests` | `internal/biz/dept_lead.go` | ✅ 借调审批方法已实现 + BorrowRequest Ent Schema + Data 层持久化 | ✅ |
| DL-11 | 借调审批门禁: `GateTypeBorrowApproval` 类型，集成到 VerificationGateExecutor | `internal/biz/verification_gate.go` | ✅ 借调审批流程已实现，LLM 解析失败默认拒绝 | ✅ |
| DL-12 | 借调比例校验: 跨部门成员不超过 Team 总人数 50%（`maxCrossDeptRatio = 0.5`） | `internal/biz/dept_lead.go` | 超限被拒绝 | ✅ |
| DL-13 | 借调监督: `GetBorrowedMemberStatus` 方法，部门主管可查看被借调成员工作状态（只读） | `internal/biz/dept_lead.go` | 主管可查看借调成员状态 | ✅ |

**Phase 2 验收**：创建部门 → 自动生成主管 → 组建 Team → 主管自动加入 → 跨部门借调 → 借调审批通过/拒绝/超时 → 删除部门 → 主管级联删除

---

### Phase 3 — 跨部门协作（P1）

> **目标**：实现交付物契约验证、审批门禁、驳回返工流程。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| XC-01 | 实现 `DeliverableContract` 类型和 `DeliverableContractValidator` | `internal/biz/deliverable_contract.go` | 单测绿（匹配/不匹配/警告场景） | ✅ |
| XC-02 | 实现 `VerificationGate` 类型和 `VerificationGateExecutor` | `internal/biz/verification_gate.go` | 单测绿（通过/驳回/超重试升级） | ✅ |
| XC-02b | 实现 `CrossDeptDeliveryGate` 双方审批：输出方主管质量把关 + 接收方主管验收确认，两方都通过才传递交付物 | `internal/biz/verification_gate.go` | ✅ 双方审批已实现，dept lead 缺失返回错误，LLM 解析失败默认拒绝 | ✅ |
| XC-03 | Spirit 编排管线适配: TaskOrchestrator 扩展，组建 Team DAG 后验证契约 + 注入主管 + 添加门禁 | `internal/biz/spirit_team_usecase.go` | 跨部门任务自动组建 DAG + 契约验证 | 🟡 待核实 |
| XC-03b | 交付物传递机制: 上游 Team 输出写入 Spirit Session 共享 Memory，DAG 调度激活下游时作为 User Message 前缀注入（含来源团队、交付物名称、内容），后续迭代支持注入 Graph StateFields | `internal/biz/spirit_team_usecase.go` | ✅ WriteDeliverablesToSession 持久化到 ParallelConfigJSON + InjectUpstreamDeliverables 优先读取缓存 | ✅ |
| XC-03c | 借调审批事件: BorrowApproved/BorrowRejected/BorrowAutoApproved 事件发布 | `internal/event/` | 事件发布成功 | 🟡 待核实 |
| XC-04 | 审批驳回返工: 部门主管驳回后标记 Team 为 pending（通过 TransitionStatus），清除执行结果，重新执行整个 Team（初期策略） | `internal/biz/spirit_team_usecase.go` | ✅ HandleTeamRejection 增加 TransitionStatus 调用触发重执行 | ✅ |
| XC-04b | ReworkStrategy 类型和常量定义（初期仅实现 full_team） | `internal/biz/rework.go` | 类型定义正确 | ✅ |
| XC-05 | 升级处理: 超过 max_retries 后升级给精灵助手（EscalateToSpirit，标记 Team 状态为 failed） | `internal/biz/spirit_team_usecase.go` | 升级事件发布成功 | ✅ |
| XC-06 | Team API 扩展: deliverables/input_contract CRUD | `internal/service/team.go` | API 可读写交付物契约 | 🟡 待核实 |
| XC-07 | Graph API 扩展: verification_gates CRUD | `internal/service/graph.go` | API 可读写审批门禁 | 🟡 待核实 |

**Phase 3 验收**：跨部门任务 → DAG 组建 → 契约验证 → 执行 → 主管审批 → 驳回返工 → 重试 → 通过 → 合成结果

---

### Phase 4 — Scenario 适配（P1）

> **目标**：适配 Scenario YAML 结构，更新种子数据和加载器。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| SC-01 | 重命名 `taxonomy.yaml` → `organization.yaml`，结构从 `industries` 改为 `companies`（含 `departments`/`positions` 子结构） | `internal/scenario/organization.yaml` | 加载器解析成功 | ✅ |
| SC-02 | 更新 `TaxonomyLoader` → `OrganizationLoader` + 新增 `CompanyLoader`，适配新 YAML 结构（OrganizationSpec.Companies 顶层 key） | `internal/scenario/loader/organization_loader.go`、`internal/scenario/loader/company_loader.go`、`internal/scenario/loader/spec.go` | 种子数据导入成功 | ✅ |
| SC-03 | 更新各场景 `agents.yaml`：`position_key` 语义适配（替代原 `taxonomy_position_key`） | `internal/scenario/softwaredev/agents.yaml` 等 | 种子数据导入成功 | ✅ |
| SC-04 | 更新各场景 `teams.yaml`：`department_id` 替代 `category_industry_id` | `internal/scenario/softwaredev/` 等 | 种子数据导入成功 | 🟡 待核实（未发现 teams.yaml 文件，可能通过其他方式定义） |
| SC-05 | 部门主管 Prompt 种子: `dept_lead.md` 模板文件（含 `{{.DepartmentName}}`/`{{.DepartmentDescription}}` 模板变量） | `internal/scenario/system/prompts/dept_lead.md` | Prompt 文件写入成功 | ✅ |

**Phase 4 验收**：清空数据库 → 种子数据导入 → 公司架构树完整 → 部门主管存在 → Team 归属部门正确

---

### Phase 5 — 前端适配（P1）

> **目标**：前端页面和组件适配新概念。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| FE-01 | 重命名 `features/industries/` → `features/organization/`，类型重命名 | `web/src/features/` | `pnpm lint` 通过 | ⏳ 未实施（前端仍在 `web/src/features/platform/` 下，未重命名为 organization/） |
| FE-02 | API 层适配: `TaxonomyService` → `OrganizationService`（同时保留 `createTaxonomyService`/`createIndustryTaxonomyService` 兼容） | `web/src/features/platform/api.ts` | API 调用成功 | 🟡 部分完成（api.ts 已支持 `createOrganizationService`，但旧 service 未移除） |
| FE-03 | 页面重命名: `TaxonomyPage` → `OrganizationPage` 等 | `web/src/pages/OrganizationPage.vue` | 页面渲染正常 | ✅ |
| FE-04 | 路由兼容: 旧 `/settings/taxonomy` 重定向到 `/settings/organization` | `web/src/router/routes.ts` | 旧 URL 可访问 | ✅ |
| FE-05 | i18n 更新: zh-CN ~18 处 + en-US ~11 处，key 前缀 taxonomy→organization、industry→company/department | `web/src/i18n/locales/zh-CN.ts`、`web/src/i18n/locales/en-US.ts` | 文案正确 | 🟡 部分完成（已有 `organization` 条目，但 `statCategories` 等旧 key 未重命名） |
| FE-05b | i18n 条目重命名: industryMarket→orgMarket, statCategories→statDepartments 等 | `web/src/i18n/` | key 引用无断裂 | ⏳ 未实施（仍为 `statCategories`） |
| FE-06 | 新增 `DeptLeadConfigDialog.vue`: 部门主管配置 | `web/src/features/platform/` | 可配置部门主管 | ⏳ 未实施 |
| FE-07 | 新增 `TeamDeliverableEditor.vue`: Team 交付物编辑 | `web/src/features/teams/` | 可编辑交付物契约 | ⏳ 未实施 |
| FE-08 | 新增 `VerificationGateConfig.vue`: 审批门禁配置 | `web/src/features/graph/` | 可配置审批门禁 | ⏳ 未实施 |
| FE-09 | 组织架构树页面优化: 显示 Agent 数量、Team 数量 | `web/src/pages/OrganizationPage.vue` | 数据展示正确 | ⏳ 未实施 |
| FE-10 | 跨部门成员借调 UI: Team 组建时跨部门选人 + 借调审批状态展示 | `web/src/features/teams/` | 可选跨部门成员，显示审批状态 | ⏳ 未实施 |
| FE-11 | 借调审批通知: 部门主管收到借调请求通知，可审批/拒绝 | `web/src/features/platform/` | 主管可审批借调 | ⏳ 未实施 |
| FE-12 | CrossDeptDAGView.vue: 跨部门 Team DAG 可视化组件 | `web/src/features/teams/components/` | DAG 可视化渲染正确 | ⏳ 未实施 |
| FE-13 | BorrowApprovalDialog.vue: 借调审批对话框 | `web/src/features/platform/components/` | 主管可审批/拒绝 | ⏳ 未实施 |
| FE-14 | BorrowStatusBadge.vue: 借调审批状态徽章 | `web/src/features/teams/components/` | 状态显示正确 | ⏳ 未实施 |

**Phase 5 验收**：`pnpm lint && pnpm test && pnpm build` 通过，前端功能正常

---

### Phase 6 — 数据迁移 + 兼容层（P0）

> **目标**：实现从旧数据到新数据的无损迁移，保持 API 兼容。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| MG-01 | 编写 Ent 自动迁移: 表重命名 + 字段重命名 + 新增字段（通过 `Schema.Create()` 实现） | `internal/data/ent/schema/organization.go` 等 | `make build` 通过 | ✅ |
| MG-02 | 编写数据迁移脚本: `level="industry"` → `"company"`，`category_industry_id` → `department_id` 映射 | `internal/data/sql/migrations/` | 迁移后数据正确 | ⏳ 未实施（未发现 organization 相关 DDL 迁移脚本） |
| MG-03 | 为现有部门自动创建部门主管 Agent | `internal/data/` | 每个部门有主管 | ⏳ 未实施 |
| MG-04 | 实现 API 兼容层: 旧 `IndustryTaxonomyService` 代理到新 `OrganizationService` | `internal/service/taxonomy.go` | 旧 API 可用 | ✅ |
| MG-05 | 实现 API 兼容层: 旧 `TaxonomyService` 代理到新 `OrganizationService` | `internal/service/taxonomy.go` | 旧 API 可用 | ✅ |
| MG-06 | 前端路由兼容: 旧 `/settings/taxonomy` 重定向（MG-06 与 FE-04 合并，在 Phase 5 中完成） | `web/src/router/routes.ts` | 旧 URL 可访问 | ✅ |
| MG-07 | 迁移回滚脚本 | `internal/data/` | 可回滚到迁移前状态 | ⏳ 未实施 |

**Phase 6 验收**：在现有生产数据上执行迁移 → 新 API 正常 → 旧 API 兼容 → 数据无损 → 可回滚

---

## 3. 依赖关系

```
Phase 1 (数据模型 + Service)
  │
  ├── Phase 6 (迁移 + 兼容) ← 依赖 Phase 1 完成
  ├── Phase 2 (部门主管) ← 依赖 Phase 1
  │     │
  │     └── Phase 3 (跨部门协作) ← 依赖 Phase 2
  │           │
  │           └── Phase 4 (Scenario 适配) ← 依赖 Phase 1-3
  │
  └── Phase 5 (前端适配) ← 依赖 Phase 1（FE-06~11 依赖 Phase 2/3）
```

**可并行**：Phase 2 和 Phase 5 的基础部分（FE-01~05）可并行；Phase 4 和 Phase 6 可并行。Phase 6 必须在 Phase 1 完成后启动。

---

## 4. 风险与缓解

| 风险 | 阶段 | 缓解 |
|------|------|------|
| Team.department_id 迁移映射错误 | Phase 6 | 迁移前备份，提供回滚脚本，先在测试环境验证 |
| 部门主管审批阻塞业务 | Phase 3 | 设置超时自动通过（可配置）+ 最大重试升级 |
| Ent 表重命名导致迁移失败 | Phase 1 | 使用 Ent auto migration 而非手动 SQL |
| 前端重命名导致功能回归 | Phase 5 | 逐页面迁移，保留旧路由重定向 |
| Spirit 编排管线适配引入 bug | Phase 3 | 先写失败测试，再改生产代码 |
| Proto 字段编号兼容性 | Phase 1 | 所有 rename 字段保持原编号不变，Protobuf wire format 只认编号 | 编号变更会导致旧客户端反序列化失败 |
| 跨部门成员权限管理 | Phase 2 | 跨部门成员加入需自动审批，初期简化为自动通过 | 未授权的跨部门访问 |
| 交付物传递丢失 | Phase 3 | Spirit Session Memory 持久化，DAG 调度前验证上游交付物存在 | 下游 Team 缺少输入 |

---

## 5. 验证策略

| 阶段 | 验证命令 |
|------|----------|
| Phase 1 | `make api && make wire && make build && make test` |
| Phase 2 | `go test ./internal/biz/... -run TestDeptLead -count=1` |
| Phase 3 | `go test ./internal/biz/... -run TestDeliverableContract -count=1 && go test ./internal/biz/... -run TestVerificationGate -count=1` |
| Phase 4 | 清空 DB → 种子导入 → 验证数据完整性 |
| Phase 5 | `cd web && pnpm lint && pnpm test && pnpm build` |
| Phase 6 | 迁移 → 新 API 测试 → 旧 API 兼容测试 → 回滚测试 |
| **全量** | 后端: `make api && make wire && make build && make test && make lint`；前端: `cd web && pnpm lint && pnpm test && pnpm build` |
