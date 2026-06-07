# 公司架构重塑方案 — 行业分类→组织架构 + 跨部门协作

> **日期**：2026-06-07
> **类型**：proposal
> **状态**：评审通过，待实施

---

## 一、背景与问题

当前系统用"行业分类"(IndustryTaxonomy) 组织 Agent 和 Team，存在以下核心问题：

| # | 问题 | 影响 |
|---|------|------|
| 1 | "行业分类"语义与用户心智不匹配 | 用户需要的是"公司组织架构"，不是行业分类 |
| 2 | Team 挂在 industry 级别（`category_industry_id`） | Team ≈ 部门下的专项团队，应挂在 department 级别 |
| 3 | Taxonomy.department 与 Team 无关联 | 部门概念分裂：Taxonomy 里有部门，Team 也有部门，但互不关联 |
| 4 | 缺少跨部门协作机制 | 多部门协作只能靠 Spirit 手动编排，无约束、无审批 |
| 5 | 缺少部门主管角色 | 跨部门交付无守门人，质量无法把关 |
| 6 | Graph 归属模糊 | Graph 可独立存在也可被 Team 引用，缺乏明确语义 |

---

## 二、核心概念重新定义

### 2.1 四+二 概念模型

| 概念 | 本质 | 边界 | 负责 | 不负责 |
|------|------|------|------|--------|
| **公司架构** | 静态编制表 | 三级树：公司→部门→岗位 | 定义部门、岗位、职责描述 | 不管谁在岗、怎么组队 |
| **Agent** | 在编员工 | 挂在某个岗位上 | 填充岗位、执行任务 | 不管组队、不管流程 |
| **Team** | 专项团队 | 从某个部门中按需抽调人员组建 | 选人、配流程、定义目标 | 不定义岗位、不定义流程细节 |
| **Graph** | 团队流程 | 归属某个 Team（或作为模板） | 定义步骤、顺序、条件 | 不定义人员、不定义组织 |
| **精灵助手** | 用户入口 | 系统级，无部门归属 | 理解意图、委派编排管家 | 不直接执行业务任务 |
| **部门主管** | 部门守门人 | 挂在部门管理岗，system_builtin | 部门内资源协调、跨部门交付审批 | 不做具体业务执行 |

### 2.2 核心隐喻

```
公司架构 = 编制表（公司有哪些部门、部门有哪些岗位、岗位的职责）
Agent   = 在编员工（某个岗位上的具体干活的实体）
Team    = 专项团队（从部门中按需抽调人员，负责某个具体业务）
Graph   = 工作流程（这个团队怎么协作、怎么干活）
```

---

## 三、关系模型

### 3.1 依赖关系

```
公司架构 ◄──── Agent.position_id           (员工占岗)
公司架构 ◄──── Team.department_id          (团队归属部门)
公司架构 ◄──── 部门主管.dept_id             (主管守门)
Agent   ◄──── Team.members[]               (团队选人)
Team    ◄──── Graph.team_id                (流程归属团队)
Agent   ◄──── Graph.nodes[]                (节点执行者)
Team    ◄──── Team.depends_on[]            (DAG 依赖，跨部门协作)
部门主管 ◄──── Team (自动加入)               (审批门禁)
精灵助手 ────► 编排管家                      (委派编排)
编排管家 ────► Team DAG                     (组建+约束)
```

**依赖铁律**：

1. **公司架构是根基** — 所有概念都依赖它，它不依赖任何其他
2. **Agent 只依赖架构** — 不知道 Team/Graph/其他部门
3. **Team 依赖架构 + Agent** — 知道归属哪个部门，有哪些成员
4. **Graph 依赖 Team + Agent** — 知道服务哪个团队，节点谁执行
5. **Team 可依赖其他 Team** — 通过 DAG 实现跨部门协作
6. **部门主管依赖架构** — 随部门而生，审批跨部门交付
7. **精灵助手/编排管家不依赖任何部门** — 公司级基础设施

### 3.2 基数关系

