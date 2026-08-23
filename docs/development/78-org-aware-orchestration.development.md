# M78: 组织感知编排（ORG-FAST）— 开发计划

> **版本**：2026-08-22 | **状态**：🟡 Phase 0–2 已落地；Phase 4 核心（分档/总经理挂岗/剧本热路径/前缀预算/三管道/确认五档 HITL/`graph_template_id` 改道）已落地；Phase 3 跨公司 Brief 仍 YAGNI
> **需求**：[78-org-aware-orchestration.md](./78-org-aware-orchestration.md)
> **设计**：[78-org-aware-orchestration.design.md](./78-org-aware-orchestration.design.md)
> **ADR**：[ORG-FAST](../reports/2026-08-22-review-adr-org-aware-orchestration.md) · [重型链](../reports/2026-08-22-review-adr-org-heavy-chain.md) · [横切](../reports/2026-08-22-review-org-heavy-chain-crosscut.md)

---

## 1. 模块定位

在现有 Plan → Allocate → Orchestrate 上补齐 **组织剪枝 + 主管排除 + 建团挂部门 + 创造分层 + 跨团队 Brief/Bulk 交接**，并落地 **重型档：分档 + 公司剧本展开 + 前缀预算**。确认五档、花名册工具面、`graph_template_id` 已接生产路径。不新建服务进程。

**代码锚点（现状）：**

| 层级 | 路径 | 现状 |
|------|------|------|
| Allocator | `internal/agent/agent_allocator_impl.go` | L0 → 花名册 → L1（需真实相似）+ OrgPrune；主管排除；L3 ≤15；BuildAll 分页不再卡 200 |
| 能力表 | `internal/agent/agent_capability_builder.go` | 填充岗位/部门/公司/variant + 空 domain_path/mission 推断；系统 Agent 仍过滤 |
| 能力 DTO | `internal/biz/agent_capability.go` | 已含组织字段 |
| 域词表 | `internal/agent/domain_lexicon.go` | 软件/运维/商务/医疗等二级域已扩；部门映射见 `org_domain_map.go` |
| 花名册推断 | `internal/biz/org_roster.go` + `internal/data/seed_roster.go` | 按岗位/部门回填 `domain_path`/`mission`/`tools_profile`；启动 seed `data.seed.roster_identity` |
| 配方缓存 | `internal/biz/spirit_orchestration_cache.go` | L0 已可用 |
| 建团 | `internal/service/team_orchestrator_real.go` | 透传 DepartmentID + CrossDeptMemberAgentIDs |
| 组装 | `internal/biz/spirit_assembly.go` | 有 DepartmentID 时主管加入/借调 |
| 主管 | `internal/biz/dept_lead.go` | CRUD/自动加入已实现 |
| 组织 | `internal/biz/organization.go` | 树 CRUD 已实现 |
| 工具入口 | `internal/tools/spirit_tools.go` | plan_and_execute 三阶段 |
| 门控 | `internal/service/pre_planning_gate.go` | 简单任务强制规划 |
| 决策提示 | `internal/scenario/system/prompts/DECISION.md` | 澄清优先、三种 mode |
| Factory | `internal/agent/agent_factory.go` | 确认创建；DepartmentID 已知时占岗 |
| 结论信封 | `internal/biz/spirit_delivery.go`、`team_types.go` `DeliverableRef` | Brief + Bulk 指针；超阈文本不进 StructuredJSON；inbox 物化 |
| 长文按需 | `read_upstream_deliverable` | 文本 50k/200k；非文件通道 |
| 制品 | `internal/artifact/`、`internal/service/artifact.go` | 会话级制品，**未**挂进跨团队信封指针 |
| 监管目录 | `internal/biz/resource_access.go` memberfs | 主管只读；员工沙箱仍隔离 |

---

## 2. 现状评估（程序逻辑，2026-08-22）

### 2.1 已经做对的（保持）

