# 31 记忆管理界面（Memory Management UX）

本文档从**用户角度**设计 aranea 的统一记忆管理界面，把 `12 memory-L0-sensory.md`、`13 memory-L1-working.md`、`14 memory-L2-episodic.md`、`15 memory-L3-semantic.md`、`16 memory-L4-persistent.md` 中的技术能力，整理成可理解、可审计、可控制的产品体验。

目标不是让用户理解 L0/L1/L2/L3/L4 的内部实现，而是让用户回答四个问题：

1. Agent 现在「看见了什么」？
2. Agent 正在「记住什么任务状态」？
3. 这次会话「发生过什么」？
4. Agent 长期「知道什么、相信什么、如何进化」？

---

## 1. 产品定位

### 1.1 页面名称

建议主入口命名为 **记忆中心**，英文为 **Memory Center**。

导航位置：

- 一级入口：左侧导航 `Memory`
- Agent 设置内入口：`Agent 设置 -> 记忆`
- Session 详情内入口：`Session 详情 -> 记忆`
- 管理员入口：`Admin -> Memory Ops`

### 1.2 用户价值

| 用户问题 | 页面回答 |
|----------|----------|
| 为什么 Agent 这次这么回答？ | Prompt 组成、上下文比例、L1/L3/L4 注入来源 |
| Agent 是否记错了我？ | L3 facts、冲突、反馈、版本历史 |
| 我能不能删除或纠正记忆？ | 编辑、反驳、归档、遗忘、回滚 |
| 某个任务现在推进到哪了？ | L1 工作记忆 task / field / subtask |
| 这次会话有哪些关键事件？ | L2 时间线、episode、标记、巩固状态 |
| Agent 是否在自我改变？ | L4 identity、strategy、proposal、evolution event |
| 记忆是否会泄露隐私？ | scope、PII 标记、访问权限、审计记录 |

### 1.3 设计原则

1. **先说人话，再暴露技术细节**：默认展示「上下文、任务、事件、知识、进化」，高级模式再展示 L0-L4。
2. **用户拥有最终控制权**：任何长期记忆、冲突仲裁、Agent 进化都必须可编辑、可拒绝、可回滚。
3. **可解释优先**：每条被注入 prompt 的记忆都能追溯来源、分数、作用域和最近使用时间。
4. **隐私默认收紧**：用户私有记忆默认不进入 workspace；PII 默认脱敏并提示。
5. **数据密集但不压迫**：列表、图谱、时间线用渐进披露，详情放抽屉，不把所有字段摊在首屏。
6. **深浅色同等设计**：所有 surface、chip、progress、diff、graph 边线均要有 light/dark token。

---

## 2. 用户角色与权限

| 角色 | 可做什么 |
|------|----------|
| 普通用户 | 查看自己的 user / agent 作用域记忆；确认、反驳、编辑、删除自己的 facts；查看自己 session 的 L0/L1/L2 |
| Agent Owner | 管理 Agent 记忆设置；查看该 Agent 的 facts、episodes、identity、strategy；审核该 Agent 的进化提议 |
| Workspace Admin | 管理 workspace/team/global 范围内的记忆；处理冲突；运行巩固、索引、衰减任务 |
| Developer / Debugger | 使用 Prompt 调试器、Recall 调试器、Trace 详情；查看 token 分段和检索分数 |
| Auditor | 只读查看审计、版本、删除、回滚、PII 处理记录 |

权限基线：

- `scope=user`：仅本人和授权 admin 可读写。
- `scope=agent`：Agent Owner 和 workspace admin 可读写。
- `scope=team`：team 成员可读，team owner/admin 可写。
- `scope=workspace`：workspace 成员可读，admin 可写。
- `scope=global`：仅平台 admin 可写，普通用户只读。

---

## 3. 五层记忆的用户化表达

