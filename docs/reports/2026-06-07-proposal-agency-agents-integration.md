# Agency-Agents 生态内容集成方案：丰富 Aranea 市场模块

> 日期：2026-06-07
> 前置文档：[agency-agents 分类分析报告](./2026-06-07-analysis-agency-agents-classification.md)
> 关联模块：M30 Ecosystem、M57 Marketplace Platform、EcosystemPreset、Pack Importer

---

## 一、现状分析

### 1.1 Aranea 市场模块现状

| 模块 | 状态 | 能力 |
|------|------|------|
| **M30 Ecosystem** | 技术预览 | Product CRUD + Install/Uninstall（仅记录关系，无实际部署） |
| **EcosystemPreset** | 已实现 | 行业预设加载/卸载，3 个行业（finance/selfmedia/softwaredev），87 个 Agent |
| **Pack Importer** | 已实现 | YAML → DB 导入（Agent + Team + Graph + Taxonomy），支持 kindOverride |
| **IndustryMarketPage** | 前端已实现 | 网格/表格视图、筛选、行业详情抽屉、安装向导（TODO） |
| **M57 Marketplace** | 需求草案 | 公网商城平台，10 类资产，独立服务，~24 周工期 |

### 1.2 Agency-Agents 生态内容

| 维度 | 数据 |
|------|------|
| 总 Agent 数 | 215 |
| 行业/部门 | 17 个部门，覆盖 12 个行业 |
| 中国原创 | 50 个（小红书/抖音/微信/飞书/钉钉/政务/医疗等） |
| Agent 文件格式 | Markdown（身份 + 规则 + 流程 + 交付物 + 工具） |

### 1.3 核心差距

| 维度 | Aranea 现有 | Agency-Agents 提供 | 差距 |
|------|------------|-------------------|------|
| 行业覆盖 | 3 个行业 | 12+ 个行业 | 9 个行业空白 |
| Agent 数量 | 87 个 | 215 个 | 128 个增量 |
| Agent 定义深度 | YAML 配置 + 文件 Prompt | Markdown（身份/规则/流程/交付物/工具） | 格式不同需转换 |
| 岗位粒度 | 3 个行业约 40 个岗位 | 17 个部门 100+ 岗位 | 岗位体系需扩展 |
| Skill 体系 | 有 Skill 框架但行业 Skill 少 | 每个 Agent 自带专业流程 | 需提取为 Skill |

---

## 二、集成策略：三阶段递进

### 核心原则

1. **复用现有管道**：EcosystemPreset + Pack Importer 已打通 YAML → DB 全链路，新增行业只需新增 YAML 数据
2. **格式桥接**：Agency-Agents 的 Markdown 格式需转换为 Aranea 的 YAML + Prompt 文件格式
3. **渐进扩展**：先补充行业分类和 Agent 模板，再补充 Skill 和 Team 编排
4. **与 M57 对齐**：数据模型预留 M57 的 Asset Schema 兼容性

---

### 阶段一：行业分类扩展 + Agent 模板入库（低风险，高价值）

**目标**：将 Agency-Agents 的 12 个行业分类体系和 215 个 Agent 定义转换为 Aranea YAML 格式，通过 EcosystemPreset 加载。

#### 2.1.1 扩展 taxonomy.yaml

在现有 3 个行业基础上，新增 9 个行业分类：