| 关系 | 基数 | 说明 |
|------|------|------|
| Company → Department | 1:N | 一个公司有多个部门 |
| Department → Position | 1:N | 一个部门有多个岗位 |
| Position → Agent | 1:N | 一个岗位可有多个员工 |
| Department → Team | 1:N | 一个部门可组建多个专项团队 |
| Team → Agent (members) | M:N | 团队从部门各岗位选人，一个 Agent 可参与多个 Team |
| Team → Graph | 1:N | 一个团队可有多个 Graph（主流程 + 辅助流程），Graph 也可作为模板不属于任何 Team |
| Graph → Agent (nodes) | N:M | 流程节点引用 Agent 执行 |

### 3.3 关键设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| Team 是否可包含跨部门成员 | **允许，但需指定主归属部门** | Team 必须有主归属部门，默认从主部门选人；允许跨部门选人但需对方部门主管同意；跨部门协作有两种模式：轻量（Team内跨部门成员）和深度（Team DAG） |
| Graph 是否必须归属 Team | **否**，Graph 可作为模板独立存在 | 模板 Graph 不属于任何 Team，Team 通过 `graph_id` 引用 |
| 部门主管是否可被用户替换 | **是**，默认 system_builtin，用户可指定其他 Agent 为主管 | 灵活性，允许用户自定义主管行为 |
| 交付物契约是否结构化 | **初期为文本描述**，后续可扩展为结构化 schema | YAGNI，先跑通流程再优化 |

---

## 四、跨部门协作机制

### 4.1 三层约束模型

```
Layer 1: DAG 依赖约束 (结构层)
  ─ 编排管家组建 Team DAG
  ─ depends_on 定义执行顺序
  ─ 上游未完成 → 下游不启动

Layer 2: 部门主管审批门禁 (治理层)
  ─ 跨部门交付物需双方主管确认
  ─ 部门主管自动加入本部门的 Team
  ─ 主管有权驳回不合格交付物 → 上游返工

Layer 3: 交付物契约 (语义层)
  ─ 上游 Team 定义 deliverables（输出描述）
  ─ 下游 Team 定义 input_contract（输入期望）
  ─ 编排管家组建时验证契约匹配
```

### 4.2 跨部门协作完整流程

```
用户: "开发XX产品，需要设计和研发协作"

1. 精灵助手接收 → 识别为跨部门任务 → 委派编排管家

2. 编排管家:
   a. 分析任务 → 拆解为子任务
   b. 匹配部门: 设计部(设计) + 研发部(开发)
   c. 组建 Team DAG:
      [设计组(设计部)] ──depends_on──► [开发组(研发部)]
       deliverables: "UI设计稿"         input_contract: "UI设计稿"
   d. 自动将部门主管加入对应 Team
   e. 验证交付物契约匹配

3. 执行:
   a. 设计组执行 → 产出设计稿
   b. 输出方主管(设计部)质量把关 → 通过 → 接收方主管(研发部)验收确认 → 通过
   c. 交付物传递到开发组（DAG 调度激活下游）
   c'. 交付物传递机制：
       - 上游 Team 输出写入 Spirit Session 共享 Memory
       - DAG 调度激活下游 Team 时，将上游交付物注入下游 Team 上下文
       - 下游 Team 可通过 Graph state_fields 引用上游交付物
       - 注入格式：作为下游 Team 的 User Message 前缀（含来源团队、交付物名称、内容）
       - 后续迭代：支持注入到 Graph StateFields（结构化数据）
   d. 开发组执行 → 产出代码
   e. 部门主管-研发审批 → 通过
   f. 编排管家合成结果 → 精灵助手呈现给用户

4. 异常:
   b'. 输出方主管(设计部)驳回 或 接收方主管(研发部)驳回
       → 设计组返工（回到步骤 3a）
       → 超过最大重试次数 → 升级给精灵助手 → 通知用户
```

### 4.3 部门主管设计

| 属性 | 值 |
|------|-----|
| Agent Key | `__dept_lead_{dept_key}__` |
| Kind | `system_builtin` |
| Position | 部门管理岗（Department 的 position 中 level=management） |
| 自动创建 | 创建 Department 时自动生成 |
| 自动加入 | 本部门的 Team 被组建时自动成为成员 |
| 可替换 | 用户可在部门设置中指定其他 Agent 替代 |
| 不可删除 | 随部门删除而删除，不可单独删除 |

**借调审批规则**：