| 能力 | 证据 | 对「快且准」的贡献 |
|------|------|-------------------|
| 简单/澄清不组队 | DECISION.md + PrePlanningGate | 最快路径：0 建团 |
| 非 idle 复用 | `reuse_existing` | 避免重复分解 |
| 领域配方 L0 | `BestRecipeForDomain` DQ≥0.7 | 重复任务 0 LLM 匹配 |
| 使命 L1 | domain 收敛 + 履历 | 同类任务复用 Agent |
| Allocate 两阶段并行 | P-ORCH.5 Phase A | 降低冷启动墙钟 |
| Factory 用户确认 | P-ORCH | 防止静默造人 |
| DAG 唯一建团路径 | PlanExecutor + RealTeamOrchestrator | 禁止旧 Orchestrate 双路径 |
| 编制表与主管模型 | M67 | 准度的数据基础已在，热路径未接上 |

### 2.2 缺口（按对快/准的伤害排序）

| ID | 缺口 | 伤害 | 修复落点 |
|----|------|------|----------|
| G1 | `BuildAll` 不过滤 `dept_lead`，主管进入匹配池 | **准**：主管被选为业务 Lead（2026-07-27 实测 `__dept_lead_media_operations__`） | Allocator + CapabilityBuilder |
| G2 | `selectAdditionalMembers` 顺序抓人 | **准**：团队凑数、跨部门、无互补 | Allocator |
| G3 | `RealTeamOrchestrator` 不写 `DepartmentID` | **准**：M67 主管加入/借调/审批全灭 | team_orchestrator_real.go |
| G4 | 匹配无组织剪枝 | **快+准**：200 人海选；L3 对全库 LLM | OrgPruner |
| G5 | Factory 不强制 `position_id` | **准**：新人生而无岗，下次仍无法剪枝 | agent_factory.go |
| G6 | 无「禁止任务创建组织」锁 | **准/治理**：后续易按最初设想误实现 | 测试 + 代码注释/断言 |
| G7 | 主管条件介入未做 | **快**：若按最初设想每单问主管会变慢；现状是完全不问，缺编时只能 Factory | P2 staffing |
| G8 | 进度事件无部门名 | 体验 | P1 payload |
| G9 | `DeliverableArtifact` 只描述 state key 字符数，不是文件 | **快+准**：大量文件只能写进摘要路径或灌 StructuredJSON | `team_types.go` + delivery + dispatch 物化 |
| G10 | B.10.15.3 文件/二进制「远期」未收口 | 下游要么看不见附件，要么把仓库塞进 prompt | Bulk 通道 §十一 |
| G11 | 易把 memberfs 当成跨团队传文件 | **准/安全**：未声明文件被读；与 M71 原则冲突 | 规则 R7 + 测试锁死员工不可 memberfs |

### 2.3 最初设想 vs 现状 vs 目标

```
最初设想：任务 → 找公司(LLM) → 找部门(LLM) → 主管分解(LLM) → 派 Agent → 没有则创造公司/人
现状：    任务 → Plan 分解(LLM) → 全库 L0–L3 → 建团(无部门) → 没有则 Factory
目标：    分档 → 轻不组队 / 中花名册+部门门禁 / 重剧本展开+三管道
          → Org-Prune → 花名册绑人 → 建团(有部门) → 新开 Team（不复活旧行）
          → 下游只吃 Brief；文件走信封指针 + inbox
          部门领导/总经理只在门禁、例外、跨公司 Brief、冲突时出现
```

---

## 3. 开发阶段

### Phase 0 — 准度急救（P0，不改协议）

