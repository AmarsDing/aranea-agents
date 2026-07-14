# agency-agents 资源导入 aranea-agents 方案

> **日期**：2026-07-14
> **类型**：迁移方案
> **状态**：待评审（v3 — 修复 system_prompt_mode、position_key 格式、sort_order 行为等关键问题）
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
- Agent 默认 model = `deepseek-chat`（DeepSeek V4 Flash），provider = `deepseek`
- 公司按业务闭环组织，支持跨部门任务流转
- **Agent 名称和字段描述尽量用中文**
- **单个 agent 文件拆分为多个中文描述文件**（类似系统 agent 的 IDENTITY.md / DECISION.md / CAPABILITIES.md 模式）

### 1.3 外部项目概况

| 维度 | 数据 |
|------|------|
| 行业（Divisions） | 17 个 |
| Agent 数量 | 230+ 个（每个是一个 .md 文件） |
| Agent 格式 | YAML frontmatter（name/description/color/emoji/vibe）+ Markdown body |
| Markdown body 结构 | Identity & Memory → Core Mission → Critical Rules → Technical Deliverables → Workflow Process → Deliverable Template → Communication Style → Learning & Memory → Success Metrics → Advanced Capabilities |
| Skills | 无独立 skills（嵌入在 agent 中） |
| 组织层级 | 仅两级：division → agent |

### 1.4 本项目数据模型与导入机制

**数据模型**：
- `organizations` 表：company → department → position 三级层次
- `agents` 表 + `agent_runtime_settings` 表 + `agent_prompt_files` 表
- `agent_prompt_files` 表支持一个 agent 有**多个 prompt 文件**（按 sort_order 排序）

**系统对特定 prompt 文件名的特殊语义处理**（经代码验证）：

| 文件名 | 系统行为 | 生效条件 | 代码位置 |
|--------|---------|---------|---------|
| `IDENTITY.md` | 跳过 display_name 自动注入（`IdentityDescriptionForAgent` 返回空字符串） | `system_prompt_mode` 为 `complete` 或 `task` | `internal/agent/prompt_mode.go:50-55` |
| `CAPABILITIES.md` | 跳过 tool cue 自动注入，替换为 "Tools: see CAPABILITIES.md in instruction" | `system_prompt_mode` 为 `complete` 或 `task` | `internal/agent/prompt.go:164,200` |
| `RULE.md` | 无特殊语义处理，但在 `task`/`minimized`/未知模式下始终保留 | — | `internal/biz/agent_settings_helpers.go:538-551` |
| `DECISION.md` | 无特殊语义处理（仅系统 agent 约定使用） | — | — |

**`system_prompt_mode` 有效值与文件过滤行为**（`internal/biz/agent_settings_helpers.go:527-560`）：

| mode | 包含的 prompt 文件 | capability cue 级别 |
|------|------------------|-------------------|
| `complete` | **全部文件** | full（最详细） |
| `task` | AGENTS_CORE, IDENTITY, RULE, AGENTS_TASK, CAPABILITIES | standard |
| `minimized` | AGENTS_CORE, RULE | compact |
| `none` | 无文件 | minimal |
| `file`（**无效**） | 回退到 default → 仅 AGENTS_CORE, RULE | full（但文件被过滤） |

> **关键纠正**：之前方案使用 `system_prompt_mode: file` 是错误的。`file` 不是有效模式，会被当作未知值处理，导致 IDENTITY.md/CAPABILITIES.md 等文件被过滤掉。**必须使用 `complete`** 模式才能确保所有 prompt 文件被包含。

**导入路径**（唯一工作的路径）：
```
Pack 目录结构（文件系统）
  → pack.ReadPackFromFS() 解析为 Pack 内存模型
  → pack.Importer.Import() 写入数据库
```