| 规则 | 说明 |
|------|------|
| 借调需审批 | 非 主归属部门成员加入 Team 需其来源部门主管同意 |
| 超时自动通过 | 审批超时（默认 5 分钟）自动通过，避免阻塞 |
| 借调比例上限 | 跨部门成员不超过 Team 总人数的 50%（可配置） |
| 双重归属 | 借调成员的工作产出归主归属部门主管审批，借调行为归来源部门主管审批 |

**部门主管的核心能力**：

1. **审批**：对跨部门交付物进行验收（通过/驳回+理由）
2. **协调**：当 Team 需要本部门资源时进行分配决策
3. **质量把关**：验证本部门 Team 的输出是否符合标准
4. **借调审批**：审批其他部门对本部门成员的借调请求（通过/拒绝/超时自动通过）
5. **借调监督**：查看被借调成员在目标 Team 的工作状态（只读）

---

## 五、数据模型变更

### 5.1 Organization（原 IndustryTaxonomy）

```
表名: organizations (原 industry_taxonomy)
字段变更:
  - level 值: "industry" → "company", "department" 不变, "position" 不变
  - 新增: dept_lead_agent_id (string) — 部门主管 Agent ID（仅 department 级节点）
  - 新增: dept_lead_config_json (text) — 部门主管配置（prompt 覆盖、工具等）
```

### 5.2 Agent

```
字段变更:
  - taxonomy_position_id → position_id
  - position_key 不变
  - agent_variant 不变
```

### 5.3 Team

```
字段变更:
  - category_industry_id → department_id (挂在 department 级)
  - 新增: deliverables (text) — 本 Team 的交付物描述 JSON
  - 新增: input_contract (text) — 期望从上游接收的内容描述 JSON
  - 新增: dept_lead_agent_id (string) — 本 Team 使用的部门主管（默认从部门继承）
  - 新增: cross_dept_member_ids (text, 默认 "[]") — 跨部门成员 Agent ID 列表 JSON
  - depends_on_json 不变（已有 DAG 依赖机制）
  - spirit_session_id 不变
```

### 5.4 Graph

```
字段变更:
  - 新增: team_id (string) — 归属 Team（可为空，表示模板）
  - 新增: is_template (bool, default false) — 是否为模板 Graph
  - 新增: verification_gates (text, default "[]") — 审批门禁定义 JSON
```

### 5.4b 部门删除级联规则

| 关联对象 | 处理策略 |
|----------|----------|
| 活跃 Team | 阻止删除（需先归档/取消） |
| 已完成 Team | 自动归档 |
| 部门主管 Agent | 级联删除 |
| 岗位下 Agent | 解除岗位关联（保留 Agent） |
| 岗位节点 | 级联删除 |
| 跨部门借调 | 自动取消 |

### 5.5 多公司支持（预留）

- 当前为单公司模式，Organization 树根节点唯一
- 预留扩展：通过 workspace_id 隔离不同公司的 Organization 树
- 多公司场景：用户可创建多个 workspace，每个 workspace 有独立的 Organization 树
- 暂不实现，schema 设计时确保 workspace_id 字段存在且可索引

### 5.6 交付物契约 Schema

```json
{
  "deliverables": [
    {
      "name": "ui_design_spec",
      "description": "UI设计规范文档，包含页面布局、交互说明、视觉规范",
      "format": "markdown",
      "required": true
    }
  ],
  "input_contract": [
    {
      "name": "ui_design_spec",
      "description": "UI设计规范文档，用于指导前端开发",
      "format": "markdown",
      "required": true
    }
  ]
}
```

### 5.7 审批门禁 Schema

```json
{
  "verification_gates": [
    {
      "node_id": "design_review",
      "gate_type": "dept_lead_approval",
      "department_id": "dept_design",
      "description": "设计部主管审批设计稿",
      "max_retries": 3,
      "escalation": "notify_user"
    }
  ]
}
```

---

## 六、迁移策略

### 6.1 阶段依赖说明

Phase 6 依赖 Phase 1 完成后启动（MG-01~05 全部依赖 Phase 1 的 Schema/Service 定义）

### 6.2 数据迁移

