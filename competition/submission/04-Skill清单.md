# 04 · Skill 清单（附录 B 模板）

> 赛道：新智基座丨Agent Infra —— 复杂任务多 Agent 自主协同
> Skill 为本方案**必选项**：作为任务能力抽象层，与 Agent 解耦、可版本化、可审批发布、可跨 Agent/场景复用。
> 平台 Skill 全生命周期证据：`api/ts3-skills.json`、`api/skills-list.json`、TS-3/TS-4 截图。

---

## Skill 1：InformationResearch（自动进化生成 · 核心创新实证）

| 字段 | 内容 |
|------|------|
| **Skill 名称** | InformationResearch |
| **Skill 类型** | 自定义 Skill —— 由 **Skill 自动进化闭环**从运行时观测中自主生成（非人工编写） |
| **使用场景** | 需要结合内部记忆与外部 Web 信息的调研类任务：市场研究、竞品分析、资料核查 |
| **输入参数** | 研究主题/问题（自然语言）；可选上下文（工作记忆中已有材料） |
| **输出结果** | 交叉综合后的结构化研究结果（内部记忆证据 + 外部 Web 证据 + 综合结论） |
| **调用条件** | ① 用户发起研究/查询/检索请求；② Agent 出现 memory_search ↔ web_fetch 交替高频调用模式；③ 需要内外数据结合的任务 |
| **依赖工具/系统** | `memory_search`（长期记忆检索）、`web_fetch`（外部页面抓取）、`working_memory_list`（工作记忆盘点） |
| **失败处理** | 任一数据源失败时降级为其余来源继续，并在结果中标注证据缺口；LLM 综合失败回退为原始证据罗列 |
| **权限与安全** | 全部只读操作，无副作用；不写入外部系统；生成-注册全程经人工审批状态机（pending→approved→registered）；注册时拒绝路径穿越字符 |
| **复用价值** | 通用调研方法论，可被任何行业 Agent 复用；由模式检测自动生成，证明 Skill 生态可自我生长 |
| **进化证据** | 提案 ID `143f9062bff31134d461681d`，Pattern Hash `d85c036e4170cd90`（去重幂等），confidence=1.0，approved_by=dev，2026-07-24 完成 registered（`api/proposal-*.json`、`api/skills-list.json`；截图 ts4-01/02/03） |

## Skill 2：planning-and-task-breakdown

| 字段 | 内容 |
|------|------|
| **Skill 名称** | planning-and-task-breakdown |
| **Skill 类型** | 自定义 Skill（filesystem 同步，v1.0.0，validation=pass） |
| **使用场景** | 有规格/明确需求需拆解为可实现任务；任务过大难以启动；需估算范围或并行化时 |
| **输入参数** | 任务描述 / 规格文档 |
| **输出结果** | 有序任务列表（todo/DAG 步骤），含依赖关系 |
| **调用条件** | 复杂任务识别（多步骤、跨职能）；编排管线 TaskPlanner 阶段 |
| **依赖工具/系统** | 任务 DAG 持久化（环检测 + 拓扑排序 + 就绪节点计算） |
| **失败处理** | 拆解失败/契约校验不通过时回退单步执行并提示人工细化 |
| **权限与安全** | 只读推理 + 内部状态写入，无外部副作用 |
| **复用价值** | 所有复杂任务的通用前置能力；**实测调用 5100+ 次，成功率 100%**，平均耗时 <20ms（`api/ts3-skills.json`，计数随运行持续累计） |

## Skill 3：idea-refine

| 字段 | 内容 |
|------|------|
| **Skill 名称** | idea-refine |
| **Skill 类型** | 自定义 Skill（filesystem 同步，v1.0.0，validation=pass） |
| **使用场景** | 创意/方案的结构化打磨：发散生成 → 收敛评估迭代 |
| **输入参数** | 初始想法（自然语言），或 "idea-refine"/"ideate" 显式触发 |
| **输出结果** | 打磨后的方案（含多轮发散-收敛记录） |
| **调用条件** | 用户要求 brainstorm/打磨想法；或生成类任务质量不足时自我迭代 |
| **依赖工具/系统** | LLM 推理（Provider 可插拔） |
| **失败处理** | LLM 不可用时回退启发式发散模板 |
| **权限与安全** | 无副作用 |
| **复用价值** | 内容创作、产品策划、研发方案等场景通用；**实测调用 5100+ 次，成功率 100%**（`api/ts3-skills.json`，计数随运行持续累计） |

---

## Skill 工程机制（与多 Agent 协同流程的关系）

| 机制 | 说明 |
|------|------|
| 渐进式加载 | L0 每轮仅注入 Skill 清单（name+description）→ LLM 按需调用 `skill_load` 加载 Body → `skill_select_docs` 加载 Refs，大幅节省 Token |
| 意图路由 | 嵌入向量评分（权重 0.3）自动匹配最相关 Skill，多 Agent 共享同一路由策略 |
| 版本管理 | major.minor.patch 语义化版本；回滚 = 基于历史版本创建新 patch 版本 |
| 去重与融合 | Pattern Hash 确定性去重（同 Agent 同模式只提案一次）；六维相似度 + LLM 炼化合并相似 Skill，冲突合并失败自动回滚 |
| 审批状态机 | detected → pending → approved → registered / rejected / expired，全程留痕可审计 |
| 健康治理 | 周均调用、成功率、耗时三维健康度评估（healthy/warning/critical），驱动消亡建议 |
| 协同关系 | Skill 是 Agent 间复用能力的载体：Orchestrator 拆解用的 planning Skill 与 Worker 调研用的 InformationResearch Skill 在同一注册表治理，跨团队/行业共享 |