**Pack 目录结构**：
```
pack-root/
├── manifest.yaml              # 元数据
├── taxonomy.yaml              # 组织架构（company → department → position）
├── agents/
│   ├── {agent_key}.yaml       # Agent 配置（字段定义）
│   └── {agent_key}/           # Agent 描述文件目录（多个 .md 文件）
│       ├── IDENTITY.md        # 身份与记忆
│       ├── MISSION.md         # 核心使命
│       ├── RULE.md            # 关键规则
│       ├── CAPABILITIES.md    # 高级能力
│       ├── WORKFLOW.md        # 工作流程
│       ├── DELIVERABLES.md    # 技术交付物
│       └── COMMUNICATION.md   # 沟通与成长
├── teams/
└── graphs/
```

---

## 二、3 公司划分方案

### 2.1 公司 1：数字内容与媒体传播公司

**业务闭环**：创意构思 → 内容创作 → 视觉设计 → 媒体发布 → 付费推广 → 销售转化 → 财务管理 → 客户支持

**涵盖行业**：academic, design, marketing, paid-media, sales, finance, support, specialized（非合规部分）

| # | 部门 | 来源行业 | Agent 数量 | 主要岗位方向 |
|---|------|---------|-----------|------------|
| 1 | 创意策划部 | academic | 6 | 人类学家、地理学家、历史学家、叙事学家、心理学家、统计学家 |
| 2 | 品牌设计部 | design | 9 | UI设计师、UX研究员、UX架构师、品牌守护者、视觉叙事者等 |
| 3 | 内容创作部 | marketing（内容创作部分） | ~8 | 内容创作者、图书联合作者、播客策略师、PR传播经理等 |
| 4 | 媒体运营部 | marketing（平台运营部分） | ~18 | SEO专家、小红书/B站/抖音/知乎/微博/快手等平台专家 |
| 5 | 付费推广部 | paid-media | 7 | PPC策略师、搜索查询分析师、程序化购买等 |
| 6 | 跨境电商部 | marketing（电商部分） | ~4 | 中国电商运营、跨境电商、直播电商教练等 |
| 7 | 销售部 | sales + specialized/sales | 10 | 外呼策略师、交易策略师、销售工程师、提案策略师等 |
| 8 | 财务部 | finance + specialized/CFO | 6 | 记账_controller、财务分析师、FP&A、税务策略师、CFO等 |
| 9 | 客户支持部 | support | 6 | 支持响应员、分析报告员、基础设施维护等 |
| 10 | 专项服务部 | specialized（非合规部分） | ~35 | 参谋长、业务策略师、变革管理顾问、客户成功经理、运营经理、HR入职、法务、供应链等 |

### 2.2 公司 2：软件工程与数字科技产品公司

**业务闭环**：产品规划 → 项目管理 → 架构设计 → 开发实现 → 测试 → 部署运维 → 安全保障 → 合规审计

**涵盖行业**：engineering, product, project-management, game-development, spatial-computing, testing, security, gis, specialized（合规部分）

| # | 部门 | 来源行业 | Agent 数量 | 主要岗位方向 |
|---|------|---------|-----------|------------|
| 1 | 产品部 | product | 5 | 产品经理、趋势研究员、反馈综合师等 |
| 2 | 项目管理部 | project-management | 7 | 制片人、项目牧羊人、高级项目经理等 |
| 3 | 后端开发部 | engineering（后端部分） | ~15 | 后端架构师、API平台工程师、数据库优化师等 |
| 4 | 前端开发部 | engineering（前端部分） | ~5 | 前端开发者、桌面应用工程师等 |
| 5 | 移动开发部 | engineering（移动部分） | ~3 | 移动应用构建师、移动发布工程师等 |
| 6 | 游戏开发部 | game-development | 17 | 游戏设计师 + Unity/Unreal/Godot/Blender/Roblox 工程师 |
| 7 | 空间计算部 | spatial-computing | 6 | XR架构师、visionOS工程师、macOS Metal工程师等 |
| 8 | 质量保障部 | testing | 9 | 测试自动化工程师、性能基准师、API测试师等 |
| 9 | 运维部 | engineering（DevOps/SRE部分） | ~8 | SRE、DevOps自动化师、网络工程师等 |
| 10 | 架构部 | engineering（架构部分） | ~5 | 软件架构师、代码审查员、技术文档作者等 |
| 11 | 安全部 | security | 10 | 安全架构师、渗透测试师、云安全架构师等 |
| 12 | 合规审计部 | specialized（合规部分） | 3 | 数据隐私官、ESG可持续官、合规审计师 |
| 13 | GIS解决方案部 | gis | 13 | GIS分析师、空间数据工程师、GeoAI工程师等 |