| 技术层 | 用户名称 | 用户理解 | 主要入口 |
|--------|----------|----------|----------|
| L0 Sensory | 上下文窗口 | 下一次发给模型的材料 | Session 详情 -> 上下文 |
| L1 Working | 工作记忆 | 当前任务的状态板 | Session 详情 -> 工作记忆 |
| L2 Episodic | 会话事件 | 本次会话发生了什么 | Session 详情 -> 事件 / Episodes |
| L3 Semantic | 知识记忆 | Agent 长期记住的事实、偏好、规则 | 记忆中心 -> 知识库 |
| L4 Persistent/Evolution | 图谱与进化 | Agent 认识的人/项目/关系，以及自我变化 | 记忆中心 -> 图谱 / 进化 |

界面中可以用 L0-L4 作为高级标签，但主文案应优先使用「上下文、工作记忆、事件、知识、进化」。

---

## 4. 信息架构

```text
Memory Center
├── Overview                    记忆总览
├── Knowledge                   知识库（L3 facts）
│   ├── Facts
│   ├── Conflicts
│   └── Feedback
├── Sessions                    会话记忆（L0/L1/L2）
│   ├── Context                 上下文窗口（L0）
│   ├── Working Memory          工作记忆（L1）
│   ├── Timeline                事件时间线（L2）
│   ├── Episodes                任务片段（L2）
│   └── Marks                   用户标记（L2）
├── Graph                       知识图谱（L4 graph）
├── Evolution                   Agent 进化（L4 evolution）
│   ├── Identity
│   ├── Strategy
│   ├── Proposals
│   └── Evolution Log
├── Debug                       调试工具
│   ├── Prompt Preview
│   ├── L2 Recall Tester
│   ├── L3 Recall Tester
│   └── Graph Recall Tester
└── Settings                    Agent 记忆设置
    ├── L0 上下文策略
    ├── L1 工作记忆
    ├── L2 事件与巩固
    ├── L3 语义检索
    └── L4 图谱与进化
```

移动端或窄屏下：

- `Overview / Knowledge / Sessions / Graph / Evolution` 用顶部 `QTabs`。
- 列表详情从右侧抽屉改为底部 `QDialog`。
- 图谱默认显示「列表 + 邻居卡片」，避免小屏强行展示复杂力导图。

---

## 5. 记忆总览页 `/memory`

### 5.1 页面目标

让用户用 30 秒判断：

- 当前 Agent 记忆是否健康。
- 哪些记忆正在影响回答。
- 是否有需要处理的冲突、提议、隐私风险。

### 5.2 首屏布局

| 区域 | 内容 |
|------|------|
| 顶部选择器 | Workspace / User / Team / Agent scope 切换；Agent 选择；时间范围 |
| 健康卡片 | 上下文风险、活跃任务数、待巩固 episodes、facts 总数、冲突数、待审核进化提议 |
| 记忆流向图 | L0 <- L1/L2/L3/L4 的简化流程；显示各层开关状态 |
| 最近影响回答 | 最近 10 条被注入 prompt 的 L1/L3/L4 片段 |
| 待处理事项 | 冲突、PII、巩固失败、proposal 审核、索引失败 |

### 5.3 KPI

| 指标 | 说明 |
|------|------|
| Context Used | 最近一次 L0 prompt token 占比 |
| Active Working Tasks | 当前 session / agent 的活跃 L1 task 数 |
| Episodes Pending | 待巩固 L2 episode 数 |
| Facts Active | 活跃 L3 fact 数 |
| Conflict Open | 未处理 fact 冲突数 |
| Evolution Pending | 待审核进化提议数 |
| Recall Hit Rate | 最近 7 天被召回后实际进入 prompt 的比例 |

### 5.4 空状态

新 Agent 没有记忆时，不展示空表格。展示引导：

```text
这个 Agent 还没有长期记忆。
开始一次对话后，系统会自动记录会话事件；当你确认偏好或任务完成后，高价值内容会进入知识记忆。
```

CTA：

- `开始会话`
- `手动添加一条知识`
- `查看记忆设置`

---

## 6. Session 详情：上下文窗口（L0）

### 6.1 用户目标

用户想知道「下一次模型调用到底会看到什么」。

### 6.2 页面结构

入口：`/sessions/:id/memory/context`