```yaml
industries:
  # === 现有（保留） ===
  - key: finance
    name: 金融
    # ... 保持不变

  - key: selfmedia
    name: 自媒体
    # ... 保持不变

  - key: softwaredev
    name: 软件开发
    # ... 保持不变

  # === 新增 ===
  - key: marketing
    name: 营销广告
    icon: campaign
    description: 社交媒体运营、付费媒体投放、内容营销、增长黑客
    sort_order: 4
    departments:
      - key: domestic_platform
        name: 国内平台运营
        positions:
          - key: xiaohongshu_operator
            name: 小红书运营专家
          - key: douyin_strategist
            name: 抖音策略师
          - key: wechat_operator
            name: 微信公众号运营
          # ... 21 个国内平台岗位

      - key: overseas_marketing
        name: 出海营销
        positions:
          - key: tiktok_strategist
            name: TikTok 策略师
          # ... 6 个出海岗位

      - key: general_marketing
        name: 通用营销
        positions:
          - key: growth_hacker
            name: 增长黑客
          # ... 9 个通用岗位

      - key: paid_media
        name: 付费媒体
        positions:
          - key: ppc_strategist
            name: PPC 竞价策略师
          # ... 7 个付费媒体岗位

  - key: sales
    name: 销售商务
    icon: handshake
    description: B2B 销售、客户拓展、投标策略
    sort_order: 5
    departments:
      - key: b2b_sales
        name: B2B 销售
        positions:
          - key: deal_strategist
            name: 赢单策略师
          # ... 8 个销售岗位

  - key: hr_legal
    name: 人力法务
    icon: gavel
    description: 招聘、绩效、合同审查、合规
    sort_order: 6
    departments:
      - key: human_resources
        name: 人力资源
        positions:
          - key: recruiter
            name: 招聘专家
          - key: performance_reviewer
            name: 绩效管理专家
      - key: legal
        name: 法务
        positions:
          - key: contract_reviewer
            name: 合同审查专家
          - key: policy_writer
            name: 制度文件撰写专家

  - key: supply_chain
    name: 供应链制造
    icon: local_shipping
    description: 库存预测、供应商管理、物流优化
    sort_order: 7
    departments:
      - key: supply_chain_ops
        name: 供应链运营
        positions:
          - key: inventory_forecaster
            name: 库存预测专家
          # ... 5 个供应链岗位

  - key: game_dev
    name: 游戏开发
    icon: sports_esports
    description: Unity/Unreal/Godot/Roblox 全引擎覆盖
    sort_order: 8
    departments:
      - key: game_design
        name: 游戏设计
      - key: unity_dev
        name: Unity 开发
      - key: unreal_dev
        name: Unreal 开发
      - key: godot_dev
        name: Godot 开发
      # ... 20 个游戏岗位

  - key: spatial_computing
    name: 空间计算
    icon: view_in_ar
    description: visionOS/XR/AR/VR 空间交互
    sort_order: 9
    departments:
      - key: xr_development
        name: XR 开发
        # ... 6 个空间计算岗位

  - key: embedded_industrial
    name: 嵌入式工业
    icon: precision_manufacturing
    description: 嵌入式固件、FPGA、IoT、上位机、机械设计
    sort_order: 10
    departments:
      - key: embedded_firmware
        name: 嵌入式固件
      - key: fpga_digital
        name: FPGA/数字设计
      - key: iot_solution
        name: IoT 方案
      - key: industrial_software
        name: 工业软件
      - key: mechanical_design
        name: 机械设计

  - key: education_academic
    name: 教育学术
    icon: school
    description: 学习规划、高考志愿、学术研究
    sort_order: 11
    departments:
      - key: academic_research
        name: 学术研究
      - key: education_planning
        name: 教育规划

  - key: specialized_vertical
    name: 垂直行业专项
    icon: business_center
    description: 医疗、政务、房地产、零售、酒店等垂直领域
    sort_order: 12
    departments:
      - key: healthcare
        name: 医疗健康
      - key: government
        name: 政务数字化
      - key: real_estate
        name: 房地产
      - key: retail
        name: 零售
      - key: hospitality
        name: 酒店服务
      - key: cross_industry
        name: 跨行业专项
```

#### 2.1.2 新增行业 YAML 数据目录