| 步骤 | 操作 | 风险 |
|------|------|------|
| 1 | `industry_taxonomy` 表重命名为 `organizations` | 低，Ent migration |
| 2 | `level="industry"` 的记录更新为 `level="company"` | 低，批量 UPDATE |
| 3 | `agents.taxonomy_position_id` 重命名为 `position_id` | 低，Ent migration |
| 4 | `teams.category_industry_id` 重命名为 `department_id`，值从 industry 级改为 department 级 | **中**，需要根据原 industry 查找其下的 department 节点 |
| 5 | 为每个 department 级节点自动创建部门主管 Agent | 低，新增数据 |
| 6 | 为每个 department 级节点设置 `dept_lead_agent_id` | 低，关联更新 |

### 6.3 API 迁移

| 旧 API | 新 API | 兼容策略 |
|--------|--------|----------|
| `/v1/industry-taxonomy/*` | `/v1/organization/*` | 旧 API 标记 deprecated，保留 2 个版本 |
| `/v1/taxonomy/*` | `/v1/organization/*` | 同上 |
| `IndustryTaxonomyService` | `OrganizationService` | 旧 Service 代理到新 Service |
| `TaxonomyNode` message | `OrgNode` message | 字段兼容，新增 dept_lead 相关字段 |
| Proto 字段编号 | 所有 rename 字段保持原编号不变 | Protobuf wire format 只认编号不认名称，rename 不改编号确保向后兼容 |

### 6.4 前端迁移

| 旧 | 新 | 说明 |
|----|-----|------|
| `features/industries/` | `features/organization/` | 目录重命名 |
| `Industry` 类型 | `Company` 类型 | 语义更新 |
| `TaxonomyPage` | `OrganizationPage` | 页面重命名 |
| `IndustryMarketPage` | `OrganizationMarketPage` | 页面重命名 |
| "行业分类" 文案 | "公司架构" 文案 | i18n 更新 |

---

## 七、影响域

| 层 | 变更 | 规模 |
|----|------|------|
| Ent Schema | 表重命名 + 字段变更 + 新增字段 | 中 |
| Proto | Service/Message 重命名 + 新增字段 | 中 |
| Biz | Usecase 重命名 + 新增部门主管逻辑 + 交付物契约 | 大 |
| Service | API 路由变更 + 部门主管自动加入 | 中 |
| Scenario | YAML 结构变更 + 部门主管 Agent 定义 | 中 |
| Spirit 编排 | 三阶段管线适配新概念 + 契约验证 | 大 |
| 前端 | 目录重命名 + 页面重做 + 部门主管 UI | 大 |
| 迁移 | 数据迁移 + API 兼容层 | 中 |

---

## 八、风险与缓解

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| Team.department_id 迁移时 industry→department 映射错误 | 中 | 高 | 迁移前备份，提供回滚脚本 |
| 部门主管审批阻塞业务流程 | 低 | 中 | 设置超时自动通过 + 最大重试升级 |
| 交付物契约匹配过于严格导致无法组建 Team | 中 | 中 | 初期契约仅做提示性验证，不阻断组建 |
| 前端重命名导致功能回归 | 中 | 高 | 逐页面迁移，保留旧路由重定向 |

---

## 九、Spirit 编排运行时融合评估

### 对现有编排规则的影响

| 现有机制 | 影响 | 说明 |
|----------|------|------|
| TaskPlanner 三阶段 | **无影响** | Plan/Allocate/Orchestrate 流程不变 |
| AgentAllocator 匹配 | **扩展** | 新增 department_id 过滤维度 |
| TaskOrchestrator 策略 | **扩展** | DAG 策略增加部门主管注入和交付物传递 |
| SpiritTeamAssembler | **扩展** | 新增部门主管注入 + 跨部门成员处理 |
| DAG 依赖调度 | **扩展** | 新增交付物传递 + 审批门禁 |
| Graph 编译执行 | **无影响** | verification_gates 是编译时注入 |
| 结果合成 | **无影响** | SynthesisEngine 不变 |

**结论**：新设计是**增量扩展**，不破坏现有编排规则。所有变更通过新增步骤和字段实现，不修改现有逻辑。

### 需要声明的模块协作

1. Team 必须声明 `department_id`（归属部门）
2. 跨部门成员必须声明 `cross_dept_member_ids` + `BorrowRequest`（借调关系）
3. DAG 依赖必须声明 `deliverables` + `input_contract`（交付物契约）
4. 部门主管通过 `VerificationGate` 在 Graph 中声明（审批门禁）