| 区域 | 内容 |
|------|------|
| Token 仪表 | 当前 used ratio、最大 ratio、context window、reserved output |
| Prompt 分段瀑布 | system / skill / L1 / L2 / L3 / L4 / summary / history / current input |
| 装配快照列表 | 最近 20 次 `memory_l0_assembly_snapshots` |
| 详情抽屉 | 每段 preview、tokens、source、warning、是否被截断 |
| 操作 | 重新预览、复制 prompt preview、打开 Trace、调整设置 |

### 6.3 交互细节

- 点击分段瀑布中的 `memory.l3`，右侧抽屉列出注入的 facts、final_score、scope、confidence。
- 点击 `history`，展示被保留的最近 turn 和被摘要替代的 turn 范围。
- `warning_codes` 用 chip 展示：`near_limit`、`exceeded`、`summary_failed`、`no_summary_available`。
- 非管理员只能看到 preview；完整 content 按权限二次确认。

### 6.4 关键状态

| 状态 | UI |
|------|----|
| normal | 绿色 progress，说明「上下文健康」 |
| warning | 橙色 progress，提示「接近摘要阈值」 |
| critical | 红色 progress，建议压缩或新开会话 |
| exceeded | 红色 banner，提供「立即生成摘要」和「减少注入记忆」 |

---

## 7. Session 详情：工作记忆（L1）

### 7.1 用户目标

用户想知道「Agent 当前任务状态是否正确」。

### 7.2 页面结构

入口：`/sessions/:id/memory/working`

| 区域 | 内容 |
|------|------|
| 左侧 Task 列表 | task title、agent、status、token 预算、更新时间 |
| 中间字段树 | `task_goal`、`active_constraints`、`subtasks`、`key_decisions`、`open_questions` |
| 右侧字段详情 | 字段值、source、revision、pin_to_prompt、visibility、TTL |
| 底部操作栏 | 保存、回滚、删除字段、导出 JSON、归档任务 |

### 7.3 字段体验

字段按「人能理解」的分组显示：

- 目标：`task_goal`
- 约束：`active_constraints`
- 待办：`subtasks`
- 中间结果：`intermediate_results`
- 决策：`key_decisions`
- 问题：`open_questions`

每个字段显示：

- 是否进入 prompt：`Pinned` / `Internal`
- 来源：user / agent / tool / system
- 版本：`v7`
- token：`320 tokens`
- 最近修改人和时间

### 7.4 风险防护

- 写入超过字段上限：字段下方显示错误，不清空输入。
- 写入超过 task budget：弹出建议「转为 internal」「精简」「归档旧字段」。
- revision 冲突：展示「你的版本 / 最新版本」双栏 diff，用户选择覆盖或合并。
- 含 PII 字段：默认 `visibility=internal`，需要显式确认才能 pin。

---

## 8. Session 详情：事件与 Episodes（L2）

### 8.1 事件时间线

入口：`/sessions/:id/memory/events`

用户目标：复盘「这次会话发生了什么」。

| 区域 | 内容 |
|------|------|
| KPI | 消息数、模型调用、工具调用、失败数、总 tokens、总成本、平均延迟 |
| 筛选 | 类型、actor、状态、关键字、时间、仅失败 |
| 时间线 | turn 聚合卡片：用户输入 -> 模型 -> 工具/Skill/MCP -> 回复 |
| 详情抽屉 | 原始 message、tool args/result、span tree、token/cost |
| 标记菜单 | 标星、加入巩固队列、需要复盘、好范例、坏范例、遗忘 |

事件卡片推荐字段：

| 字段 | 说明 |
|------|------|
| kind chip | message / model_call / tool_call / skill_call / mcp_call / summary |
| actor | user / agent / tool / skill |
| status | success / failed / cancelled |
| preview | 160 字内摘要 |
| metrics | duration、tokens、cost |
| marks | star / consolidate / postmortem |

### 8.2 Episode 列表

入口：`/sessions/:id/memory/episodes`

用户目标：查看「哪些任务片段会被沉淀为长期知识」。

| 区域 | 内容 |
|------|------|
| Episode 表格 | title、kind、outcome、importance、confidence、duration、tokens、consolidation_status |
| 详情抽屉 | goal、outcome_summary、result_preview、key_decisions、key_artifacts、L1 snapshot |
| 巩固结果 | 已生成 L3 facts / L4 entities / L4 relations 数量 |
| 操作 | 修改 importance、立即巩固、重建索引、创建复盘、删除 |