### 2.3 公司 3：医疗公司

**业务闭环**：临床证据 → 医疗创新 → 主权健康系统

**涵盖行业**：healthcare

| # | 部门 | 来源行业 | Agent 数量 | 主要岗位方向 |
|---|------|---------|-----------|------------|
| 1 | 临床证据部 | healthcare | 1 | 临床证据agent |
| 2 | 医疗创新部 | healthcare | 1 | 医疗创新策略师 |
| 3 | 主权健康部 | healthcare | 1 | 主权健康系统agent |

---

## 三、Prompt 文件拆分策略（核心）

### 3.1 拆分原则

将 agency-agents 的单个 agent markdown（包含多个 section）拆分为 **5-7 个中文描述文件**，每个文件对应 agent 的一个方面。

利用系统对特定文件名的特殊语义处理（仅在 `system_prompt_mode: complete` 下生效）：
- `IDENTITY.md` — 系统检测到后跳过 display_name 自动注入
- `CAPABILITIES.md` — 系统检测到后跳过 tool cue 自动注入
- `RULE.md` — 通用规则

**Prompt 文件排序机制**（经代码验证，`internal/biz/pack/importer.go:413-426`）：

导入时，prompt 文件按**文件名字母序**排序后分配 `sort_order`（0, 1, 2, ...）。`BuildSystemPrompt` 按此顺序拼接文件内容，每个文件包裹在 `<internal_config name="...">` 标签中。

| 文件名 | 字母序 | sort_order | 能否改名 |
|--------|--------|-----------|---------|
| `CAPABILITIES.md` | 1 | 0 | ❌ 系统按名称检测 |
| `COMMUNICATION.md` | 2 | 1 | ✅ |
| `DELIVERABLES.md` | 3 | 2 | ✅ |
| `IDENTITY.md` | 4 | 3 | ❌ 系统按名称检测 |
| `MISSION.md` | 5 | 4 | ✅ |
| `RULE.md` | 6 | 5 | ✅（建议保留，task/minimized 模式白名单） |
| `WORKFLOW.md` | 7 | 6 | ✅ |

> **注意**：字母序导致 CAPABILITIES.md 排在第一位、IDENTITY.md 排在第四位。由于每个文件被包裹在独立的 `<internal_config>` 标签中，LLM 可以独立解析各块，顺序不影响语义正确性。如需强制逻辑顺序（身份→使命→规则→能力→流程→交付→沟通），需要修改导入器代码支持显式 sort_order——**本次不做此修改，接受字母序**。

### 3.2 文件拆分映射表

| 目标文件名 | 来源 section（agency-agents） | 内容说明 | 系统特殊处理 |
|-----------|-------------------------------|---------|-------------|
| `IDENTITY.md` | `## 🧠 Your Identity & Memory` | 身份与记忆：角色、性格、记忆、经验 | 跳过 display_name 注入 |
| `MISSION.md` | `## 🎯 Your Core Mission` | 核心使命：主要职责和目标 | 无 |
| `RULE.md` | `## 🚨 Critical Rules You Must Follow` | 关键规则：必须遵守的规则 | 通用规则文件 |
| `CAPABILITIES.md` | `## 🚀 Advanced Capabilities` | 高级能力：专业技能和高级能力 | 跳过 tool cue 注入 |
| `WORKFLOW.md` | `## 🔄 Your Workflow Process` | 工作流程：工作步骤和流程 | 无 |
| `DELIVERABLES.md` | `## 📋 Your Technical Deliverables` + `## 📋 Your Deliverable Template` | 技术交付物：交付物示例和模板 | 无 |
| `COMMUNICATION.md` | `## 💭 Your Communication Style` + `## 🔄 Learning & Memory` + `## 🎯 Your Success Metrics` | 沟通与成长：沟通风格、学习记忆、成功指标 | 无 |

