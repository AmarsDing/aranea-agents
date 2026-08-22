# 组织与专项不变量（Agent 必读）

> **地位**：编制表 + 专项 Agent + 组织感知编排的架构锁。改 `internal/biz/organization.go`、`dept_lead.go`、`company_lead.go`、Allocator、花名册、工具装配、Spirit 热路径前必读。
> **产品入口**：[README §3.5](../../README.md) · **模块**：[M67](./67-organization-redesign.md) · [M78](./78-org-aware-orchestration.md) · [65 §1.42](./65-module-cross-reference-full.md)
> **ADR**：[ORG-FAST](../reports/2026-08-22-review-adr-org-aware-orchestration.md) · [重型链](../reports/2026-08-22-review-adr-org-heavy-chain.md) · [横切](../reports/2026-08-22-review-org-heavy-chain-crosscut.md)

**禁止**：把本文「简化」成全库海选、通用工人池、或全员挂精灵工具箱。需求变了先改 M67/M78 三件套，再改代码。

---

## 1. 公司框架

- 一棵树：`company` → `department` → `position`（`organizations.level`）。不是「行业分类」。
- 当前 workspace **默认一棵公司树**。换业务主体 = 换 workspace，不是任务内检索/创建公司。
- 创建部门时自动挂 `dept_lead`；创建公司时自动挂 `company_lead`（幂等，`__company_lead_{key}__`）。
- `company_lead` 挂在真实岗位上：公司 → 系统部门「总经理办公室」（`{companyKey}_office`，不另生 `dept_lead`）→ 岗位「总经理」（`{companyKey}_gm`）。打开组织树 / Agent 列表会回填已有公司。编制/预设区可见，不进精灵管家区。
- 任务路径**禁止** `OrganizationWriter` 创建 company/department。

## 2. 部门架构

- Team 必须能落到主归属 `department_id`（成员岗位可推导时）。
- 跨部门：轻耦合 → 借调（≤50%、超时自动过）；有交付依赖 → 每部门一 Team + DAG。
- 文件耦合重（共改同一工作树）→ 优先同团借调，不要为抄文件拆队。

## 3. 每个 Agent 是专项，不是通用劳动力

挂在**岗位**上的业务 Agent 出生即带齐：

| 配置 | 字段/来源 | 含义 |
|------|-----------|------|
| 身份 | `mission_statement`、`domain_path`、`agent_variant`、岗位 Prompt | 这个人是谁、干什么专题 |
| 能力 | `AgentCapability`（Roles/Skills/Tools 从 config 抽出） | 匹配与花名册绑定 |
| 工具 | `agent_runtime_settings`：`ToolsProfile` + Allow/Deny | **自己的工具面**，不继承精灵 `spirit` profile |
| MCP | 有效工具门禁 + `mcp:` allow/deny | 只连自己声明的服务器；broker 按需拉 schema |
| 技能 | Skill 绑定 + 渐进加载 | 按专项加载，不灌全库 Skill 正文 |

花名册：`domain_path` → 该岗 primary（+ backup）。有专题时不 L3 海选、不低分交差、热路径不 Factory。缺编 fail-closed：指定已有专项或去编制表补人。

`dept_lead` / `company_lead`：**治理身份**，默认 `ToolsProfile=read_only`（监管工具由身份注入，如 memberfs/deptmail）。`IsHeuristicAssignable` / `IsCatalogAgentAssignable` 必须为 false。

## 4. 编排怎么用这张编制表

- 轻：不组队。中：花名册 + 部门门禁。重：已授权剧本展开部门槽 + 三管道。用户点名组织链但公司无剧本 → fail-closed（`playbook_fill_required`），禁止 TaskPlanner 按行业常识拆岗；授权走管理面 `AuthorizeCompanyPlaybook`。
- 生产建团唯一路径：`PlanExecutor` + `RealTeamOrchestrator`。
- 同类班底复用 = 配方槽位 + 新 `AssembleTeam`，禁止复活 completed Team。
- 交接：Brief/Bulk；知识库引用不复制；记忆 L3 不横向倒给兄弟。
- 成员首轮：Brief + 协议受 6KB 硬顶；知识/记忆默认按需工具取，不预灌正文。主对话不刷成员 token。

## 5. 改架构的合法入口

只有修订 M67/M78 需求+设计+ADR 之后，才允许改上述不变量。禁止在「顺手重构」里把专项配平、把领导选成 Lead、或给员工挂上 `plan_and_execute`。