### 8.3 Mark 中心

入口：`/sessions/:id/memory/marks`

按照用户行为组织：

- 标星
- 巩固候选
- 复盘
- 好范例
- 坏范例
- 已遗忘

用户可以批量把标星事件创建为 milestone episode。

---

## 9. 知识库（L3）

### 9.1 页面目标

用户管理 Agent 长期「相信」的事实、偏好、规则、经验。

入口：`/memory/knowledge`

### 9.2 列表页

| 区域 | 内容 |
|------|------|
| 顶栏 | scope chip：user / agent / team / workspace / global；新增 Fact |
| 统计 | 总数、活跃、归档、存疑、冲突、平均 confidence、近 7 日命中率 |
| 筛选 | kind、tags、status、scope、min confidence、来源、时间 |
| 表格 | statement、kind、scope、confidence、importance、hit_count、source、updated_at |
| 批量操作 | 归档、删除、重建 embedding、添加 tag、导出 |

### 9.3 Fact 详情抽屉

详情抽屉分 5 个 tab：

| Tab | 内容 |
|-----|------|
| 内容 | statement、details、tags、kind、scope |
| 证据 | source episode、source session、source message、外部来源 |
| 使用 | 最近召回记录、进入 prompt 次数、hit_count、use_count |
| 反馈 | confirm / reject / refine / used / not_used 时间线 |
| 版本 | fact_versions、diff、回滚 |

### 9.4 用户反馈动作

| 动作 | 文案 | 后端行为 |
|------|------|----------|
| 确认 | `这是正确的` | feedback confirm，confidence + |
| 反驳 | `这不对` | feedback reject，confidence -，可能触发 conflict |
| 修改 | `改成...` | update fact，新版本 |
| 忘记 | `不要再记住` | soft delete 或 archived |
| 限制作用域 | `只对我有效` | scope 改为 user |

### 9.5 冲突待办

入口：`/memory/knowledge/conflicts`

冲突卡片以「裁判视角」展示：

```text
冲突：React 项目默认使用 Tailwind
Fact A：本项目使用 Tailwind CSS
Fact B：本项目禁止 Tailwind，使用 SCSS Modules
来源：Episode #123 / 用户手动添加
建议：保留 B，因为来源更新时间更近且用户确认过
```

操作：

- 保留 A
- 保留 B
- 合并
- 标记存疑
- 拆分作用域
- 延后处理

---

## 10. 知识图谱（L4 Graph）

### 10.1 页面目标

用户查看 Agent 长期认识的实体、项目、技术栈、人员和它们之间的关系。

入口：`/memory/graph`

### 10.2 页面结构

| 区域 | 内容 |
|------|------|
| Sidebar | scope、entity_type、keyword、importance、status |
| 主图 | force-directed graph；节点大小=importance；边粗细=weight |
| 右侧详情 | entity 属性、aliases、description、facts、relations、versions |
| 底部 mini map | 当前图谱范围 |
| 操作 | 新建实体、合并、重命名、归档、导出 JSON/GraphML |

### 10.3 降级体验

图谱可能数据量大，必须提供非图形视图：

- 实体表格
- 邻居列表
- 关系表格
- 搜索结果列表

小屏默认进入列表视图，用户可手动打开图谱。

### 10.4 Entity 详情

| Tab | 内容 |
|-----|------|
| Overview | name、type、description、aliases、importance、confidence |
| Relations | outgoing / incoming relations |
| Facts | 关联 L3 facts |
| Evidence | episode / fact / message 证据链 |
| Versions | merge / split / rename 历史 |

---

## 11. Agent 进化（L4 Evolution）

### 11.1 页面目标

让用户知道 Agent 是否正在改变自己，以及为什么改变。

入口：

- Agent 详情：`/agents/:id/evolution`
- 全局提议中心：`/memory/evolution/proposals`

### 11.2 Identity 面板

展示并编辑：