**注意**：不是所有 agent 都有所有 section。缺少的 section 不创建对应文件。最少必须有 `IDENTITY.md`。

### 3.3 中文化策略

| 字段/内容 | 语言 | 说明 |
|----------|------|------|
| 公司名称 | 中文 | 如"数字内容与媒体传播公司" |
| 公司描述 | 中文 | 业务闭环描述 |
| 部门名称 | 中文 | 如"创意策划部" |
| 部门描述 | 中文 | 部门职责描述 |
| 岗位名称 | 中文 | 如"前端开发者"（从英文名翻译） |
| 岗位描述 | 中文 | 从 agency-agents description 翻译 |
| 岗位职责 | 中文 | 从 Core Mission 提取并翻译 |
| Agent display_name | 中文 | 与岗位名称一致 |
| Agent description | 中文 | 与岗位描述一致 |
| Prompt 文件内容 | 中文 | 将 agency-agents 的英文 markdown 翻译为中文 |
| 代码示例 | 保留英文 | DELIVERABLES.md 中的代码块保留英文 |
| 技术术语 | 保留英文 | 如 React, Vue, API, SQL 等 |

### 3.4 拆分示例

以 `engineering-frontend-developer.md` 为例，拆分后的文件结构：

```
agents/frontend_developer__general/
├── IDENTITY.md         # 身份与记忆（中文）
├── MISSION.md          # 核心使命（中文）
├── RULE.md             # 关键规则（中文）
├── CAPABILITIES.md     # 高级能力（中文）
├── WORKFLOW.md         # 工作流程（中文）
├── DELIVERABLES.md     # 技术交付物（中文说明 + 英文代码）
└── COMMUNICATION.md    # 沟通与成长（中文）
```

**对应的 agent yaml**（`agents/frontend_developer__general.yaml`）：
```yaml
key: frontend_developer__general
display_name: 前端开发者
description: 专精于现代Web技术、React/Vue/Angular框架、UI实现和性能优化的前端开发专家
icon: 🖥️
position_key: digital_tech/software_engineering/frontend_developer
variant: general
variant_description: 通用
provider: deepseek
model: deepseek-chat
model_tier: fast
system_prompt_mode: complete
context_window: 64000
tools_deny: [workspace_exec, filesystem, shell, bash]
ownership_kind: ecosystem_preset
source: imported
files:
  - name: IDENTITY.md
  - name: MISSION.md
  - name: RULE.md
  - name: CAPABILITIES.md
  - name: WORKFLOW.md
  - name: DELIVERABLES.md
  - name: COMMUNICATION.md
```

**IDENTITY.md 示例内容**（中文翻译后）：
```markdown
## 你是谁

你是**前端开发者**，一位专精于现代 Web 技术、UI 框架和性能优化的前端开发专家。
你创建响应式、可访问、高性能的 Web 应用，追求像素级精确的设计实现和卓越的用户体验。

## 身份与记忆
- **角色**：现代 Web 应用与 UI 实现专家
- **性格**：注重细节、关注性能、以用户为中心、技术精准
- **记忆**：你记住成功的 UI 模式、性能优化技巧和可访问性最佳实践
- **经验**：你见过应用因卓越的 UX 而成功，也因糟糕的实现而失败
```

---

## 四、字段映射规则

### 4.1 公司（taxonomy.yaml → company level）

| Pack 字段 | 来源 | 示例 |
|-----------|------|------|
| key | 自定义（英文 snake_case） | `digital_content_media` |
| name | 中文 | `数字内容与媒体传播公司` |
| description | 中文 | `创意构思→内容创作→视觉设计→媒体发布→付费推广→销售→财务→客服` |
| icon | Lucide 图标名 | `megaphone` |
| sort_order | 数字 | 1 |

**3 家公司的完整 key 映射**：