> 目标：立刻消灭「主管当 Lead、建团无部门、补员乱抓」。不引入新 LLM。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| ORGFAST-01 | 匹配池排除 `IsDeptLeadAgent`；`IsCatalogAgentAssignable` 文档化「主管不可分配」；explicit `agent_keys` 仍可指定主管 | `agent_capability_builder.go`、`agent_allocator_impl.go` | 单测：主管不出现在 AssignedKey / 补员；explicit 路径保留 | ✅ |
| ORGFAST-02 | `BuildAll` 填充 Position/Department/Company/Variant（批量查组织祖先） | `agent_capability.go`、builder、organization reader | 有岗位 Agent 的 DepartmentID 非空；无岗位不报错 | ✅ |
| ORGFAST-03 | `selectAdditionalMembers` 改为同部门 + 角色不与 Lead 完全重复；不足再跨部并标记 | `agent_allocator_impl.go` | 单测：同部门优先；跨部进入 CrossDept 字段 | ✅ |
| ORGFAST-04 | `RealTeamOrchestrator` 写入 `DepartmentID`（step 透传或成员岗位多数票）+ `CrossDeptMemberAgentIDs` | `team_orchestrator_real.go`、`PlanStep`/`TaskAllocation` 透传 | 组装后 Team.DepartmentID 非空（成员有岗时）；主管自动加入单测或现有 assembly 测被激活 | ✅ |
| ORGFAST-05 | 断言：Allocate/Factory 测试中 mock OrganizationWriter 创建 company/department 次数 = 0 | 测试 | 锁定 G6 | ✅ |

**Phase 0 验收**：`go test ./internal/agent/ ./internal/service/ -count=1` 相关包绿；dept_lead 不再成为默认 Lead。

### Phase 1 — 又快又准的剪枝（P1）

> 目标：L3 只对 ≤15 人；部门 Top-1 可测。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| ORGFAST-10 | 实现 `OrgPruner` + `org_domain_map.go` | `internal/agent/org_pruner.go` 等 | 单测映射/FallbackAll/空域 | ✅ |
| ORGFAST-11 | `matchSubTask`/`matchWholePlan`/`llmColdStart` 使用剪枝集 | `agent_allocator_impl.go` | L3 候选数断言；剪枝空 Warn 后回退 | ✅ |
| ORGFAST-12 | Factory 出生占岗：profile 带 DepartmentID 时解析默认 position | `agent_factory.go` | 新 Agent position_id 非空或明确走「其他」岗 | ✅ |
| ORGFAST-13 | MatchReason / orchestration_progress 带部门 | allocator 进度事件 | 日志字段 `department_id`，无字符串拼接 | ✅ |
| ORGFAST-14 | 20 条固定中文任务评测夹具（`internal/agent/org_pruner_eval_test.go`） | test/ | 部门 Top-1 ≥ 90%；主管误选 Lead = 0 | ✅ |

**Phase 1 验收**：NFR-78-01/02/04 在夹具上可复现；B.10.21 旧测全绿。

### Phase 1b — 交接双通道（P1，可与剪枝并行）

> 目标：结论继续走现有信封；文件不再进 prompt。不新建 blob 表。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| ORGFAST-15 | `DeliverableArtifact` 增 `Kind/SizeBytes/SHA256/ArtifactID/RelPath/MimeType`，旧 JSON 缺省 `state_key` | `internal/biz/team_types.go` + 解析单测 | 旧信封仍能 Parse | ✅ |
| ORGFAST-16 | 写信封时：工作区已声明文件登记 RelPath/制品 ID，禁止把大文件写入 StructuredJSON 正文 | `spirit_delivery.go` | 超阈文本不进 StructuredJSON 全量；清单完整 | ✅ |
| ORGFAST-17 | 下游 `StartTeamTurn` 前按信封物化 `inbox/<upstream_team_id>/`，前缀只追加清单 | `BuildTeamTurnInput` + `TeamInboxFS` | 磁盘集合 = 声明集合；前缀无文件正文 | ✅ |
| ORGFAST-18 | 交付协议文案：必须写 summary；大文件列附件 | `DeliverableProtocolSuffix` | DAG 团队 prompt 含 Brief/Bulk 规则 | ✅ |
| ORGFAST-19 | 回归：无结论仍 fail-closed；`read_upstream_deliverable` 行为不变（文本） | 既有 deliverable 测 | 全绿 | ✅ |
| ORGFAST-19b | 配方回放：MemoryHit + AgentKeys → AllocateExplicit（初版单槽；ORGFAST-25 改为专题多槽） | planner + `plan_and_execute` | 跳过分解/匹配 LLM；仍新建 Team | ✅ |