```
internal/scenario/
├── categories.yaml          # 扩展：新增 9 个行业
├── taxonomy.yaml            # 扩展：新增 9 个行业的完整分类
├── agent_templates.yaml     # 扩展：新增行业通用模板
├── finance/                 # 现有，保留
├── selfmedia/               # 现有，保留
├── softwaredev/             # 现有，保留
├── marketing/               # 新增
│   ├── agents.yaml
│   ├── prompts/positions/   # Agent system prompt 文件
│   │   ├── xiaohongshu_operator/
│   │   │   └── general.md
│   │   ├── douyin_strategist/
│   │   │   └── general.md
│   │   └── ...
│   └── skills/              # 行业专属 Skill
│       ├── xiaohongshu-content-creation/
│       │   └── SKILL.md
│       └── ...
├── sales/                   # 新增
│   ├── agents.yaml
│   └── prompts/positions/
├── hr_legal/                # 新增
│   ├── agents.yaml
│   └── prompts/positions/
├── supply_chain/            # 新增
│   ├── agents.yaml
│   └── prompts/positions/
├── game_dev/                # 新增
│   ├── agents.yaml
│   └── prompts/positions/
├── spatial_computing/       # 新增
│   ├── agents.yaml
│   └── prompts/positions/
├── embedded_industrial/     # 新增
│   ├── agents.yaml
│   └── prompts/positions/
├── education_academic/      # 新增
│   ├── agents.yaml
│   └── prompts/positions/
└── specialized_vertical/    # 新增
    ├── agents.yaml
    └── prompts/positions/
```

#### 2.1.3 Markdown → YAML + Prompt 转换

Agency-Agents 的每个 Agent 是一个 Markdown 文件，需转换为 Aranea 格式：

**输入**（Agency-Agents 格式）：
```markdown
# 小红书运营专家

## 身份
你是一位专注小红书平台的内容运营专家...

## 关键规则
1. 种草笔记必须包含...
2. 达人合作策略...

## 工作流程
1. 分析品牌定位 → 2. 制定内容策略 → ...

## 交付物
- 种草笔记文案
- 达人合作方案
- 数据复盘报告
```

**输出**（Aranea 格式）：

agents.yaml:
```yaml
defaults:
  provider: openrouter
  fast_model: gpt-4.1-mini
  strong_model: gpt-4.1
  tools_deny: [workspace_exec, filesystem, shell, bash]
  system_prompt_mode: file
  context_window: 64000

agents:
  - key: xiaohongshu-operator-general
    position_key: xiaohongshu_operator
    variant: general
    model_tier: strong
    skills: [xiaohongshu-content-creation]
    tools_allow: [web_search, web_fetch]
```

prompts/positions/xiaohongshu_operator/general.md:
```markdown
# 小红书运营专家

## 身份
你是一位专注小红书平台的内容运营专家...

## 关键规则
1. 种草笔记必须包含...
2. 达人合作策略...

## 工作流程
1. 分析品牌定位 → 2. 制定内容策略 → ...

## 交付物
- 种草笔记文案
- 达人合作方案
- 数据复盘报告
```

**转换规则**：

| Agency-Agents 字段 | Aranea 字段 | 转换逻辑 |
|-------------------|------------|----------|
| 文件名 `marketing-xiaohongshu-operator.md` | `key: xiaohongshu-operator-general` | `{部门前缀}-{岗位名}-{variant}` |
| 身份/规则/流程/交付物 | `prompts/positions/{position_key}/{variant}.md` | 整体作为 system prompt |
| 工具/技术栈 | `tools_allow` | 提取工具名映射到 Aranea 工具 ID |
| 来源标记 | `kind: ecosystem_preset` | 通过 `WithKindOverride("ecosystem_preset")` |
| 行业归属 | `position_key` → taxonomy 节点 | 通过分类体系关联 |

#### 2.1.4 转换脚本

编写一次性转换脚本 `scripts/convert-agency-to-aranea.go`：

```
输入：docs/scenarios/agency-agents/{industry}/*.md
输出：internal/scenario/{industry}/agents.yaml + prompts/positions/

逻辑：
1. 扫描 agency-agents 目录下所有 .md 文件
2. 解析文件名 → 提取行业/岗位/variant
3. 解析 Markdown → 提取身份/规则/流程/交付物/工具
4. 映射到 taxonomy.yaml 中的 position_key
5. 生成 agents.yaml（Agent 定义）
6. 生成 prompts/positions/{position_key}/{variant}.md（System Prompt）
7. 生成 skills/{skill_key}/SKILL.md（提取可复用流程为 Skill）
```

#### 2.1.5 EcosystemPreset 扩展