- Persona
- Values
- Tone
- Domains
- User Expectations
- Current Phase

所有修改写入 `agent_evolution_events`，不可静默覆盖。

### 11.3 Strategy 面板

| 区域 | 内容 |
|------|------|
| 风格滑杆 | exploration、conciseness、caution、delegation |
| 工具偏好 | tool、preference_score、调用次数、失败率、blacklist |
| Provider 偏好 | provider、score、成功率、成本 |
| Model 偏好 | model、score、延迟、质量反馈 |

### 11.4 Proposal 审核中心

Proposal 卡片必须包含：

- 建议修改什么：target_field
- 当前值 vs 建议值：diff
- 为什么：rationale
- 证据：episodes / facts / feedback
- 风险：low / medium / high
- 预计影响：expected_impact
- 到期时间：expires_at

操作：

- 批准并应用
- 拒绝并填写原因
- 延后
- 查看证据

高风险 proposal 必须二次确认；影响 system prompt / tool blacklist 的 proposal 默认需要审核。

### 11.5 Evolution Log

时间线展示所有 EvolutionEvent：

- kind
- target_field
- before -> after diff
- trigger_kind
- evidence
- applied / reverted
- revert 按钮

回滚后必须生成新的 rollback event，而不是删除旧 event。

---

## 12. Agent 设置：记忆 Tab

Agent 设置中新增独立 `记忆` Tab，避免把所有记忆配置塞进基础 Agent 页。

### 12.1 总开关

顶部展示：

- 记忆总开关
- 当前记忆模式：轻量 / 标准 / 深度
- 预计 prompt 影响：低 / 中 / 高
- 隐私级别：严格 / 标准 / 宽松

推荐预设：

| 模式 | 说明 |
|------|------|
| 轻量 | L0 + L1；L3 recall 少量；L4 关闭 |
| 标准 | L0/L1/L2/L3 开启；L4 graph 仅后台抽取 |
| 深度 | L0-L4 全开；需要管理员审核进化 |

### 12.2 L0 子区：上下文策略

包含 `12 memory-L0-sensory.md` 中的配置：

- 最近窗口轮数
- 最近窗口 tokens
- 摘要触发阈值
- 摘要后保留最近轮数
- 裁剪策略
- 注入 L1/L2/L3/L4 开关
- 快照模式

### 12.3 L1 子区：工作记忆

- 启用 L1
- 任务总预算 tokens
- 单字段上限 tokens
- 历史版本保留数
- 默认 Schema
- 闲置归档分钟

### 12.4 L2 子区：会话事件

- 启用 episode
- 最低 importance
- 启用索引
- embedding model
- 是否允许 L2 recall 注入 L0
- retention days
- archive after days

### 12.5 L3 子区：知识记忆

- 启用 L3
- recall topK
- min score
- recall scopes
- embedding model
- decay interval
- archive threshold
- max chars per recall

### 12.6 L4 子区：图谱与进化

- 启用 L4
- 注入图邻居
- max neighbors
- max hops
- 注入身份
- 注入策略
- 启用自我演化
- 低风险自动应用
- proposal TTL
- throttle hours

### 12.7 配置防呆

- 当 `l3_enabled=false` 时，自动禁用 `l0_inject_l3`。
- 当 `l4_enabled=false` 时，自动禁用 graph recall，但 identity inject 可单独保留。
- 当 `evo_auto_apply=true` 时，显示风险提示，并要求 admin 权限。
- 当 prompt 预计占比过高时，展示「当前配置可能导致上下文超限」。

---

## 13. 调试工具

### 13.1 Prompt Preview

入口：`/memory/debug/prompt`

用户输入：

- session
- agent
- user_message
- provider / model
- context_window
- reserved_for_output

输出：

- prompt 分段瀑布
- token 占比
- recall 来源
- warning
- 仅 preview 的 prompt messages

### 13.2 Recall Tester

拆成三类：

| 页面 | 用途 |
|------|------|
| L2 Recall Tester | 查 episode 是否能被找回 |
| L3 Recall Tester | 查 fact 排名、score、scope |
| L4 Graph Recall Tester | 查实体邻居和路径 |