### Phase 2 — 主管治理（P2）

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| ORGFAST-20 | 高置信路径主管 LLM = 0（默认已满足，加指标计数） | `metrics.OrgFastDeptLeadTotal` | L0/L1 记 `skipped_high_confidence` | ✅ |
| ORGFAST-21 | `staffing`：剪枝无人过阈值时可选问主管一次，超时 fail-closed | `staffing.go` + allocator Phase B | 超时不创建 Agent；不二次全局分解 | ✅ |
| ORGFAST-23 | 热路径关闭 Factory / 低分交差 | `agent_allocator_impl.go` | 默认 miss → error；`allowFactoryCreate` 才创建 | ✅ |
| ORGFAST-24 | 专项花名册绑定 | `specialty_roster.go` | `domain_path` → primary+backup；按岗位 token 计分；dept_lead 不可选 | ✅ |
| ORGFAST-25 | 配方按专题槽位回放 | planner + cache `Specialties` | 多 SubTask，不再压成单 st_recipe_replay | ✅ |
| ORGFAST-26 | 评测：原话 → 专题 → 人 | `specialty_roster_eval_test.go` | 20 条；人在花名册；无 Factory | ✅ |
| ORGFAST-27 | 多团队协作可解释（协议统一，非主管 Lead） | `collaboration.go` + progress `collaborating` | 2+ 槽位发出 sketch；Brief+精灵汇总 | ✅ |
| ORGFAST-28 | 组队过程前端可观测 | allocating meta + PlanStep 花名册字段 + 团队卡片 | 专题→人；缺编 allocate_failed | ✅ |
| ORGFAST-22 | 借调 vs DAG 仅标注借调（不重写 Plan） | allocator | 跨部成员写入 CrossDept；Plan.SubTasks 不变 | ✅ |

### Phase 3 — 多公司（P3，YAGNI 直到产品要）

| ID | 任务 | 状态 |
|----|------|------|
| ORGFAST-30 | workspace 切换即换公司树；任务内不检索公司 | ⏸ 明确不做，直到多公司产品需求立项 |
| ORGFAST-31 | 跨公司总经理 Brief（树内 2+ 公司节点） | ⏸ 契约见设计 §13.3；无第二公司则休眠 |

### Phase 4 — 重型组织链（P1，设计已复审）

> 目标：分档 + 公司剧本预授权 + 三管道。不把四层做成每步 LLM。依赖 M67 增 `company_lead`。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| ORGFAST-40 | 分档器：light/medium/heavy 规则 + 单测 | `internal/agent` 或 gate | 轻不组队；中不叫醒总经理；跨部门 DAG→heavy | ✅ |
| ORGFAST-41 | M67：`company_lead` 幂等创建/排除出 Assignable | `company_lead.go` + capability | 总经理不得为 Team Lead；挂「总经理」岗；Tree/List 回填；编制区可见 | ✅ |
| ORGFAST-42 | 流程剧本读写（公司 metadata）+ 预授权 | organization + planner | 任务点名已授权剧本 → 展开 0 总经理 LLM；组织链无名剧本 fail-closed（`playbook_fill_required`）+ `AuthorizeCompanyPlaybook` 沉淀 | ✅ |
| ORGFAST-43 | 精灵粗路由只输出 playbook_id（重型） | planner prompt / 分类 | 不按行业常识拆到岗 | ✅ DECISION.md + `next_action=authorize_playbook` |
| ORGFAST-44 | 三管道：上行心跳/例外事件；横向仍 Brief | delivery + progress | 上行 ≤2KB；无源码 | ✅ PlanExecutor 阶段开工/完成发 heartbeat，失败发 upward；非调度栅栏 |
| ORGFAST-45 | 配方约束指纹 | orchestration cache | 指纹不合不复用 keys | ✅ planner 不合则丢 keys；Allocate 再滤领导 |
| ORGFAST-46 | checkpoint 记 playbook/授权阶段/Brief | 对齐 M70 | 旧 checkpoint 缺省可恢复 | ✅ 只快照 `executing` 计划；resume 读回 playbook/阶段/`IssuedBriefIDs`（= 授权阶段 ID）并写入续跑 prompt |
| ORGFAST-47 | 仲裁：部门→总经理，公司→精灵呈用户 | 门禁/事件 | 禁止总经理互怼循环 | 📋 |