| 公司 | key | name | icon | sort_order |
|------|-----|------|------|-----------|
| 数字内容与媒体传播公司 | `digital_content_media` | 数字内容与媒体传播公司 | `megaphone` | 1 |
| 软件工程与数字科技产品公司 | `digital_tech` | 软件工程与数字科技产品公司 | `code` | 2 |
| 医疗公司 | `healthcare` | 医疗公司 | `heart-pulse` | 3 |

### 4.2 部门（taxonomy.yaml → department level）

| Pack 字段 | 来源 | 示例 |
|-----------|------|------|
| key | 自定义（英文 snake_case） | `creative_planning` |
| name | 中文 | `创意策划部` |
| description | 中文 | 部门职责描述 |
| sort_order | 数字 | 1 |

### 4.3 岗位（taxonomy.yaml → position level）

| Pack 字段 | 来源 | 示例 |
|-----------|------|------|
| key | 从 agency-agents 文件名提取（英文 snake_case，全局唯一） | `frontend_developer` |
| name | 中文翻译 | `前端开发者` |
| description | 从 agency-agents description 翻译 | `专精于现代Web技术、React/Vue/Angular框架、UI实现和性能优化的前端开发专家` |
| seniority_level | 默认 | `senior` |
| skills_required | 从 agent markdown 提取关键词 | `["React", "Vue", "TypeScript", "CSS"]` |
| responsibilities | 从 Core Mission 提取（中文） | `["构建响应式Web应用", "实现像素级精确的设计", "优化Core Web Vitals性能"]` |
| variants | 固定 | `[{key: general, name: 通用}]` |

> **Position key 全局唯一性**：岗位 key 必须在所有公司/部门中全局唯一。不同部门下如有相似岗位，需加部门前缀消歧（如 `finance_analyst` vs `marketing_analyst`）。

### 4.4 Agent（agents/{agent_key}.yaml）

| Pack 字段 | 来源 | 示例 |
|-----------|------|------|
| key | `{position_key}__general` | `frontend_developer__general` |
| display_name | 中文（与岗位名称一致） | `前端开发者` |
| description | 中文（与岗位描述一致） | `专精于现代Web技术...` |
| icon | agency-agents frontmatter.emoji | `🖥️` |
| position_key | **完整路径** `company_key/department_key/position_key` | `digital_tech/software_engineering/frontend_developer` |
| variant | 固定 | `general` |
| variant_description | 中文 | `通用` |
| provider | 固定 | `deepseek` |
| model | 固定 | `deepseek-chat` |
| model_tier | 固定 | `fast` |
| system_prompt_mode | 固定 | **`complete`**（不是 `file`） |
| context_window | 固定 | 64000 |
| tools_deny | 默认 | `[workspace_exec, filesystem, shell, bash]` |
| ownership_kind | 固定 | `ecosystem_preset` |
| source | 固定 | `imported` |
| files | 声明引用的文件列表（仅元数据，不影响导入） | `[IDENTITY.md, MISSION.md, RULE.md, ...]` |

> **关键纠正**：
> 1. `system_prompt_mode` 必须为 `complete`，不是 `file`。`file` 是无效值，会导致大部分 prompt 文件被过滤掉。
> 2. `position_key` 必须为完整路径格式 `company_key/department_key/position_key`，不是仅 position key。导入器通过 `ParseOrgKeyPath` 解析此路径并提取最后一段作为 `agent.PositionKey`（`internal/biz/pack/importer.go:386-397`）。
> 3. `files` 字段仅作元数据声明，实际文件从 `agents/{agent_key}/` 目录读取（`internal/biz/pack/reader.go:50-60`）。

### 4.5 默认运行时配置（agent_runtime_settings）

| 字段 | 值 |
|------|-----|
| tools_enabled | true |
| tools_profile | `general` |
| memory_enabled | true |
| l0_inject_l1 | true |
| l1_enabled | true |
| intent_pass_enabled | true |

---

## 五、实施计划

### Phase 1：清理现有数据

**目标**：清除非系统的 agent/team/公司架构，保留 4 个系统必需 agent