调试器必须支持：

- 输入 query
- 调整 topK / min score
- 查看 raw score 和 final score
- 查看将注入 prompt 的 markdown
- 保存为测试用例

---

## 14. 隐私、信任与可控性

### 14.1 用户可见的隐私标签

| 标签 | 说明 |
|------|------|
| Private | 仅用户可见 |
| Agent-only | 仅当前 Agent 使用 |
| Team | Team 共享 |
| Workspace | Workspace 共享 |
| Global | 平台公共 |
| PII | 含敏感信息，默认脱敏 |
| Prompted | 最近进入过 prompt |
| Auto-created | 系统自动生成 |
| User-confirmed | 用户确认过 |

### 14.2 删除与遗忘

用户点击「忘记」时提供三种选择：

| 选项 | 行为 |
|------|------|
| 暂停使用 | status=archived，不再 recall |
| 删除这条记忆 | soft delete，保留 audit |
| 删除并申请清除证据 | 合规流程，可能影响 L2/L3/L4 证据链 |

### 14.3 审计

所有以下操作必须进入 audit：

- fact 创建、修改、删除、回滚
- feedback confirm/reject/refine
- conflict resolve
- episode consolidate
- entity merge/rename/archive
- proposal approve/reject/apply
- evolution revert
- 完整 prompt content 查看

---

## 15. 视觉与交互规范

### 15.1 风格

适合 aranea 的记忆界面风格：

- 数据产品 / AI 控制台风格
- 高信息密度
- 圆角卡片 + 清晰分割线
- 轻量图表和 timeline
- 深色模式优先可读

### 15.2 颜色语义

| 语义 | 用途 |
|------|------|
| primary | 当前选择、主 CTA |
| positive | 健康、已确认、成功 |
| warning | 近阈值、待处理、待审核 |
| negative | 冲突、失败、超限、拒绝 |
| info | 自动生成、调试信息 |
| muted | 归档、禁用、低置信 |

颜色不能作为唯一信息来源，必须配合文字和 icon。

### 15.3 可访问性

- 所有 icon-only button 必须有 aria-label / tooltip。
- 主要表格支持键盘排序和焦点状态。
- diff 不能只靠红绿，应增加 `-` / `+` 标识。
- 图谱必须提供表格替代视图。
- token progress 必须展示数值文本。
- 所有 destructive action 需要确认，并提供 undo 或回滚路径。

### 15.4 加载与错误

| 场景 | UI |
|------|----|
| 列表加载 | skeleton rows |
| 图谱加载 | skeleton graph + 文案 |
| recall 失败 | error banner + retry |
| embedding pending | chip `Embedding pending` |
| consolidation error | row error state + 查看错误 |
| 权限不足 | 解释可见范围，不只显示 403 |

---

## 16. 前端组件建议（Quasar / Vue）

| 用途 | 组件 |
|------|------|
| 页面容器 | `QPage` + responsive grid |
| 顶部筛选 | `QToolbar` / `QSelect` / `QInput` / `QBtnToggle` |
| KPI | `QCard` + `QLinearProgress` |
| 数据表 | `QTable`，大量数据用 server pagination |
| 时间线 | `QTimeline` 或 `QVirtualScroll` |
| 详情 | `QDrawer` desktop，`QDialog` mobile |
| 分段预览 | `QExpansionItem` + `<pre>` |
| Diff | 双栏 `<pre>` + semantic colors |
| 图谱 | Cytoscape.js / D3；小屏降级列表 |
| 表单 | `QForm` + inline validation |
| 状态 | `QChip` + icon + text |

---

## 17. API 汇总

本 UI 不要求新增所有 API，但需要聚合五份文档中的接口。

| 页面 | API 来源 |
|------|----------|
| 上下文窗口 | L0 `/sessions/{id}/l0/snapshots`、`/l0/preview` |
| 工作记忆 | L1 `/sessions/{sid}/l1/tasks`、fields、history |
| 事件时间线 | L2 `/sessions/{sid}/l2/events` |
| Episodes | L2 `/sessions/{sid}/l2/episodes` |
| Marks | L2 `/sessions/{sid}/l2/marks` |
| Facts | L3 `/memory/l3/facts` |
| Conflicts | L3 `/memory/l3/conflicts` |
| L3 Recall | L3 `/memory/l3/recall` |
| Graph | L4 `/memory/l4/entities`、relations、neighborhood |
| Evolution | L4 `/agents/{id}/identity`、strategy、proposals、events |
| Settings | `PATCH /api/v1/agents/{id}/runtime-settings` |