### Phase 4b — 链路横切（P1，与 Phase 4 同批设计、可并行实施）

> 目标：速度按 token 算；管道/存储不混用；确认少、干预复用现网。不新造 Graph/共享盘/报告卡片。

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| ORGFAST-50 | 成员首轮前缀预算（R14） | assembly / inject | 四段合计 ≤6KB；超限先砍知识 | ✅ Brief+协议封顶；知识/记忆按需工具，不预灌 |
| ORGFAST-51 | 花名册 tool/MCP 允许集 | roster + Assemble | 员工不继承精灵全家桶 | ✅ Assemble 写 `specialist_tool_faces` 并在 team build 消费；effective tools **硬拒** spirit 保留集（Allow 不能放行） |
| ORGFAST-52 | 领导工具白名单 | capability / tools | dept_lead / company_lead 仅治理工具 | ✅ `read_only` + Assemble `specialist_tool_faces` + effective tools 拒绝 spirit 保留集 |
| ORGFAST-53 | 剧本阶段 `collection_ids` | playbook + knowledge | Brief 无知识正文；检索引用 | ✅ 热路径 Assemble + team run；Subscribe 从 `sub_tasks_json` 回灌（不改 plan_steps_v2 列） |
| ORGFAST-54 | 记忆隔离单测 | memory 边界 | 兄弟不可读对方 L3 | ✅ 专项成员注入时 `ClampSpecialistL3Scopes`（Spirit 除外） |
| ORGFAST-55 | 确认五档（无默认开工卡） | gate / HITL | 仅造人/新剧本/高风险/危险工具/`confirm_before` | ✅ `confirm_before` 卡：`pbconfirm:` 禁止回落工具 await；无 waiter 记决定 + `stepWriter` 关卡；取消清 waiter；启动扫 `planning`/`executing` 再 Subscribe；`tool_blocked` 状态机守卫，落库失败不 Accepted |
| ORGFAST-56 | 主对话不订阅成员 token | progress / WS | 用户侧只有芯片/心跳/例外 | ✅ `noticeFilter.ts` 将 `token_usage` 标为系统内部，不渲染 NoticeBlock |
| ORGFAST-57 | 汇总输入收紧 | spirit_synthesis | prompt 仅 Brief+例外+清单 | ✅ |
| ORGFAST-58 | `deptmail`/`inject` 不得当横向/上行 | 测试锁 | 混用路径编译期或单测拒绝 | ✅ |
| ORGFAST-59 | 阶段可选 `graph_template_id` | playbook + M53 | 未填走 Team Turn；不新引擎 | ✅ Assemble 写入 definition；builtin 改 mode；资产 id 编译期 LoadGraphBuildConfig |

---

## 4. 改动文件清单（按阶段）

### Phase 0

| 文件 | 改动 |
|------|------|
| `internal/biz/agent_capability.go` | 组织字段 |
| `internal/agent/agent_capability_builder.go` | 批量填部门、排除主管出 Assignable 池（或匹配期排除） |
| `internal/agent/agent_allocator_impl.go` | 排除主管、补员策略、透传 DepartmentID |
| `internal/biz/task_plan.go` 或 PlanStep 结构 | DepartmentID 透传（若字段尚无） |
| `internal/service/team_orchestrator_real.go` | 填 DepartmentID / CrossDept |
| `internal/agent/agent_allocator_impl_test.go` 等 | 主管排除、补员、透传 |
| `internal/service/*_test.go` | 建团部门 |

### Phase 1

