# 03 · Agent Identity 清单（附录 A 模板）

> 赛道：新智基座丨Agent Infra —— **方向一：零人工运维**
> 平台 Agent 总数：**304**（证据：`api/ts1-agents-count.json`、`api/ts9-itops-agents.json`）。以下为核心职能 Agent 清单：TS-9 运维实跑团队（方向一·核心）+ 系统级职能 Agent + 组织层级角色示例。

---

## 一、TS-9 零人工运维实跑团队（方向一·核心，2026-07-29 实跑）

> 场景：电商平台订单服务 P2 生产事故（502 错误率 23% + 慢查询 47s + IO 98% + 连接池耗尽）。Spirit 规划 4 团队 DAG（告警分诊→根因定位→修复方案→恢复执行），复盘专家收尾。成员均来自 IT 运维岗位包（`internal/scenario/packs/it-ops-pack`，1 公司 / 5 部门 / 12 岗位）。

### 1. 精灵助手（Spirit）

| 字段 | 内容 |
|------|------|
| **Name** | `agent___spirit__`（精灵助手，系统内置总管家） |
| **Role** | Orchestrator —— 用户唯一对话入口，动态编排引擎 |
| **Capabilities** | 能做：事故理解、意图澄清、复杂度评估、DAG 拆解、团队组建、任务分派、结果合成。不能做：不替代岗位 Worker 做专业执行；不做未经审批的高风险动作 |
| **Inputs** | 事故描述（告警/指标/工单）；澄清问题答案；五层记忆召回 |
| **Outputs** | 澄清问题、任务计划（todo/DAG）、团队定义、分派指令、合成结论；plan draft 须经人工 confirm |
| **Dependencies** | Skill：planning-and-task-breakdown；工具：set/get_deliverable、memory_search；IT 运维岗位 Agent |
| **Decision Boundary** | 自主：澄清、拆解、组队、分派、合成。需人工确认：plan confirm 硬门控、高风险工具调用、Skill 进化注册 |
| **Trace** | 编排事件流（plan/team_stage/graph_stage）+ 检查点持久化 + PlanBoard-TaskPlan 状态传播（confirmed→executing→completed） |

### 2. 告警处理专家（alert_handler）

| 字段 | 内容 |
|------|------|
| **Name** | `alert_handler__general`（告警处理专家，运维部） |
| **Role** | Triage Worker —— 告警聚合降噪与定级 |
| **Capabilities** | 能做：告警聚合、降噪、告警风暴判断、P 级定级、根因方向初判、五级处置建议。不能做：操作生产系统、执行变更 |
| **Inputs** | 原始告警流（多源监控）+ 共享黑板任务上下文 |
| **Outputs** | alert-triage-report（定级 P2，含认知信封 assumptions/decisions/confidence=0.95/open_questions/rejected） |
| **Dependencies** | Skill：alert-diagnosis（skill_load 调用）；工具：knowledge_search、datetime、set_deliverable |
| **Decision Boundary** | 定级自主；P1 升级须通知事件响应指挥官；只读操作全自动 |
| **Trace** | 39 个活动（thinking/action/reply）+ 交付物落库（`ts9-team1-activities.json`） |

### 3. 故障诊断专家（fault_diagnostician）

| 字段 | 内容 |
|------|------|
| **Name** | `fault_diagnostician__general`（故障诊断专家，运维部） |
| **Role** | RCA Worker —— 根因链分析 |
| **Capabilities** | 能做：日志与指标关联分析、根因链推理、直接/间接/系统三层根因输出、放大因素识别。不能做：未经验证直接下结论、执行修复 |
| **Inputs** | 上游 alert-triage-report（get_deliverable 消费）+ 日志/指标证据 |
| **Outputs** | root-cause-analysis-report + rca-report：根因链（未评审变更→缺复合索引→全表扫描 47s→连接池耗尽→502），三层根因置信度 95/90/85% + 放大因素 + 修复建议 |
| **Dependencies** | 上游交付物；knowledge_search；skill_run |
| **Decision Boundary** | 分析自主；结论必须附置信度与证据链；工具失败带 reflection_hint 重试自纠正 |
| **Trace** | 55 个活动 + 双交付物落库（`ts9-team2-activities.json`） |

### 4. 变更执行专家（change_executor）

| 字段 | 内容 |
|------|------|
| **Name** | `change_executor__general`（变更执行专家，运维部） |
| **Role** | Executor Worker —— 恢复执行与验证 |
| **Capabilities** | 能做：应用回滚、在线变更、受控命令执行（tools_allow=shell_exec）、执行产物落盘、恢复指标验证。不能做：未经审批的高危操作、不可逆变更 |
| **Inputs** | 上游 recovery-plan-doc（首要方案=回滚 17:55 变更；后备=pt-osc 在线加索引） |
| **Outputs** | recovery-execution-report：回滚 v1.2.9→v1.2.8、pt-osc 128s 加索引（874 万行无锁表）；验证 502 23%→0.03%、慢查询 187→2/min、HikariCP 198→85、QPS 45→320 |
| **Dependencies** | shell_exec（白名单授权）、file_save_file、set_deliverable；上游方案交付物 |
| **Decision Boundary** | 高危操作（加索引）执行前**强制输出人工审批请求**；可回滚方案优先；exec_command 未注册时自适应降级为落盘执行产物（实跑验证，赛后已修复为受控命令执行） |
| **Trace** | 91 个活动 + 5 份执行产物 + 交付物落库（`ts9-team4-activities.json`） |

### 5. 复盘文档专家（postmortem_writer）

| 字段 | 内容 |
|------|------|
| **Name** | `postmortem_writer__general`（复盘文档专家，运维部） |
| **Role** | Postmortem —— 事故复盘与知识沉淀 |
| **Capabilities** | 能做：时间线还原、根因复核、处置评估、改进项制定、标准化复盘报告。不能做：评判个人责任、修改历史证据 |
| **Inputs** | 全链路交付物证据链（12 份 set_deliverable 记录） |
| **Outputs** | 12K 字标准《事故复盘报告》（时间线/根因/处置评估/改进项），入知识库供 RAG 召回 |
| **Dependencies** | get_deliverable；知识库写入 |
| **Decision Boundary** | 只读访问证据；对事不对人原则 |
| **Trace** | `ts9-postmortem-session.json`、`ts9-postmortem-reply.json` |

### IT 运维岗位包 12 岗位全清单

| 岗位 | 职能 | 岗位 | 职能 |
|------|------|------|------|
| 事件响应指挥官 | 事故统筹与升级决策 | 数据库运维专家 | 慢查询/死锁/索引/SQL 审核 |
| 告警处理专家 | 聚合降噪与定级 | 日志分析专家 | 日志模式挖掘与关联 |
| 故障诊断专家 | 根因链分析 | 指标分析专家 | 异常检测/容量预测/基线偏离 |
| 变更执行专家 | 回滚/在线变更/受控执行 | 网络巡检专家 | 网络链路排查 |
| 修复方案工程师 | 方案制定与风险评估 | 系统巡检专家 | 主机/系统健康检查 |
| 合规检查专家 | 变更合规审计 | 复盘文档专家 | 事故复盘与知识沉淀 |

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
| 部门主管-后端开发部 `__dept_lead_backend_dev__` | 部门协调者 | 后端开发资源协调与交付审批 | 软件开发行业岗位 Agent |

> 组织体系：行业（金融/自媒体/软件开发 3 内置行业）→ 部门（16+）→ 岗位（90+），每个岗位有预置 prompt 模板（general/technical/management 变体），开箱即用并可按行业复制。