---

## 18. 实施阶段

### Phase 1：用户可见的基础记忆中心

- [ ] `/memory` 总览页。
- [ ] L3 Facts 列表 + 详情抽屉。
- [ ] L3 新增 / 编辑 / 归档 / 删除 / confirm / reject。
- [ ] Agent 设置 -> 记忆 Tab 基础配置。
- [ ] Session 详情 -> 上下文 Tab 只读展示。

### Phase 2：会话复盘与工作记忆

- [ ] Session 详情 -> 工作记忆 Tab。
- [ ] Session 详情 -> 事件时间线。
- [ ] Episodes 列表与详情。
- [ ] Marks 标记与批量创建 milestone。
- [ ] Prompt Preview 调试器。

### Phase 3：知识治理

- [ ] L3 conflict 仲裁中心。
- [ ] L3 versions / rollback / feedback 时间线。
- [ ] L2/L3 recall tester。
- [ ] 巩固状态与失败重试 UI。
- [ ] PII 标签、脱敏查看、完整内容访问审计。

### Phase 4：图谱与进化

- [ ] L4 实体列表和详情。
- [ ] 图谱浏览器 MVP。
- [ ] Agent Identity / Strategy 面板。
- [ ] Evolution proposal 审核中心。
- [ ] Evolution log + revert。

### Phase 5：高级运维

- [ ] 全局 Memory Ops 管理页。
- [ ] embedding rebuild、decay、retention、consolidation 手动任务。
- [ ] 测试用例保存与回归对比。
- [ ] 导出 JSON / CSV / GraphML。

---

## 19. 验收标准

### 用户体验

- [ ] 用户能在 `/memory` 看懂当前 Agent 的记忆健康状态，无需理解 L0-L4。
- [ ] 用户能找到「为什么这次回答引用了某条记忆」的证据链。
- [ ] 用户能确认、反驳、修改、遗忘一条长期 fact。
- [ ] 用户能查看 session 的 prompt 分段和 token 占比。
- [ ] 用户能查看当前任务的 L1 字段，并回滚错误写入。
- [ ] 用户能从事件时间线定位失败工具调用并标记复盘。
- [ ] 用户能审核 Agent 进化 proposal，并一键回滚已应用事件。

### 安全与隐私

- [ ] 非管理员不能查看完整 prompt content，只能看 preview。
- [ ] PII fact 默认脱敏，并禁止扩大到 workspace/global scope。
- [ ] 删除、回滚、进化应用都有 audit 记录。
- [ ] 所有 scope 切换都遵守 user/team/workspace/global 权限。

### 可访问性与主题

- [ ] 深浅色模式下文字、chip、progress、diff、graph 均可读。
- [ ] 所有主要操作可键盘访问。
- [ ] 表格、图谱、时间线都有空状态、加载状态和错误状态。
- [ ] 图谱提供列表替代视图。

### 性能

- [ ] Facts / Events / Episodes 表格使用分页，不一次性加载大数据。
- [ ] Timeline 超过 100 条使用虚拟滚动。
- [ ] 图谱默认限制节点数，超过阈值提示缩小筛选。
- [ ] Recall tester 响应慢时显示 loading 和可取消状态。

---

## 20. 关键结论

记忆管理界面不是数据库管理后台，而是 Agent 的「记忆解释器」和「信任控制台」。

它需要把五层记忆统一成一条用户可理解的链路：

```text
当前 prompt 看到了什么
-> 当前任务状态是什么
-> 这次会话发生了什么
-> 哪些内容沉淀成长期知识
-> 哪些长期知识改变了 Agent 的行为
```

只要这条链路可见、可编辑、可回滚，用户就能信任 Agent 的记忆系统。