**清除范围**：
- `organizations` 表：删除所有非系统记录
- `agents` 表：删除所有 `kind != 'system_builtin'` 的记录
- `teams` 表：删除所有 `kind != 'system_builtin'` 的记录
- `agent_runtime_settings` 表：删除对应被清除 agent 的设置
- `agent_prompt_files` 表：删除对应被清除 Agent 的提示词文件
- `graph_definitions` 表：删除非系统 graph 模板

**保留**：
- 4 个系统 agent 及其 runtime settings 和 prompt files
- 用户创建的会话记录（sessions/turns/steps 等）
- 系统配置

**实施方式**：通过数据库迁移脚本（DDL Migration Registry 注册）

### Phase 2：构建 Pack 资源

**目标**：创建完整的 Pack 目录结构，包含所有 agent 定义和中文描述文件

**产出目录**：`internal/scenario/packs/agency-pack/`

```
internal/scenario/packs/agency-pack/
├── manifest.yaml
├── taxonomy.yaml
├── agents/
│   ├── frontend_developer__general.yaml
│   ├── frontend_developer__general/
│   │   ├── IDENTITY.md
│   │   ├── MISSION.md
│   │   ├── RULE.md
│   │   ├── CAPABILITIES.md
│   │   ├── WORKFLOW.md
│   │   ├── DELIVERABLES.md
│   │   └── COMMUNICATION.md
│   ├── backend_architect__general.yaml
│   ├── backend_architect__general/
│   │   └── ...
│   └── ... (230+ agents)
└── (无 teams/ 和 graphs/ 目录)
```

### Phase 3：转换 agency-agents 资源

**目标**：将 254 个 agent markdown 转换为 Pack 格式

**规模估算**：
- 254 agent × 5-7 文件/agent = 1270-1778 个文件
- 每个 prompt 文件约 200-800 字
- 总翻译量约 25-140 万字

**实施策略**（分批转换，避免单次过载）：

**Phase 3a：编写转换工具**（Go 脚本，放在 `cmd/tools/agency-converter/`）
- 读取 `F:\agency-agents-main` 下所有 .md 文件
- 解析 YAML frontmatter（name/description/color/emoji/vibe）
- 按 section 标记（`## 🧠`/`## 🎯`/`## 🚨` 等）拆分 markdown body
- 根据文件所在 division 目录映射到 3 公司 × N 部门
- 生成 `manifest.yaml` + `taxonomy.yaml` + 所有 `agents/{key}.yaml`
- 生成 prompt 文件骨架（英文原文，待翻译）
- 输出中文翻译清单（agent name + description 对照表）

**Phase 3b：AI 翻译中文字段**（分部门批次进行）
- 按部门分批翻译 agent name/description（yaml 字段）
- 按部门分批翻译 prompt 文件内容（IDENTITY.md/MISSION.md/RULE.md 等）
- 翻译原则：代码块保留英文、技术术语保留英文（React/API/SQL 等）、其余翻译为中文

**Phase 3c：生成最终 Pack**
- 将翻译后的内容写入 `internal/scenario/packs/agency-pack/`
- 校验所有文件完整性

**转换步骤**（每个 agent）：
1. 读取 agency-agents 的 .md 文件
2. 解析 YAML frontmatter 提取 name/description/color/emoji/vibe
3. 将 name 翻译为中文 → 岗位名称 + agent display_name
4. 将 description 翻译为中文 → 岗位描述 + agent description
5. 按 section 拆分 markdown body
6. 将每个 section 翻译为中文，写入对应的 .md 文件
7. 根据文件所在目录（division）映射到对应的部门
8. 生成 position_key（全局唯一 snake_case）和 agent_key（`{position_key}__general`）
9. 生成 agent yaml 配置文件（含完整 position_key 路径）

**翻译原则**：
- 代码块保留英文
- 技术术语保留英文（React, API, SQL 等）
- 其余内容翻译为中文
- 保持原文的语义和结构

### Phase 4：注册 Pack 导入

