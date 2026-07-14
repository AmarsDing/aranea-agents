# agency-agents 资源导入 aranea-agents 方案

> **日期**：2026-07-14
> **类型**：迁移方案
> **状态**：待评审
> **来源项目**：F:\agency-agents-main（The Agency — 230+ AI agent 模板库，MIT 许可）
> **目标项目**：aranea-agents（多智能体编排平台）

---

## 一、背景与目标

### 1.1 需求

将外部项目 `agency-agents-main` 的 230+ AI agent 模板整合后导入 aranea-agents 系统，替换现有预置 agent/team/公司架构。

### 1.2 约束

- 系统必需的 4 个 agent 不可清除：`__spirit__`（精灵）、`__memory__`（记忆管家）、`__skills__`（技能管家）、`__system_admin__`（系统管家）
- 公司数量 ≤ 4（最终定为 3 个）
- 一个 agent = 一个岗位（variant=general）
- Skills 跳过（不处理）
- Agent 默认 model = deepseek-chat（DeepSeek V4 Flash），其他配置用默认值
- 公司按业务闭环组织，支持跨部门任务流转

### 1.3 外部项目概况

| 维度 | 数据 |
|------|------|
| 行业（Divisions） | 17 个 |
| Agent 数量 | 230+ 个（每个是一个 .md 文件） |
| Agent 格式 | YAML frontmatter（name/description/color/emoji/vibe）+ Markdown body |
| Skills | 无独立 skills（嵌入在 agent 中）；有 strategy/playbooks/runbooks |
| 组织层级 | 仅两级：division → agent |

### 1.4 本项目数据模型

- **Organization 表**：company → department → position 三级层次
- **Agent 表**：agent_key, display_name, provider, model, position_key, agent_variant, config_json 等
- **关联表**：agent_runtime_settings（运行时配置）、agent_prompt_files（提示词文件）
- **数据加载**：`internal/scenario/organization.yaml` 定义层级；`{company}/agents.yaml` 定义 agent；`{company}/prompts/positions/{position_key}/{variant}.md` 存放 system prompt

---

## 二、3 公司划分方案

### 2.1 公司 1：数字内容与媒体传播公司

**业务闭环**：创意构思 → 内容创作 → 视觉设计 → 媒体发布 → 付费推广 → 销售转化 → 财务管理 → 客户支持

**涵盖行业**：academic, design, marketing, paid-media, sales, finance, support, specialized（非合规部分）

| # | 部门 | 来源行业 | Agent 数量 | 主要岗位方向 |
|---|------|---------|-----------|------------|
| 1 | 创意策划部 | academic | 6 | 人类学家、地理学家、历史学家、叙事学家、心理学家、统计学家 |
| 2 | 品牌设计部 | design | 9 | UI设计师、UX研究员、UX架构师、品牌守护者、视觉叙事者、趣味注入者、图像提示工程师、包容视觉专家、人格走查专家 |
| 3 | 内容创作部 | marketing（内容创作部分） | ~8 | 内容创作者、图书联合作者、全球播客策略师、PR传播经理、邮件策略师、多平台发布者、视频优化专家、AEO基础架构师 |
| 4 | 媒体运营部 | marketing（平台运营部分） | ~18 | SEO专家、小红书专家、微信公众号、B站、抖音、知乎、微博、快手、Twitter、Instagram、Reddit、LinkedIn、TikTok、百度SEO、中国本地化、AI引用策略、代理搜索优化、社交媒体策略师 |
| 5 | 付费推广部 | paid-media | 7 | PPC策略师、搜索查询分析师、付费媒体审计师、追踪专家、广告创意策略师、程序化购买、付费社交策略师 |
| 6 | 跨境电商部 | marketing（电商部分） | ~4 | 中国电商运营、跨境电商、直播电商教练、私域运营 |
| 7 | 销售部 | sales + specialized/sales-outreach | 10 | 外呼策略师、发现教练、交易策略师、销售工程师、提案策略师、管道分析师、客户策略师、销售教练、销售外联、offer_lead_gen |
| 8 | 财务部 | finance + specialized/CFO | 6 | 记账_controller、财务分析师、FP&A、投资研究员、税务策略师、CFO |
| 9 | 客户支持部 | support | 6 | 支持响应员、分析报告员、财务追踪、基础设施维护、法务合规检查、执行摘要生成 |
| 10 | 专项服务部 | specialized（非合规部分） | ~35 | 参谋长、业务策略师、变革管理顾问、客户成功经理、运营经理、M&A整合、组织心理学家、HR入职、语言翻译、法律（计费/客户/文档审查）、贷款、房地产、零售退货、招聘、留学、供应链、定价分析师、ESG、企业培训、个人成长导师、政府数字售前、医疗客服、酒店客服、grant_writer、workflow_architect、strategy_duel、agents_orchestrator、LSP工程师、数据提取/整合/分发agent、身份信任/图谱、自动化治理、MCP Builder、文档生成器、Model QA、ZK Steward、Developer Advocate、文化智能策略师 |