| 文件 | 改动 |
|------|------|
| `internal/agent/org_pruner.go`（新） | 剪枝 |
| `internal/agent/org_domain_map.go`（新） | 域-部门 alias |
| `internal/agent/org_pruner_test.go`（新） | 单测 |
| `internal/agent/org_pruner.go` / `org_domain_map.go` | OrgPruner |
| `internal/agent/agent_factory.go` | 占岗 |
| `cmd/admin/wire.go` / `wire_gen.go` | CapabilityBuilder / Factory / Orchestrator 注入 OrganizationReader |
| `internal/agent/org_pruner_eval_test.go` | 20 条中文任务评测夹具 |

### Phase 1b

| 文件 | 改动 |
|------|------|
| `internal/biz/team_types.go` | Artifact 指针字段 |
| `internal/biz/spirit_delivery.go` | 写清单、禁止大文件进 JSON 正文 |
| `internal/service/team_orchestrator_real.go` 或 assembly | inbox 物化 |
| `internal/biz/spirit_assembly.go` | 协议后缀 Brief/Bulk |
| `internal/biz/spirit_team_deliverable_test.go` / `spirit_phase1b_test.go` | 清单/物化/兼容 |
| `internal/biz/spirit_inbox.go` | RelPath 消毒 + inbox 物化 |
| `internal/service/member_fs.go` | `TeamInboxFS` |
| `internal/agent/task_planner_impl.go` | MemoryHit 合成 `st_recipe_replay` |
| `internal/tools/spirit_tools.go` | MemoryHit.AgentKeys → AllocateExplicit |

### Phase 2

| 文件 | 改动 |
|------|------|
| `internal/metrics/vars.go` | `OrgFastDeptLeadTotal` |
| `internal/biz/agent_allocator.go` | `StaffingAdvisor` 端口 |
| `internal/agent/staffing.go` | 一次咨询、超时、解析 |
| `internal/agent/agent_allocator_impl.go` | Phase B 先 staffing 再 Factory |
| `internal/agent/staffing_test.go` | 采纳 / 超时 / 不改写 DAG |

### Phase 4（核心已落地，发射/补写/仲裁仍开）

| 文件 | 改动 |
|------|------|
| `internal/agent` 分档器（新，如 `task_gear.go`） | light/medium/heavy 早判 + Plan 后晚升；单测锁死不额外 LLM |
| `internal/biz/company_lead.go`（新，对称 `dept_lead.go`） | `CompanyLeadAgentKeyPrefix`、幂等挂接公司节点 |
| `internal/data/ent/schema/organization.go` | 公司节点 `company_lead_agent_id`（Evolving） |
| `internal/agent/agent_capability_builder.go` | `company_lead` 与 `dept_lead` 一并排除出 Assignable |
| 公司 `metadata_json.playbooks` + 展开器 | 已授权剧本 → 部门槽 SubTasks；无剧本走 `playbook_fill` |
| `internal/scenario/system/prompts` / planner | 重型粗路由只出 `playbook_id` |
| `internal/biz/spirit_orchestration_cache.go` | `ConstraintFingerprint`；不合不复用 keys |
| checkpoint（对齐 M70，omitempty） | `gear` / `playbook_id` / 已授权阶段 / 已发 Brief |
| `orchestration_progress` | `upward` / `heartbeat`；载荷 ≤2KB |
| 门禁/事件 | 部门争议→总经理；公司争议→精灵呈用户 |

### Phase 4b（预算/汇总/锁/装配允许集/HITL/知识引用已落地）

| 文件 | 改动 |
|------|------|
| 成员 Turn 前缀装配 | R14 预算裁剪（知识/记忆） |
| `specialty_roster` / Assemble | toolset + MCP 允许集 |
| playbook stage | `collection_ids`、`confirm_before`、可选 `graph_template_id` |
| `PlanExecutor.RecoverUnfinishedBoards` | 启动扫 `planning`/`executing`，`ListPlanStepsByPlan` 后 `startBoardDAG`；`run()` 派发依赖已完成的 pending |
| `chat_confirm.go` | 剧本确认：`tool_blocked` 合法转换才落库；同决定幂等；落库失败不 Note/Resolve |
| `spirit_synthesis.go` | 合成输入去掉会话原文 |
| 单测 | L3 隔离、deptmail≠Brief、无默认开工确认；重启恢复 executing 看板；确认卡不可翻转 / 落库失败不成功 |