现有 `EcosystemPresetUsecase` 已支持动态行业加载，只需：

1. 将新行业目录加入 `internal/scenario/`
2. 在 `categories.yaml` / `taxonomy.yaml` 中注册新行业
3. 前端 `IndustryMarketPage` 自动发现新行业（已支持动态渲染）

**无需修改后端代码**，这是 EcosystemPreset 架构的核心优势。

---

### 阶段二：Skill 提取 + Team 编排（中风险，中价值）

**目标**：从 Agency-Agents 的 Agent 定义中提取可复用 Skill，并构建跨 Agent 的 Team 编排。

#### 2.2.1 Skill 提取策略

Agency-Agents 的每个 Agent 包含"工作流程"和"交付物"部分，这些是天然的可复用 Skill：

| Agent | 可提取 Skill | Skill Key |
|-------|-------------|-----------|
| 小红书运营专家 | 种草笔记创作 | `xiaohongshu-content-creation` |
| 小红书运营专家 | 达人合作策略 | `xiaohongshu-kol-collaboration` |
| 抖音策略师 | 短视频策划 | `douyin-video-planning` |
| 抖音策略师 | 直播带货流程 | `douyin-livestream-commerce` |
| 赢单策略师 | MEDDPICC 资质审查 | `sales-meddpicc-qualification` |
| 安全工程师 | OWASP 代码审计 | `security-owasp-audit` |
| 前端开发者 | React 组件审查 | `frontend-react-review` |

**Skill 文件结构**：

```
internal/scenario/marketing/skills/
├── xiaohongshu-content-creation/
│   └── SKILL.md
├── xiaohongshu-kol-collaboration/
│   └── SKILL.md
├── douyin-video-planning/
│   └── SKILL.md
└── ...
```

**SKILL.md 格式**（遵循 trpc-agent-go Skill 规范）：

```markdown
---
name: xiaohongshu-content-creation
description: 小红书种草笔记创作技能，包含标题公式、正文结构、标签策略
---

# 小红书种草笔记创作

## 触发条件
当用户需要创作小红书种草笔记时激活

## 执行步骤
1. 分析产品定位和目标人群
2. 选择笔记类型（好物分享/教程/测评/合集）
3. 创作标题（使用数字+痛点+场景公式）
4. 撰写正文（痛点引入→解决方案→使用体验→行动号召）
5. 设计标签策略（3个核心标签+5个长尾标签）

## 交付物
- 完整种草笔记文案
- 标题备选方案（3个）
- 标签组合方案
```

#### 2.2.2 Team 编排

Agency-Agents 的 strategy 目录已提供 Phase 0-6 Playbook 和场景 Runbook，可映射为 Aranea Team 编排：

**示例：小红书品牌推广 Team**

```yaml
teams:
  - key: xiaohongshu-brand-launch
    display_name: 小红书品牌推广团队
    mode: coordinator
    description: 从品牌定位到内容投放的全链路小红书推广团队
    max_concurrency: 3
    timeout_seconds: 300
    loop_max_iterations: 5
    members:
      - agent_key: xiaohongshu-operator-general
        role: coordinator
        name: 小红书运营专家
        sort_order: 1
        task_prompt: 统筹品牌推广全流程，协调内容创作和达人合作
      - agent_key: content-creator-general
        role: worker
        name: 内容创作者
        sort_order: 2
        task_prompt: 根据运营策略创作种草笔记
      - agent_key: brand-guardian-general
        role: worker
        name: 品牌守护者
        sort_order: 3
        task_prompt: 审核内容是否符合品牌调性
      - agent_key: data-analyst-general
        role: synthesizer
        name: 数据分析师
        sort_order: 4
        task_prompt: 追踪投放数据，输出复盘报告
```

#### 2.2.3 行业间 Skill 共享

部分 Skill 可跨行业复用：

| Skill | 适用行业 |
|-------|---------|
| `content-creation` | 营销、自媒体、教育 |
| `data-analysis` | 金融、营销、供应链 |
| `compliance-review` | 金融、医疗、政务 |
| `technical-writing` | 软件开发、教育、法律 |
| `project-management` | 全行业通用 |