### 2.2 公司 2：软件工程与数字科技产品公司

**业务闭环**：产品规划 → 项目管理 → 架构设计 → 开发实现（后端/前端/移动/游戏/空间）→ 测试 → 部署运维 → 安全保障 → 合规审计

**涵盖行业**：engineering, product, project-management, game-development, spatial-computing, testing, security, gis, specialized（合规部分）

| # | 部门 | 来源行业 | Agent 数量 | 主要岗位方向 |
|---|------|---------|-----------|------------|
| 1 | 产品部 | product | 5 | 产品经理、趋势研究员、反馈综合师、行为助推引擎、Sprint优先级 |
| 2 | 项目管理部 | project-management | 7 | 制片人、项目牧羊人、工作室运营、实验追踪、高级项目经理、Jira工作流、会议纪要 |
| 3 | 后端开发部 | engineering（后端/基础设施部分） | ~15 | 后端架构师、API平台工程师、数据库优化师、搜索相关性工程师、支付计费工程师、身份认证工程师、实时协作工程师、数据工程师、AI数据修复工程师、AI工程师、邮件智能工程师、语音AI集成、提示工程师、多Agent系统架构师、OrgScript工程师 |
| 4 | 前端开发部 | engineering（前端部分） | ~5 | 前端开发者、桌面应用工程师、i18n工程师、WebAssembly工程师、Section 508专家 |
| 5 | 移动开发部 | engineering（移动部分） | ~3 | 移动应用构建师、移动发布工程师、微信小程序开发者 |
| 6 | 游戏开发部 | game-development | 17 | 游戏设计师、关卡设计师、技术美术、游戏音频、叙事设计师 + Unity(4)/Unreal(4)/Godot(3)/Blender(1)/Roblox(3) |
| 7 | 空间计算部 | spatial-computing | 6 | XR接口架构师、macOS Metal工程师、XR沉浸式开发者、XR座舱交互、visionOS工程师、终端集成 |
| 8 | 质量保障部 | testing | 9 | 证据收集器、现实检查器、测试结果分析、性能基准、API测试师、工具评估师、工作流优化、可访问性审计、测试自动化工程师 |
| 9 | 运维部 | engineering（DevOps/SRE/网络/ITSM部分） | ~8 | SRE、DevOps自动化师、网络工程师、事件响应指挥官、IT服务经理、FinOps工程师、Drupal/WordPress性能、CMS开发者 |
| 10 | 架构部 | engineering（架构部分） | ~5 | 软件架构师、代码审查员、代码库入职工程师、最小变更工程师、技术文档作者 |
| 11 | CMS电商部 | engineering（CMS/电商部分） | ~5 | Drupal购物车、WordPress购物车、Drupal性能、WordPress性能、USWDS开发者 |
| 12 | 安全部 | security | 10 | 安全架构师、应用安全工程师、渗透测试师、云安全架构师、事件响应、威胁情报分析师、威胁检测工程师、高级SecOps、合规审计师、区块链安全审计师 |
| 13 | 合规审计部 | specialized（合规部分） | 3 | 数据隐私官、ESG可持续官、FedRAMP/RMF合规工程师 |
| 14 | GIS解决方案部 | gis | 13 | 技术顾问、解决方案工程师、GIS分析师、空间数据工程师、地理处理专家、GIS QA、GeoAI工程师、BIM/GIS专家、3D场景开发、空间数据科学家、无人机测绘、Web GIS开发、制图设计师 |