**目标**：在系统启动时自动导入新 Pack

**实施方式**（参照 `SeedPackBuiltinTemplates` 模式）：

1. **创建 seed 函数**：在 `internal/data/seed_pack.go` 中添加 `SeedPackAgency` 函数
   ```go
   const SeedPackAgencyV1 = "seed_pack_agency_v1"

   func SeedPackAgency(ctx context.Context, client *ent.Client, d Dialect, scenarioDir string, lg loggateway.Logger) error {
       // 版本门控
       applied, err := isMigrationApplied(ctx, client, SeedPackAgencyV1, lg)
       if err != nil { return entErrToBizErr(err, "SEED") }
       if applied { return nil }

       // 从 scenarioDir 读取 agency-pack
       fsys := os.DirFS(filepath.Join(scenarioDir, ".."))
       p, readErr := pack.ReadPackFromFS(fsys, "scenario/packs/agency-pack")
       if readErr != nil { return entErrToBizErr(readErr, "SEED") }

       // 创建 Importer 并导入（ConflictOverwrite + ecosystem_preset）
       importer := newPackImporter(client, lg)
       result, importErr := importer.Import(ctx, p, pack.ConflictOverwrite, pack.WithKindOverride("ecosystem_preset"))
       if importErr != nil { return entErrToBizErr(importErr, "SEED") }

       lg.Info("agency-pack seed completed",
           loggateway.StepID("data.seed.pack_agency"),
           loggateway.Int("agents_created", result.AgentsCreated),
           loggateway.Int("agents_updated", result.AgentsUpdated),
           loggateway.Int("org_nodes", result.OrgNodes),
           loggateway.Int("failures", len(result.Failures)))

       // 记录版本
       if recordErr := recordMigrationApplied(ctx, client, d, SeedPackAgencyV1, "pack_agency_v1", lg); recordErr != nil {
           return entErrToBizErr(recordErr, "SEED")
       }
       return nil
   }
   ```

2. **注册 seed step**：在 `internal/data/data.go` 的 P1 阶段迁移序列中添加 `SeedPackAgency` 调用，位于 `SeedPackBuiltinTemplates` 之后

3. **版本门控**：使用 `schema_migrations` 表记录 `seed_pack_agency_v1`，确保只执行一次（重新导入需手动删除版本记录或使用 force 参数）

### Phase 5：验证

- 系统正常启动，无报错
- 数据库中 3 个公司 / 所有部门 / 所有岗位 / 所有 agent 正确创建
- 每个 agent 有 5-7 个中文 prompt 文件
- 前端 UI 能正确显示新的公司架构和 agent 列表
- 4 个系统 agent 保持不变
- Agent 详情页能正确显示中文描述

---

## 六、风险与注意事项

### 6.1 数据准确性

- **公司描述**：需要为 3 个公司编写准确的业务闭环描述（中文）
- **部门描述**：需要为 ~30 个部门编写准确的职责描述（中文）
- **岗位描述**：从 agency-agents description 翻译为中文
- **岗位职责**：从 agency-agents agent 的 Core Mission 提取并翻译
- **Prompt 文件内容**：翻译需保持原文语义，代码示例保留英文

### 6.2 技术风险

- **Agent 数量大**：230+ agent × 5-7 文件 = 1150-1610 个文件需要生成，必须用脚本辅助
- **Position key 冲突**：不同 division 可能有同名 agent，需要全局唯一性检查
- **翻译质量**：机器翻译可能不准确，关键 agent 需要人工校对
- **数据库迁移**：清除现有数据不可逆，建议先备份

### 6.3 不处理的内容

- **Skills**：跳过，不创建新 skill，也不清除现有 skills
- **Teams**：清除所有非系统 team，不创建新 team
- **Graphs**：清除现有非系统 graph，不创建新 graph

---

## 七、已确认事项（原待确认事项已解决）