实现方式：将跨行业 Skill 放在 `internal/scenario/shared/skills/` 目录，Agent 通过 `skills` 字段引用。

---

### 阶段三：Ecosystem 安装流程打通 + M57 预留（高风险，高价值）

**目标**：将 EcosystemPreset 的"记录关系"升级为"实际部署"，并与 M57 Asset Schema 对齐。

#### 2.3.1 安装流程打通

当前 `EcosystemPreset.LoadEcosystemPreset()` 已调用 `seedPack` 完成实际导入，但 `Ecosystem.InstallProduct()` 仅记录关系。需要：

```
Ecosystem.InstallProduct()
  → 检查 Product 类型
  → 如果是 industry_preset：调用 EcosystemPreset.LoadEcosystemPreset()
  → 如果是 agent_template：调用 Pack Importer 导入单个 Agent
  → 如果是 skill_pack：调用 Skill Import 流程
  → 返回 InstallResult（包含实际创建的 Agent/Team/Skill 数量）
```

#### 2.3.2 IndustryMarketPage 安装向导完成

前端 `IndustryMarketPage` 的 `onInstall` 和 `onViewPrompts` 标注为 TODO，需要：

1. `onInstall` → 调用 `EcosystemPreset.Load` API → 加载行业数据 → 刷新页面
2. `onViewPrompts` → 打开 Agent Prompt 预览对话框
3. 安装向导 `IndustryWizard` → 选择行业 → 预览 Agent 列表 → 确认安装 → 进度反馈

#### 2.3.3 M57 Asset Schema 预留

在 Agent/Team/Skill 的 DB Schema 中，新增字段为 M57 做准备：

| 表 | 新增字段 | 用途 |
|----|---------|------|
| `agent` | `asset_id` | M57 Asset 全局 ID |
| `agent` | `asset_version` | M57 Asset 版本号 |
| `agent` | `asset_signature` | M57 Asset 签名 |
| `team` | 同上 | 同上 |
| `platform_skill` | 同上 | 同上 |

这些字段在阶段一/二不使用（留空），M57 上线后直接复用。

---

## 三、实施路径

### 3.1 工作量估算

| 阶段 | 任务 | 复杂度 |
|------|------|--------|
| **阶段一** | 扩展 taxonomy.yaml（9 个行业分类） | 低 |
| | 编写 Markdown→YAML 转换脚本 | 中 |
| | 执行转换，生成 9 个行业的 agents.yaml + prompt 文件 | 中 |
| | 验证 EcosystemPreset 加载新行业 | 低 |
| | 前端 IndustryMarketPage 展示新行业 | 低（已自动支持） |
| **阶段二** | 从 Agent 定义中提取 Skill | 中 |
| | 构建行业 Team 编排 | 中 |
| | 跨行业 Skill 共享机制 | 低 |
| **阶段三** | Ecosystem Install 流程打通 | 高 |
| | IndustryMarketPage 安装向导 | 中 |
| | M57 Asset Schema 预留字段 | 低 |

### 3.2 优先级排序

1. **P0**：阶段一 — 行业分类扩展 + Agent 模板入库（零代码改动，纯数据扩展）
2. **P1**：阶段二 — Skill 提取 + Team 编排（丰富生态内容）
3. **P2**：阶段三 — 安装流程打通（需要代码改动，与 M57 对齐）

### 3.3 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| Agency-Agents Markdown 格式不统一 | 转换脚本增加容错 + 人工抽检 |
| 新增行业 Agent 的 Prompt 质量参差 | 先入库，后续通过 Skill Evolution 自动优化 |
| taxonomy.yaml 膨胀影响加载性能 | 按需加载（EcosystemPreset 已支持） |
| 与 M57 Schema 冲突 | 预留字段 + M57 上线前做迁移 |

---

## 四、数据映射参考

### 4.1 Agency-Agents 行业 → Aranea 行业 Key 映射