### 2.3 公司 3：医疗公司

**业务闭环**：临床证据 → 医疗创新 → 主权健康系统

**涵盖行业**：healthcare

| # | 部门 | 来源行业 | Agent 数量 | 主要岗位方向 |
|---|------|---------|-----------|------------|
| 1 | 临床证据部 | healthcare | 1 | 临床证据agent |
| 2 | 医疗创新部 | healthcare | 1 | 医疗创新策略师 |
| 3 | 主权健康部 | healthcare | 1 | 主权健康系统agent |

---

## 三、字段映射规则

### 3.1 公司（Organization → company level）

| aranea-agents 字段 | 来源 | 说明 |
|-------------------|------|------|
| org_key | 自定义 | 如 `digital_content_media` |
| name | 自定义 | 如 `数字内容与媒体传播公司` |
| description | 自定义 | 业务闭环描述 |
| level | 固定 | `company` |
| sort_order | 自定义 | 1/2/3 |
| icon | 自定义 | Lucide 图标名 |

### 3.2 部门（Organization → department level）

| aranea-agents 字段 | 来源 | 说明 |
|-------------------|------|------|
| org_key | 自定义 | 如 `creative_planning` |
| name | 自定义 | 如 `创意策划部` |
| description | 自定义 | 部门职责描述 |
| level | 固定 | `department` |
| parent_id | 父公司 ID | |
| sort_order | 自定义 | |

### 3.3 岗位（Organization → position level）

| aranea-agents 字段 | 来源 | 说明 |
|-------------------|------|------|
| org_key | agency-agents agent 文件名去前缀 | 如 `frontend_developer` |
| name | agency-agents frontmatter.name | 如 `Frontend Developer` → 中文名 `前端开发者` |
| description | agency-agents frontmatter.description | 原文保留（英文） |
| level | 固定 | `position` |
| parent_id | 父部门 ID | |
| sort_order | 自定义 | |
| skills_required | 从 agent markdown 的 Core Mission 提取关键词 | |
| responsibilities | 从 agent markdown 的 Core Mission 提取 | |

### 3.4 Agent（agents 表）

| aranea-agents 字段 | 来源 | 说明 |
|-------------------|------|------|
| agent_key | `{position_key}__general` | 如 `frontend_developer__general` |
| display_name | agency-agents frontmatter.name | 英文名 |
| agent_description | agency-agents frontmatter.description | 英文描述 |
| provider | 固定 | `deepseek` |
| model | 固定 | `deepseek-chat` |
| position_key | 对应岗位 key | |
| position_id | 对应岗位 ID | |
| agent_variant | 固定 | `general` |
| variant_description | 固定 | `通用` |
| system_prompt_mode | 固定 | `file` |
| context_window | 固定 | 64000 |
| kind | 固定 | `ecosystem_preset` |
| source | 固定 | `imported` |
| icon | agency-agents frontmatter.emoji | |
| status | 固定 | `active` |
| config_json | 默认 | `""` |

### 3.5 System Prompt（agent_prompt_files 表）

| aranea-agents 字段 | 来源 | 说明 |
|-------------------|------|------|
| file_name | `{variant}.md` | `general.md` |
| body | agency-agents markdown body | 去掉 frontmatter 的完整 markdown 内容 |

### 3.6 默认运行时配置（agent_runtime_settings）

| 字段 | 值 | 说明 |
|------|-----|------|
| tools_enabled | true | |
| tools_profile | `general` | |
| tools_deny_json | `["workspace_exec","filesystem","shell","bash"]` | 安全默认 |
| memory_enabled | true | |
| skill_load_mode | `auto` | |
| code_executor_type | `local` | |
| intent_pass_enabled | true | |
| l0_inject_l1 | true | 注入岗位职责 |
| l1_enabled | true | |

---

## 四、实施计划

### Phase 1：清理现有数据

**目标**：清除非系统的 agent/team/公司架构，保留 4 个系统必需 agent