1. **DeepSeek model 名称**：✅ 确认使用 `deepseek-chat`（系统中 DeepSeek provider 的标准名称）。用户提到的"DeepSeek V4 Flash"对应此 model。
2. **翻译方式**：✅ 全部由 AI 翻译（即由本助手在实施阶段完成翻译）。翻译原则：代码块保留英文、技术术语保留英文（React/API/SQL 等）、其余内容翻译为中文、保持原文语义和结构。
3. **现有 sessions 数据**：✅ **保留会话数据不清除**。清除 agent/team 后，历史会话引用的 agent_id 将失效，但会话记录本身保留（用户可在 UI 中看到历史对话，但无法继续与已删除 agent 对话）。如需清理可后续手动执行。
4. **现有 skills 目录**：✅ 保留 `internal/scenario/{finance,selfmedia,softwaredev}/skills/` 目录不动。本次不处理 skills（不清除、不创建）。
5. **Pack 导入时机**：✅ 系统启动时自动导入（类似 builtin-templates），通过 `SeedPackAgency` 函数在 P1 阶段执行，使用版本门控确保只导入一次。

---

## 八、代码验证附录（v3 新增）

以下结论均经实际代码验证，非猜测：

### 8.1 Prompt 文件特殊语义处理验证

| 验证项 | 代码位置 | 验证结论 |
|--------|---------|---------|
| IDENTITY.md 跳过 display_name | `internal/agent/prompt_mode.go:50-55` `IdentityDescriptionForAgent` | 仅当 `HasFilteredPromptFile` 返回 true 时跳过，即 IDENTITY.md 在 `FilesForMode` 结果中存在 |
| CAPABILITIES.md 跳过 tool cue | `internal/agent/prompt.go:164,200` `StaticRuntimeCapabilityCue`/`DynamicRuntimeCapabilityCue` | 同上，依赖 `HasFilteredPromptFile` |
| FilesForMode 白名单 | `internal/biz/agent_settings_helpers.go:527-560` | `complete` 返回全部文件；`file` 是无效值，回退到 default（仅 AGENTS_CORE + RULE） |
| capabilityCueLevelForMode | `internal/agent/prompt_mode.go:20-31` | `file` 回退到 default → `cueLevelFull`（但文件已被过滤，矛盾） |

### 8.2 Pack 导入流程验证

| 验证项 | 代码位置 | 验证结论 |
|--------|---------|---------|
| SeedPackBuiltinTemplates 模式 | `internal/data/seed_pack.go:21-68` | 使用 `os.DirFS` + `ReadPackFromFS` + `Importer.Import` + 版本门控 |
| Agent 文件读取 | `internal/biz/pack/reader.go:50-60` | 从 `agents/{agent_key}/{filename}` 路径读取，存入 `Pack.AgentFiles[agentKey][fileName]` |
| Prompt 文件 sort_order | `internal/biz/pack/importer.go:413-426` | 按文件名字母序排序，分配 sort_order 0,1,2,...（无显式 sort_order 支持） |
| position_key 解析 | `internal/biz/pack/importer.go:386-397` + `mapper.go:25-37,131-139` | 需完整路径 `company/dept/position`，通过 `ParseOrgKeyPath` 解析，`ResolvePositionKey` 查找映射 |
| ConflictOverwrite 行为 | `internal/biz/pack/importer.go:335-337,378-383` | 覆盖非保留字段，保留 Status/Readonly/Kind/Source |
| AgentFileRef 结构 | `internal/biz/pack/spec.go:226-228` | 仅 `Name` 字段，无 `sort_order` 字段 |

### 8.3 已知限制

1. **AgentFileRef 无 sort_order**：`AgentFileRef` 结构体只有 `Name` 字段，无法在 yaml 中指定文件排序。排序完全由文件名字母序决定。
2. **Files 字段仅元数据**：agent yaml 中的 `files` 字段不影响导入，实际文件从文件系统读取。
3. **file 模式的遗留问题**：现有 builtin-templates 中的 agent 使用 `system_prompt_mode: file`，但这些 agent 没有 prompt 文件子目录（仅靠 description 字段），所以 `file` 模式的过滤行为不影响它们。新导入的 agent 有 prompt 文件，必须用 `complete`。