| Agency-Agents 目录 | 行业名 | Aranea industry_key |
|-------------------|--------|-------------------|
| `engineering/` + `design/` + `product/` + `testing/` + `project-management/` | 互联网/软件 | `softwaredev`（已有） |
| `marketing/` + `paid-media/` | 营销广告 | `marketing`（新增） |
| `finance/` | 金融 | `finance`（已有） |
| `sales/` | 销售商务 | `sales`（新增） |
| `hr/` + `legal/` | 人力法务 | `hr_legal`（新增） |
| `supply-chain/` | 供应链制造 | `supply_chain`（新增） |
| `game-development/` | 游戏开发 | `game_dev`（新增） |
| `spatial-computing/` | 空间计算 | `spatial_computing`（新增） |
| `academic/` | 教育学术 | `education_academic`（新增） |
| `specialized/` + `support/` | 垂直行业专项 | `specialized_vertical`（新增） |
| `engineering/`（嵌入式方向） | 嵌入式工业 | `embedded_industrial`（新增） |

### 4.2 Agent Kind 映射

| Agency-Agents 来源 | Aranea Agent Kind | 说明 |
|-------------------|------------------|------|
| 英文版翻译 | `ecosystem_preset` | 通过 EcosystemPreset 加载 |
| 中国市场原创 | `ecosystem_preset` | 同上 |
| 用户自创建 | `user` | 不涉及 |
| M57 商城安装 | `marketplace` | 预留 |

### 4.3 工具映射

| Agency-Agents 工具 | Aranea 工具 ID | 说明 |
|-------------------|---------------|------|
| WebSearch | `web_search` | 已有 |
| WebFetch | `web_fetch` | 已有 |
| Read/Write/Edit | `workspace_exec` | 已有 |
| Shell/Bash | `shell` | 已有 |
| Playwright | `browser` | 已有 |
| Excel 处理 | `code_executor` | 已有 |
| 特定平台 API | `mcp_tool` | 需配置 MCP Server |

---

## 五、与 M57 Marketplace 的衔接

### 5.1 阶段一/二产出的数据如何迁移到 M57

M57 上线后，阶段一/二产出的行业数据可通过以下路径迁移：

```
internal/scenario/{industry}/
  → aranea CLI pack → .aranea 资产包
  → 签名 + 上传 → M57 Marketplace
  → 审核 + 上架 → 公网可发现
```

### 5.2 EcosystemPreset 与 M57 Installer 的关系

| 维度 | EcosystemPreset（当前） | M57 Installer（未来） |
|------|----------------------|---------------------|
| 数据来源 | 本地 YAML 文件 | 公网 .aranea 资产包 |
| 安装触发 | 用户在行业市场页点击加载 | 用户在商城点击安装 |
| 依赖解析 | 无（YAML 已包含完整定义） | 拓扑排序递归解析 |
| 签名校验 | 无 | 必须签名 + 权限声明 |
| 版本管理 | 无（覆盖式） | 增量更新 + 回滚 |
| 迁移路径 | EcosystemPreset 退化为 M57 的"官方第一方预置" | M57 成为主安装通道 |

### 5.3 建议

- 阶段一/二的数据格式设计应**向前兼容** M57 的 Asset Manifest
- Agent YAML 中的 `key`/`position_key`/`skills` 字段可直接映射到 M57 Asset 的 `asset_id`/`category`/`dependencies`
- Prompt 文件和 Skill 文件已是 M57 资产包的标准组成部分

---

## 六、总结

| 阶段 | 核心动作 | 产出 | 代码改动 |
|------|---------|------|---------|
| **一** | 转换 Agency-Agents 215 个 Agent 为 Aranea YAML + Prompt 格式 | 9 个新行业、128 个新 Agent、100+ Prompt 文件 | **零**（纯数据） |
| **二** | 提取可复用 Skill + 构建 Team 编排 | 50+ 行业 Skill、20+ Team 编排 | 少量（Skill 加载路径） |
| **三** | 打通安装流程 + M57 预留 | 完整的行业市场体验 | 中等（Install 流程 + Schema） |

**推荐路径**：先执行阶段一（纯数据扩展，零风险），验证行业市场页面展示效果后，再推进阶段二和三。