**清除范围**：
- `organizations` 表：删除所有非 `is_system=true` 的记录（company/department/position 全部）
- `agents` 表：删除所有 `kind != 'system_builtin'` 的记录（保留 `__spirit__`/`__memory__`/`__skills__`/`__system_admin__`）
- `teams` 表：删除所有 `kind != 'system_builtin'` 的记录
- `agent_runtime_settings` 表：删除对应被清除 agent 的设置
- `agent_prompt_files` 表：删除对应被清除 Agent 的提示词文件

**保留**：
- 4 个系统 agent 及其 runtime settings 和 prompt files
- 用户创建的会话记录（sessions/turns/steps 等）
- 系统配置

### Phase 2：构建新的公司架构

**目标**：修改 `internal/scenario/organization.yaml`，定义 3 个公司及其部门/岗位

**产出**：
- 修改后的 `internal/scenario/organization.yaml`
- 3 个公司目录：`internal/scenario/{company_key}/`

### Phase 3：转换 agency-agents 资源

**目标**：将 230+ 个 agent markdown 转换为 aranea-agents 格式

**产出**：
- 每个公司的 `agents.yaml`（agent 定义）
- 每个岗位的 `prompts/positions/{position_key}/general.md`（system prompt 文件）

**转换逻辑**：
1. 读取 agency-agents 的每个 .md 文件
2. 解析 YAML frontmatter 提取 name/description/color/emoji/vibe
3. 去掉 frontmatter，保留 markdown body 作为 system prompt
4. 根据文件所在目录（division）映射到对应的部门
5. 生成 position key（从文件名提取，如 `engineering-frontend-developer.md` → `frontend_developer`）
6. 生成 agent key（`{position_key}__general`）

### Phase 4：数据导入

**目标**：将新架构导入数据库

**方式**：通过系统启动时的 scenario loader 自动加载
- 修改 `organization.yaml` → 启动时自动加载公司/部门/岗位
- 修改各公司的 `agents.yaml` → 启动时自动加载 agent 定义
- 创建 prompt 文件 → 启动时自动加载 system prompt

### Phase 5：验证

- 系统正常启动，无报错
- 数据库中 3 个公司/所有部门/所有岗位/所有 agent 正确创建
- 前端 UI 能正确显示新的公司架构和 agent 列表
- 4 个系统 agent 保持不变

---

## 五、风险与注意事项

### 5.1 数据准确性

- **公司描述**：需要为 3 个公司编写准确的业务闭环描述
- **部门描述**：需要为 ~30 个部门编写准确的职责描述
- **岗位描述**：直接使用 agency-agents 的 description（英文），如有需要可后续翻译
- **岗位职责**：从 agency-agents agent 的 Core Mission 部分提取

### 5.2 技术风险

- **Agent 数量大**：230+ agent 的 prompt 文件需要批量生成，可能需要脚本辅助
- **Position key 冲突**：不同 division 可能有同名 agent（如 cross-border-ecommerce 在 marketing 和 specialized 都可能有），需要全局唯一性检查
- **数据库迁移**：清除现有数据是不可逆操作，建议先备份

### 5.3 不处理的内容

- **Skills**：跳过，不创建新的 skill 实体，也不清除现有 skills
- **Teams**：清除所有非系统 team，不创建新 team（team 由用户使用时按需创建）
- **Graphs**：清除现有 graph 定义，不创建新 graph
- **Schemas**：不处理 output schema

---

## 六、待确认事项

1. **DeepSeek model 名称**：用户提到"DeepSeek V4 Flash"，系统中 DeepSeek provider 的标准 model 名称为 `deepseek-chat`。方案使用 `deepseek-chat`，如用户有其他确切名称请告知。
2. **岗位中文名**：agency-agents 的 agent 名称为英文。是否需要翻译为中文？还是保留英文名？
3. **agent_description 语言**：保留英文原文，还是翻译为中文？
4. **现有 sessions 数据**：清除 agent/team 后，历史会话引用的 agent_id 将失效。是否需要一并清除会话数据？还是保留但标记为孤儿数据？
5. **Skills 目录**：跳过 skills 是否意味着保留现有的 `internal/scenario/{finance,selfmedia,softwaredev}/skills/` 目录不动？还是也一并清除？