**禁止顺带**：不改 Graph 运行时内核、不改组织 CRUD UI、不改 DECISION.md 的三种 mode（除非发现与 R1 冲突）；不把 memberfs 开放给员工；Phase 4 不实现跨公司发送（ORGFAST-31）；不复活 ExecutionReportCard。

---

## 5. 验收标准（汇总）

- [x] 非显式指定时 dept_lead 不得为 AssignedKey / 团队 Lead
- [x] 成员有岗位时自动 Team.DepartmentID 非空，主管可 auto_join
- [x] 配方命中路径 Allocate 0 LLM
- [x] L3 只看见剪枝集
- [x] Factory 不创建公司/部门
- [x] domain_path 空 / 无组织树：组队不失败
- [x] B.10.21 与 spirit 相关既有测试绿
- [x] 文档：本三件套状态与代码一致（DOC-SYNC-5）
- [x] 下游前缀不含大文件正文；inbox 仅含信封声明条目
- [x] 旧 `DeliverableRef` 无 Kind 时仍能注入 Brief
- [x] 配方回放：MemoryHit 可按 Specialties 多槽回放 + AllocateExplicit；仍新建 Team
- [x] 高置信路径主管 LLM 计数 `skipped_high_confidence`
- [x] staffing 超时 fail-closed，不热路径 Factory；不二次分解
- [x] Allocate 不改写 Plan DAG，跨部只标 CrossDept
- [x] 分档规则可测（ORGFAST-40）；点名已授权剧本走热路径展开（未点名不套唯一剧本）
- [x] `company_lead` 不得为 AssignedKey / 启发式匹配；挂「总经理」岗；Tree/List 回填
- [x] 指纹不合不复用 keys；Allocate 滤掉治理领导
- [x] 成员首轮 Brief+协议受 6KB 硬顶；知识/记忆按需
- [x] 合成 prompt 无成员会话原文
- [x] 默认阶段无开工确认卡（`NeedsUserConfirm` 空输入 = 无卡）
- [x] DAG 发射 upward/heartbeat（非调度栅栏）；组织链无名剧本 fail-closed + 管理面授权沉淀
- [x] 专项成员 L3 去掉 team 总线（注入期 clamp）
- [x] 确认五档 HITL；花名册 tool/MCP 允许集在 Assemble 收口；PlanExecutor 按 `graph_template_id` 改道 M53
- [x] `confirm_before` 发出 ConfirmBlock；checkpoint 写入 playbook/阶段；剧本 `collection_ids` 落到知识检索
- [x] 进程重启扫未完成 PlanBoard 再 Subscribe；剧本确认 `tool_blocked` 守卫，落库失败不报成功

---

## 6. 风险与回滚

| 风险 | 缓解 |
|------|------|
| 剪枝过窄漏掉正确 Agent | FallbackAll + Warn；阈值可配；空映射不剪枝 |
| 多数票部门与 domain 映射不一致 | 以 OrgPrune 主部门为准，成员跨部走借调 |
| PlanStep 加字段破坏续跑 | 空值兼容；旧 draft 无部门则运行时多数票回填 |
| 评测集过拟合 | 夹具公开规则（领域词），不写死 AgentKey |
| inbox 拷贝过大拖慢调度 | 默认同声明文件硬链/拷贝；超阈只指针 + 按需拉；目录型协作改借调同团 |
| 分档误判把轻活做成重型 | 早判条件收窄；默认中档；升档规则单测锁死 |
| 无剧本时 TaskPlanner 偷偷拆岗 | 重型无剧本只允许 `playbook_fill` 或 fail-closed |
| 上行被当成调度栅栏 | 心跳不阻塞无依赖开工；单测锁死 |
| 旧 checkpoint 缺 Phase 4 字段 | omitempty；缺省仍 Recover |

回滚：Phase 0 的主管排除与 DepartmentID 回填可独立保留（即使剪枝回滚也更准）。OrgPruner 用 `FallbackAll` 开关即可关剪枝。
