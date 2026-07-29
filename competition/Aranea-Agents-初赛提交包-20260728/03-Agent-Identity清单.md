# 03 · Agent Identity 清单（附录 A 模板）

> 赛道：新智基座丨Agent Infra —— 复杂任务多 Agent 自主协同
> 平台 Agent 总数：**274**（证据：`api/ts1-agents-count.json`）。以下为核心职能 Agent 清单：实跑团队 4 成员 + 系统级职能 Agent + 组织层级角色示例。

---

## 一、实跑团队核心成员（TS-2 医疗云市场调研，2026-07-25 实跑）

### 1. 精灵助手（Spirit）

| 字段 | 内容 |
|------|------|
| **Name** | `agent___spirit__`（精灵助手，系统内置总管家） |
| **Role** | Orchestrator —— 用户唯一对话入口，动态编排引擎 |
| **Capabilities** | 能做：任务理解、复杂度评估、DAG 拆解、临时团队组建、任务分派、结果合成、记忆召回。不能做：不替代专业 Worker 做领域执行；不做未经审批的高风险动作 |
| **Inputs** | 用户自然语言任务；五层记忆召回结果；编排缓存（历史同类任务 DQ 评分） |
| **Outputs** | 任务计划（todo/DAG）、团队定义、分派指令、合成后的最终交付物；质量要求：计划可校验（PlanStep 契约校验）、交付物带证据引用 |
| **Dependencies** | Skill：planning-and-task-breakdown；工具：set/get_deliverable、memory_search；Worker Agent（按任务动态分配） |
| **Decision Boundary** | 自主：拆解、组队、分派、合成。需人工确认：成本/合规等关键维度（实跑中就「成本估算维度」主动发起确认）、高风险工具调用、Skill 进化注册 |
| **Trace** | 编排事件流（plan/team_stage/graph_stage）+ 检查点持久化 + 会话参与者记录（`api/ts2-session-participants.json`）+ TeamRun 状态机 |

### 2. claudecode（WebSearch Worker）

| 字段 | 内容 |
|------|------|
| **Name** | `claudecode`（实跑中承担 Web 检索职能） |
| **Role** | Worker —— 外部信息检索与证据收集 |
| **Capabilities** | 能做：Web 搜索、页面抓取、原始证据整理。不能做：写外部系统、发布内容、做出业务结论 |
| **Inputs** | 上级分派的检索子任务 + 共享黑板中的任务上下文 |
| **Outputs** | 结构化检索结果（来源、摘要、URL）；证据等级标注 |
| **Dependencies** | web_fetch / web_search 工具；MCP Playwright（浏览器自动化） |
| **Decision Boundary** | 只读操作全自动；网络不可达时自动降级为内部知识路径并上报降级标记 |
| **Trace** | 工具调用审计（ts6-03）+ 活动树 action 事件 + runs 记录 |

### 3. deepresearch（MarketResearch Worker）

| 字段 | 内容 |
|------|------|
| **Name** | `deepresearch`（实跑中承担市场研究职能） |
| **Role** | Worker —— 竞品调研与行业分析 |
| **Capabilities** | 能做：竞品方案对比、行业趋势分析、结构化研究报告。不能做：财务承诺、法律意见 |
| **Inputs** | 检索证据 + 上游交付物（deliverable 引用） |
| **Outputs** | 竞品分析表、趋势判断、合规评分（如信创合规评分表）；结论附证据等级（强/弱/缺失） |
| **Dependencies** | 上游 WebSearch 产物；知识库 RAG；memory_search |
| **Decision Boundary** | 分析自主；证据不足时必须输出「证据缺口」而非编造结论 |
| **Trace** | 活动树 thinking/action/reply 全程流式记录（ts2-04） |

### 4. finance（CostEstimation Worker）

| 字段 | 内容 |
|------|------|
| **Name** | `finance`（实跑中承担成本估算职能） |
| **Role** | Worker —— 成本建模与估算 |
| **Capabilities** | 能做：TCO/CAPEX/OPEX 建模、敏感性分析、分阶段成本建议。不能做：真实采购、对外报价承诺 |
| **Inputs** | 市场研究结论 + 成本估算维度（经人工确认） |
| **Outputs** | 成本估算表（实跑产出：三甲医院上云 60-90 万/年 vs 传统机房 300-500 万 CAPEX 等） |
| **Dependencies** | 上游研究产物；成本模型知识 |
| **Decision Boundary** | 估算自主；对外可用数字须经 Orchestrator 合成时复核 |
| **Trace** | Token 用量与成本事件（六维定价 × 微美元精度）+ 活动树记录 |

---

## 二、系统级职能 Agent

| Name | Role | Capabilities | Decision Boundary | Trace |
|------|------|-------------|-------------------|-------|
| 记忆管家 `__memory__` | 记忆治理 | 选择性记忆、质量驱动遗忘、记忆蒸馏；不直接服务业务任务 | 自动归档/衰减；PII 内容须人工审查 | 记忆策略审计（MemoryPolicyEngine）+ 6 个 Cron Worker 状态面板 |
| 技能管家 `__skills__` | Skill 治理 | 技能健康度评估、进化/消亡建议、工具权重优化；不直接注册 Skill | 仅产出建议；注册必须人工审批 | Skill 提案状态机 + 调用统计 |
| 系统管家 `__system_admin__` | 系统运维 | 管理 Skill/Agent/Team 资源；不处理业务对话 | 管理操作全审计 | 管理操作审计日志（actor/IP/severity） |

## 三、组织层级角色（示例）

| Name | Role | Capabilities | Dependencies |
|------|------|-------------|--------------|
| 部门主管-财务部 `__dept_lead_finance_dept__` | 部门协调者 | 财务追踪/分析/FP&A 资源协调，跨部门交付审批 | 下属岗位 Agent + dept_lead 工具配置 |
| 部门主管-医疗创新部 `__dept_lead_medical_innovation__` | 部门协调者 | 医疗创新策略资源协调与交付审批 | 医疗行业岗位 Agent |

> 组织体系：行业（金融/自媒体/软件开发 3 内置行业）→ 部门（16+）→ 岗位（90+），每个岗位有预置 prompt 模板（general/technical/management 变体），开箱即用并可按行业复制。
