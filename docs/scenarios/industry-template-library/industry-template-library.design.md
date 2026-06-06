# 行业模板库（Industry Template Library）— 设计文档

> **文档版本**：v1.0（2026-05-30）
> **定位**：为 Aranea-Agents 多智能体编排平台设计内置行业模板库体系
> **参考场景**：[daily_stock_analysis](../scenarios/daily_stock_analysis/daily-stock-analysis.design.md)（金融-股票分析子领域）
> **状态**：✅ 设计已批准，待进入实施规划

---

## 0. 设计目标与范围

### 0.1 一句话定义

> **行业模板库** 是 Aranea-Agents 平台的内置「行业→部门→岗位→Agent」四层分类体系。每个行业是一个**可安装的完整场景包**（Scenario Package），包含该行业专属的岗位级 Agent 定义、Team 编排模板、领域 Tool/Skill/Knowledge。用户可在 Web UI 中浏览行业市场、一键安装、即刻使用。

### 0.2 首期覆盖行业

| # | 行业 | Key | 核心方向 | 部门数 | 岗位数 | Agent 总数 |
|---|------|-----|---------|--------|--------|-----------|
| 1 | **软件开发** | `softwaredev` | 系统开发 / 游戏(UE) / 全栈 / DevOps | 10 | ~45+ | ~90-110 |
| 2 | **自媒体 / 内容创作** | `selfmedia` | 网文小说 / 短视频 / 图文 / 直播 | 5 | ~25+ | ~50-60 |
| 3 | **金融 / 投资** | `finance` | 证券研究 / 量化交易 / 固收 / 财富管理 | 6 | ~30+ | ~55-65 |
| | **合计** | — | — | **21** | **~100+** | **~195-235** |

### 0.3 设计原则

| # | 原则 | 体现 |
|---|------|------|
| 1 | **场景安装包模式** | 每个行业 = `internal/scenario/<key>/` + install.go + YAML 配置，复用 stockx 已验证模式 |
| 2 | **岗位级 Agent 粒度** | 不用泛称（如"后端开发"），用精确岗位名（如"Golang 高级工程师"） |
| 3 | **一岗多 Agent** | 同一 position_key 下通过 agent_variant 区分不同职责方向的 Agent |
| 4 | **Prompt 深度注入** | 行业描述 → 部门职责 → 岗位职责 → Agent variant 定位，逐级注入 system_prompt |
| 5 | **不修改平台核心** | 新增 industries/departments/positions 表 + agents 表扩展字段；通过标准接口注册 |
| 6 | **可关闭可卸载** | 每个行业有独立开关；卸载时清理关联资源 |

---

## 1. 四层分类体系

### 1.1 数据模型

在平台 SQLite（Ent ORM）中新增三张表，扩展 agents 表：

```
┌──────────────────────────────────────────────────────────┐
│  industries（行业表）                                      │
│  ┌──────┬────────────┬────────┬───────────┬───────────┐  │
│  │ key  │ name       │ icon   │ description│ scenario_key│  │
│  │ PK   │            │        │ text      │ FK→scenarios│  │
│  ├──────┼────────────┼────────┼───────────┼───────────┤  │
│  │ soft │ 软件开发    │ 💻     │ 覆盖系统...│ softwaredev│  │
│  │ self │ 自媒体      │ 🎬     │ 覆盖网文...│ selfmedia  │  │
│  │ fin  │ 金融投资    │ 📈     │ 覆盖证券...│ finance    │  │
│  └──────┴────────────┴────────┴───────────┴───────────┘  │
├──────────────────────────────────────────────────────────┤
│  departments（部门表）                                    │
│  ┌──────┬───────────┬────────────┬──────────────────────┐│
│  │ key  │ name      │ industry_key│ description          ││
│  │ PK   │           │ FK→industries│ responsibilities_json││
│  ├──────┼───────────┼────────────┼──────────────────────┤│
│  │ back │ 后端研发部 │ soft       │ 负责服务端...         ││
│  │ front│ 前端研发部 │ soft       │ 负责 Web UI...       ││
│  │ game │ 游戏开发部 │ soft       │ UE/Unity 客户端...   ││
│  │ fict │ 小说创作部 │ self       │ 网文策划与创作...     ││
│  │ video│ 视频制作部 │ self       │ 短视频全流程...       ││
│  │ equity│ 证券研究部│ fin        │ ★ 复用 stockx 能力   ││
│  │ quant│ 量化交易部 │ fin        │ 策略研究与实盘...     ││
│  └──────┴───────────┴────────────┴──────────────────────┘│
├──────────────────────────────────────────────────────────┤
│  positions（岗位表）                                       │
│  ┌────────┬──────────────────┬──────────────┬───────────┐│
│  │ key    │ name             │ department_key│ desc      ││
│  │ PK     │                  │ FK→departments│ resp_json ││
│  ├────────┼──────────────────┼──────────────┼───────────┤│
│  │go_sen  │ Golang 高级工程师 │ back         │ {...}     ││
│  │vue_sen │ Vue3 高级前端    │ front        │ {...}     ││
│  │ue_clt  │ UE 客户端程序    │ game         │ {...}     ││
│  │nov_auth│ 网文小说作者     │ fict         │ {...}     ││
│  │tech_anl│ 技术分析师       │ equity       │ {...}     ││
│  │quant_r │ 量化研究员       │ equity       │ {...}     ││
│  └────────┴──────────────────┴──────────────┴───────────┘│
├──────────────────────────────────────────────────────────┤
│  agents 表（现有，新增字段）                                │
│  ┌─────────────────┬────────────────┬──────────────────┐│
│  │ position_key     │ agent_variant  │ variant_desc     ││
│  │ FK→positions     │ string         │ text             ││
│  ├─────────────────┼────────────────┼──────────────────┤│
│  │ go_sen          │ general        │ 日常开发          ││
│  │ go_sen          │ code_review    │ 代码审查          ││
│  │ go_sen          │ architect      │ 架构设计          ││
│  │ nov_auth        │ drafting       │ 正文起草          ││
│  │ nov_auth        │ polishing      │ 润色修饰          ││
│  │ nov_auth        │ data_driven    │ 数据驱动写作      ││
│  │ quant_r         │ factor         │ 因子研究          ││
│  │ quant_r         │ backtest       │ 回测执行          ││
│  │ quant_r         │ portfolio      │ 组合构建          ││
│  │ quant_r         │ ml_alpha       │ ML alpha 研究     ││
│  └─────────────────┴────────────────┴──────────────────┘│
```

### 1.2 Ent Schema 定义要点

```go
// internal/data/ent/schema/industry.go
type Industry struct {
    ent.Schema
}

func (Industry) Fields() []ent.Field {
    return []ent.Field{
        field.String("key").Unique().Immutable(),
        field.String("name"),
        field.String("icon").Optional(),
        field.Text("description").Optional(),
        field.String("scenario_key").Optional(),
        field.Bool("enabled").Default(true),
        field.Int("sort_order").Default(0),
    }
}

func (Industry) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("departments", Department.Type),
    }
}
```

```go
// internal/data/ent/schema/position.go
type Position struct {
    ent.Schema
}

func (Position) Fields() []ent.Field {
    return []ent.Field{
        field.String("key").Unique().Immutable(),
        field.String("name"),
        field.Text("description"),
        field.JSON("responsibilities", map[string]string{}).Optional(),
        field.Strings("skills_required").Optional(),
        field.String("seniority_level").Optional(),
        field.Int("sort_order").Default(0),
    }
}

func (Position) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("department", Department.Type).Ref("positions").Unique(),
        edge.To("agents", Agent.Type), // 一个岗位对应多个 Agent
    }
}
```

### 1.3 agents 表扩展

```sql
-- 在现有 agents 表上新增字段
ALTER TABLE agents ADD COLUMN position_key TEXT REFERENCES positions(key);
ALTER TABLE agents ADD COLUMN agent_variant TEXT NOT NULL DEFAULT 'general';
ALTER TABLE agents ADD COLUMN variant_description TEXT;

CREATE UNIQUE INDEX idx_agent_position_variant ON agents(position_key, agent_variant);
```

### 1.4 Prompt 注入链路

当 Agent 被加载时，system_prompt 按以下顺序拼接：

```markdown
{prompt_file_content}                    ← prompts/positions/{pos}/{variant}.md 的主体
                                           （已包含角色定义、专业领域、工作原则、输出约定）

## 组织上下文
{industry.description}                   ← 来自 industries 表
{department.description}                 ← 来自 departments 表
{department.responsibilities_json}       ← 部门职责详情

## 岗位职责
{position.name} - {position.description}
{position.responsibilities_json}         ← 岗位详细职责 JSON

## 当前定位
你是本岗位的 {agent_variant} 方向专家。
{variant_description}                    ← 来自 agents.variant_description
```

---

## 2. 场景包目录结构规范

### 2.1 通用结构

```
internal/scenario/<industry_key>/
├── install.go                          # 幂等安装入口（复用 stockx 模式）
├── industry.yaml                       # 行业元信息 + taxonomy 全定义
├── agents.yaml                         # 全部 Agent 注册定义
├── teams.yaml                          # 预置 Team 编排模板
├── crons.yaml                          # 定时任务（可选）
├── graphs/                             # Graph 工作流定义（可选）
│   ├── <graph_name>.graph.json
│   └── handlers/
│       └── <handler_name>.go
├── prompts/                            # ★ System Prompt 文件（按岗位·变体组织）
│   ├── positions/
│   │   ├── <position_key>/
│   │   │   ├── <variant>.md           # 核心：该变体的 system_prompt
│   │   │   └── <variant>.schema.json  # Output Schema（JSON Schema）
│   │   └── ...
│   └── team/                           # Team 协调者 prompt
│       └── <team_key>.md
├── schemas/                            # 共享 Output Schema
│   └── *.schema.json
├── skills/                             # 行业专属 Skill 包
│   └── <skill_key>/
│       └── SKILL.md
└── README.md                           # 行业包说明
```

### 2.2 industry.yaml 格式

```yaml
industry:
  key: softwaredev
  name: 软件开发
  icon: 💻
  description: >
    覆盖系统软件、Web 应用、移动 App、游戏（含 UE 引擎）的全栈软件开发行业。
    从需求分析、架构设计、编码实现、质量保障到运维部署的完整软件生命周期。
  scenario_key: softwaredev
  version: "1.0"
  author: "Aranea Team"

departments:
  - key: backend
    name: 后端研发部
    description: 负责服务端架构设计与核心业务逻辑实现
    responsibilities:
      lead: "主导技术方案评审与架构决策"
      develop: "高质量编码、Code Review、性能优化"
      maintain: "线上问题排查、系统稳定性保障"
    sort_order: 1
    positions:
      - key: go_senior_engineer
        name: Golang 高级工程师
        description: >
          负责高并发微服务后端的架构设计与核心模块开发。
          精通 Go 语言特性、Kratos/gRPC/Etcd/Kafka/Redis/PostgreSQL。
        seniority: "P6-P7"
        skills_required:
          - "Go 语言精通（goroutine/channel/interface/泛型/GC）"
          - "微服务架构（gRPC/Kratos/Etcd/服务发现）"
          - "存储引擎（PostgreSQL/Redis/Kafka）"
          - "工程实践（Clean Architecture/DDD/TDD）"
        agents:
          - variant: general
            key: agent_go_senior_dev
            display_name: Go 高级开发
            model_preference: "strong"
            tools_allow: [git, codeexecutor_go, web_search, filesystem]
            skills_allow: [softwaredev-go-best-practices, softwaredev-clean-arch]
            prompt_file: prompts/positions/go_senior_engineer/general.md
            output_schema: schemas/go_dev_output.schema.json
          - variant: code_review
            key: agent_go_code_reviewer
            display_name: Go 代码审查员
            model_preference: "medium"
            tools_allow: [git_diff, linter_result, static_analysis]
            skills_allow: [softwaredev-code-review-checklist, softwaredev-security-audit]
            prompt_file: prompts/positions/go_senior_engineer/code_review.md
            output_schema: schemas/go_review_output.schema.json
          - variant: architect
            key: agent_go_architect
            display_name: Go 架构师
            model_preference: "strong"
            tools_allow: [mermaid_render, doc_generator, web_search]
            skills_allow: [softwaredev-ddd-tactical, softwaredev-patterns-catalog]
            prompt_file: prompts/positions/go_senior_engineer/architect.md
            output_schema: schemas/go_arch_output.schema.json

      - key: java_senior_engineer
        name: Java 高级工程师
        # ... 同理展开
```

---

## 3. 行业一：软件开发（softwaredev）详细定义

### 3.1 行业概览

| 维度 | 内容 |
|------|------|
| **Key** | `softwaredev` |
| **名称** | 软件开发 |
| **Icon** | 💻 |
| **定位** | 覆盖系统软件、Web 应用、移动 App、游戏（含 UE5）的全栈开发生命周期 |
| **预置 Skill 包** | ~15 个（Go 最佳实践、Clean Architecture、DDD 战术、代码审查清单、安全审计、UE GAS、CI/CD 等） |
| **预置 Tool 扩展** | git 操作、代码分析（lint/static analysis）、CI 触发、Docker 管理 |
| **预置 Team 模板** | ~8-10 个 |

### 3.2 部门清单（10 个）

#### A. 后端研发部 (`backend`)

| # | Position Key | 岗位名称 | 职级 | Variant 数 | Agent Keys |
|---|-------------|---------|------|-----------|------------|
| A1 | `go_senior_engineer` | Golang 高级工程师 | P6-P7 | 3 | general / code_review / architect |
| A2 | `java_senior_engineer` | Java 高级工程师 | P6-P7 | 3 | general / code_review / architect |
| A3 | `python_senior_engineer` | Python 高级工程师 | P6-P7 | 2 | general / data_pipeline |
| A4 | `rust_engineer` | Rust 工程师 | P5-P7 | 2 | general / systems_programming |
| A5 | `cpp_backend_engineer` | C++ 后端工程师 | P5-P7 | 2 | general / performance |
| A6 | `database_administrator` | 数据库管理员 DBA | P5-P7 | 2 | tuning / reliability |
| A7 | `backend_intern` | 后端开发实习生 | P3-P4 | 1 | learning |

**A1 岗位详细职责**：

```json
{
  "core_responsibilities": [
    "负责高并发微服务后端的架构设计与核心模块开发",
    "精通 Go 语言特性（goroutine/channel/interface/泛型）、GC 调优、性能剖析",
    "熟练使用 Kratos/gRPC/Etcd/Kafka/Redis/PostgreSQL 等技术栈",
    "编写高质量、可测试、符合 Go 惯用法的代码；遵循 Clean Architecture / DDD 分层",
    "参与 Code Review，保障代码规范与系统稳定性",
    "排查线上问题（panic/死锁/内存泄漏/Goroutine 泄漏）"
  ],
  "required_competencies": [
    "Go 1.22+ 泛型、iter、slices/maps/cmp 新标准库",
    "goroutine 调度模型（GMP）、channel 模式、context 传播取消",
    "Kratos v2 transport/middleware/wire DI、protobuf 向后兼容策略",
    "PostgreSQL 事务隔离、索引优化、连接池；Redis 缓存策略、分布式锁",
    "Kafka 消费者组、重试、死信队列",
    "Clean Architecture（Entity/UseCase/Interface Adapter/Framework）",
    "DDD 战术设计（聚合根、值对象、领域事件）"
  ],
  "work_principles": [
    "接口先行：先定义接口契约（proto + Go interface），再实现",
    "错误透明：包装错误，禁止 fmt.Errorf 裸返回",
    "零容忍 panic：生产代码必须 recover 或保证不触发",
    "可观测性：关键路径埋点 trace + metric + structured log",
    "并发安全：共享状态必须 sync.Mutex/RWMutex 或原子操作"
  ],
  "output_standards": {
    "code_style": "遵循项目现有命名风格和目录结构",
    "documentation": "每个 public 函数必须有 godoc 注释",
    "error_handling": "错误处理必须显式，不允许 _ 吞掉错误",
    "deliverable": "包含：设计思路 → 代码实现 → 测试用例 → 风险说明"
  }
}
```

**A1 Prompt 文件示例**（`prompts/positions/go_senior_engineer/general.md`）：

```markdown
## 你是谁
你是一位拥有 8 年经验的 **Golang 高级工程师**，隶属于「后端研发部」。

## 专业领域
- **语言精通**：Go 1.22+（泛型、iter、slices/maps/cmp 新标准库）、goroutine 调度模型、
  channel 模式、error wrapping/is/as 链、context 传播与取消
- **框架深度**：Kratos v2（transport/middleware/wire DI）、gRPC streaming、
  protobuf 向后兼容策略、Etcd 服务发现与 Watch
- **存储**：PostgreSQL（事务隔离、索引优化、连接池）、Redis（缓存策略、
  分布式锁、Pipeline）、Kafka（消费者组、重试、死信队列）
- **工程实践**：Clean Architecture（Entity/UseCase/Interface Adapter/Framework），
  DDD 战术设计（聚合根、值对象、领域事件），TDD/BDD

## 工作原则
1. **接口先行**：先定义接口契约（proto + Go interface），再实现
2. **错误透明**：用 kerrors 包装错误，禁止 fmt.Errorf 裸返回
3. **零容忍 panic**：生产代码必须 recover 或保证不触发
4. **可观测性**：关键路径埋点 trace + metric + structured log
5. **并发安全**：共享状态必须 sync.Mutex/RWMutex 或原子操作

## 输出约定
- 代码遵循项目现有命名风格和目录结构
- 每个 public 函数必须有 godoc 注释
- 错误处理必须显式，不允许 `_` 吞掉错误
- 提交的方案包含：设计思路 → 代码实现 → 测试用例 → 风险说明
```

#### B. 前端研发部 (`frontend`)

| # | Position Key | 岗位名称 | Variant 数 | Agent Variants |
|---|-------------|----------|-----------|----------------|
| B1 | `vue3_senior_engineer` | Vue 3 高级前端工程师 | 3 | general / code_review / ux_auditor |
| B2 | `react_senior_engineer` | React 高级前端工程师 | 3 | general / code_review / ux_auditor |
| B3 | `typescript_specialist` | TypeScript 技术专家 | 2 | type_system / migration |
| B4 | `frontend_performance_engineer` | 前端性能优化工程师 | 2 | optimization / audit |
| B5 | `ui_ux_implementer` | UI/UX 还原工程师 | 1 | implementation |

#### C. 游戏开发部 (`gamedev`) — ★ 特色部门

| # | Position Key | 岗位名称 | 子方向 | Variant 数 | Agent Variants |
|---|-------------|---------|--------|-----------|----------------|
| C1 | `ue_client_programmer` | UE 客户端程序 | Unreal Engine 5 | 4 | general / gameplay / performance / network |
| C2 | `ue_gameplay_programmer` | UE 游戏逻辑程序 | Gameplay Framework | 2 | ability_system / combat_logic |
| C3 | `ue_graphics_programmer` | UE 图形渲染程序 | Rendering Pipeline | 2 | material / optimization |
| C4 | `game_server_engineer` | 游戏服务端工程师 | Go/C++/Java | 2 | general / real_time_sync |
| C5 | `game_technical_artist` | 技术 TA | 美术-程序桥梁 | 2 | pipeline / shader |
| C6 | `game_planner_designer` | 系统策划 | 数值/系统/关卡 | 2 | system_design / balance |

**C1 岗位详细职责**：

```json
{
  "core_responsibilities": [
    "基于 Unreal Engine 5 进行客户端功能开发（C++ + Blueprint 协作）",
    "精通 UEFN（Unreal Engine Framework）：GameFramework、Actor/Component 模型",
    "Gameplay Ability System (GAS) 的集成与定制",
    "网络 replication（属性复制、RPC、Role 权限）、多线程渲染",
    "性能优化：Draw Call 优化、GPU Profile、Stat Commands、Unreal Insights",
    "平台适配（PC/Console/Mobile 的差异化处理）"
  ],
  "required_competencies": [
    "UE5 GameFramework（AGameBase/UGameInstance/APlayerController/ACharacter）",
    "Actor Component 组合模式 vs 继承的选择判断",
    "GAS（AttributeSet/AbilitySystemComponent/GameplayAbility/GameplayEffect）",
    "Replication（RepNotify/OnRep、RPC Server/Client/Multicast、Role Authority）",
    "Render Thread / Game Thread 并发模型",
    "Unreal Insights / Unreal Frontend / Commandlet 自动化"
  ]
}
```

#### D～J 部门概览

| 部门 | Key | 核心岗位 | 约 Agent 数 |
|------|-----|---------|-------------|
| **D. 移动端研发部** | `mobiledev` | Flutter 高级(3)、iOS 原生(2)、Android 原生(2)、跨平台架构师(2) | ~12 |
| **E. DevOps/基础设施** | `devops` | CI/CD 工程(2)、K8s 工程(2)、云基础设施(2)、SRE(2) | ~10 |
| **F. 架构与设计** | `architecture` | 系统架构师(3)、技术委员会评审(2)、领域建模专家(2) | ~8 |
| **G. 质量保障** | `qa` | 自动化测试(2)、测试开发 SDET(2)、性能测试(1)、安全测试(1) | ~6 |
| **H. 数据工程** | `dataeng` | 数据管道(2)、BI 分析(1)、数据平台(2) | ~5 |
| **I. 安全** | `security` | 应用安全(2)、安全审计(1)、渗透测试(1) | ~4 |
| **J. 产品与项目管理** | `productpm` | 产品经理(2)、Scrum Master(1)、技术文档(1) | ~4 |

### 3.3 预置 Team 模板（软件开发）

| Team Key | 名称 | 模式 | 成员组合 | 典型用途 |
|----------|------|------|---------|---------|
| `team_fullstack_feature` | 全栈功能开发团队 | coordinator | PM + 架构师 + Go/Vue/各1 + QA | 从需求到上线的完整功能交付 |
| `team_code_review_bureau` | 代码审查局 | parallel | Go/Java/TS 审查员 × N + 安全扫描 | MR/PR 的自动化深度审查 |
| `team_ci_cd_pipeline` | CI/CD 流水线 | sequential | DevOps + SRE + 安全员 | 构建→测试→部署→验证 |
| `team_incident_response` | 故障响应小组 | coordinator | SRE + 后端(对应语言) + DBA + 监控 | 线上故障排查与恢复 |
| `team_game_content` | 游戏内容生产 | graph | 策划 + TA + UE客户端 + UE服务端 + 音效 | 游戏功能从设计到实现 |
| `team_security_audit` | 安全审计 | parallel + synthesizer | 应用安全 + 渗透测试 + 合规检查 | 发布前安全评估 |
| `team_architecture_review` | 架构评审会 | coordinator | 架构师 × N + 各域 Tech Lead | 技术方案 RFC 评审 |
| `team_onboarding_mentor` | 新人导师团 | coordinator | 导师(对应岗位) + 实习生 + 文档工程师 | 新员工入职引导 |

---

## 4. 行业二：自媒体 / 内容创作（selfmedia）详细定义

### 4.1 行业概览

| 维度 | 内容 |
|------|------|
| **Key** | `selfmedia` |
| **名称** | 自媒体 / 内容创作 |
| **Icon** | 🎬 |
| **定位** | 覆盖网文小说创作、短视频制作、图文内容、直播运营、多平台分发的全链路内容创作流水线 |
| **预置 Skill 包** | ~12 个（小说节奏学、爆款拆解、平台算法适配、剪辑规范、SEO 等） |
| **预置 Tool 扩展** | 字数统计、风格检测、平台数据分析、封面生成、多平台分发 API |
| **预置 Team 模板** | ~6 个 |

### 4.2 部门清单（5 个）

#### A. 小说创作部 (`fiction_writing`) — ★ Prompt 最精细的部门

| # | Position Key | 岗位名称 | Variant 数 | Agent Variants |
|---|-------------|----------|-----------|----------------|
| A1 | `webnovel_author` | 网文小说作者（主力） | 4 | drafting / polishing / data_driven / ghostwriting |
| A2 | `webnovel_plotter` | 剧情策划 / 大纲设计师 | 2 | outline / pacing |
| A3 | `worldbuilding_designer` | 世界观 / 设定设计师 | 2 | magic_system / geography_history |
| A4 | `character_writer` | 角色塑造专家 | 2 | creation / consistency |
| A5 | `fiction_editor` | 责任编辑 | 2 | review / compliance |
| A6 | `fan_engagement_ops` | 读者互动运营 | 1 | engagement |

**A1 岗位详细职责**：

```json
{
  "core_responsibilities": [
    "负责网文小说的正文创作，单章 2000-3000 字，日更 6000-10000 字",
    "熟悉主流平台调性：起点（男频玄幻/仙侠/都市）、番茄（新媒体风/脑洞）、晋江（女频）",
    "掌握「黄金三章」开篇法则、爽点节奏设计、悬念钩子（章节尾钩子）",
    "维护角色一致性（人设卡 + 行为准则），避免 OOC（角色崩坏）",
    "战斗场景描写、情感戏推进、对话自然度",
    "能根据数据反馈（追读率、章均点推比）调整写作策略"
  ],
  "writing_disciplines": {
    "chapter_structure": {
      "total_words": "2000-3000 字/章",
      "opening_hook": "100-200 字承上启下 + 制造悬念",
      "body": "1500-2000 字剧情/战斗/对话推进",
      "ending_hook": "100-200 字反转/悬念/新冲突/情绪高点"
    },
    "prohibitions": [
      "不水字数（禁止大段环境描写凑字、重复内心独白）",
      "不 OOC（角色行为必须符合已确立的人设卡）",
      "不剧透未来超过 3 章的内容",
      "不使用过于生僻的词汇（保持通俗性）",
      "不在正文中插入作者吐槽"
    ],
    "dialogue_rules": [
      "每句对话必须带动作/神态/心理描写的「对话标签」",
      "不同角色的说话方式必须有辨识度（口头禅、句式习惯、称谓差异）",
      "避免「他说」「她道」的机械交替"
    ]
  },
  "output_format": {
    "chapter_summary": "50 字内章节简介",
    "body_markdown": "Markdown 格式正文（含对话/描写/叙述）",
    "foreshadowing_tags": "[伏笔:xxx] 标注",
    "emotion_curve": "本章情感高低点标记"
  }
}
```

**A1 Prompt 文件示例**（`prompts/positions/webnovel_author/drafting.md`）：

```markdown
## 你是谁
你是一位 **全勤级别网文小说作者**，日更万字，累计创作超 500 万字。
你隶属于「小说创作部」，当前正在创作一部 {genre} 类型的作品。

## 平台与风格适配
- 当前发布平台：{platform}（起點中文网 / 番茄小说 / 晋江文学城）
- 该平台读者偏好：{platform_reader_profile}
- 作品标签：{tags}
- 参考对标作品：{reference_works}

## 创作铁律

### 1. 章节结构（每章 ~2500 字）
```
┌─────────────────────────────┐
│ 开头钩子（100-200字）        │ ← 承上启下 + 制造悬念
│ ─────────────────────────── │
│ 主体推进（1500-2000字）      │ ← 剧情/战斗/对话 三选一或组合
│ ─────────────────────────── │
│ 尾巴钩子（100-200字）        │ ← 反转/悬念/新冲突/情绪高点
└─────────────────────────────┘
```

### 2. 禁止事项
- ❌ 不水字数（禁止大段环境描写凑字、重复内心独白）
- ❌ 不 OOC（角色行为必须符合已确立的人设卡）
- ❌ 不剧透未来超过 3 章的内容
- ❌ 不使用过于生僻的词汇（保持通俗性）
- ❌ 不在正文中插入作者吐槽（用旁白或角色口吻表达）

### 3. 输出格式
每章输出必须包含：
1. **本章概要**（50 字内，用于章节简介）
2. **正文**（Markdown 格式，含对话/描写/叙述）
3. **伏笔标注**（如有埋设，标注 [伏笔:xxx]）
4. **情感曲线标记**（本章情感高低点）

### 4. 对话规范
- 每句对话必须带动作/神态/心理描写的「对话标签」
- 不同角色的说话方式必须有辨识度（口头禅、句式习惯、称谓差异）
- 避免「他说」「她道」的机械交替

## 你的工具
- `outline_reader`：读取当前章节对应的大纲节点
- `character_card`：查询出场角色的人设卡（性格/口头禅/关系/状态）
- `world_bible`：查询世界观设定（力量体系/地名/组织）
- `continuity_checker`：检查与前文的连续性（时间线/人物位置/物品归属）
```

#### B. 视频制作部 (`video_production`)

| # | Position Key | 岗位名称 | Variant 数 | Agent Variants |
|---|-------------|----------|-----------|----------------|
| B1 | `short_video_director` | 短视频导演 / 编导 | 3 | planning / storyboard / review |
| B2 | `video_scriptwriter` | 视频脚本编剧 | 2 | scriptwriting / platform_adapt |
| B3 | `video_editor_premiere` | 视频剪辑师（PR 达人） | 2 | editing / effects |
| B4 | `video_editor_capcut` | 剪映专业剪辑师 | 2 | editing / template |
| B5 | `motion_graphics_artist` | 动效 / 包装设计师 | 2 | motion / branding |
| B6 | `sound_designer` | 音效 / 配乐设计师 | 1 | design |
| B7 | `thumbnail_designer` | 封面图设计师 | 1 | design |

#### C～E 部门概览

| 部门 | Key | 核心岗位 | 约 Agent 数 |
|------|-----|---------|-------------|
| **C. 图文内容部** | `content_graphic` | 公众号运营(2)、小红书种草(2)、知识付费撰稿(1) | ~6 |
| **D. 直播运营部** | `live_streaming` | 主播/场控(2)、直播脚本(1)、直播数据(1) | ~5 |
| **E. 多平台分发与运营** | `distribution` | 多平台运营(2)、SEO(1)、粉丝增长(1)、变现(1) | ~5 |

### 4.3 预置 Team 模板（自媒体）

| Team Key | 名称 | 模式 | 成员组合 | 典型用途 |
|----------|------|------|---------|---------|
| `team_novel_daily_production` | 小说日更生产线 | graph | 作者(drafting) → 润色(polish) → 编辑(review) → 分发 | 日更 6000-10000 字流水线 |
| `team_viral_video_factory` | 爆款视频工厂 | coordinator | 导演 + 编剧 + 剪辑(PR/剪映) + 封面 + 分发 | 从选题到发布的短视频全流程 |
| `team_multi_platform_sync` | 多平台同步分发 | parallel | 各平台运营 Agent + 格式适配 | 一键分发到抖音/B站/小红书/视频号 |
| `team_live_stream_crew` | 直播工作组 | sequential | 主播 + 场控 + 脚本 + 数据复盘 | 直播全流程支持 |
| `team_content_calendar` | 内容日历规划 | coordinator | 各内容方向策划 + 数据分析师 + 排期 | 月度内容规划与排期 |
| `team_fan_community` | 粉丝社群运营 | parallel | 读者互动 + 评论管理 + 粉丝活动 | 社群活跃度提升 |

---

## 5. 行业三：金融 / 投资（finance）详细定义

### 5.1 行业概览

| 维度 | 内容 |
|------|------|
| **Key** | `finance` |
| **名称** | 金融 / 投资 |
| **Icon** | 📈 |
| **定位** | 覆盖证券研究、量化交易、固收衍生品、合规风控、财富管理的全链条金融行业；**证券研究子领域直接复用 stockx 场景的全部能力** |
| **与 stockx 关系** | finance.equity_research 部门的 Agent/Team/Tool/Skill 复用 stockx 场景；finance 在此基础上扩展到更广范畴 |
| **预置 Skill 包** | ~18 个（含 stockx 的 13 个 + 量化策略、风控模型、合规检查等） |
| **预置 Tool 扩展** | 含 stockx 的 27 个 + 量化回测、组合优化、合规筛查等 |
| **预置 Team 模板** | ~8 个（含 stockx 的 6 个） |

### 5.2 部门清单（6 个）

#### A. 证券研究部 (`equity_research`) — ★ 复用 stockx

本部门的全部 Agent 和 Tool 直接引用 stockx 场景，并在其基础上做 **agent_variant 扩展**：

| # | Position Key | 岗位名称 | 来源 | Variant 数 | Agent Variants |
|---|-------------|---------|------|-----------|----------------|
| A1 | `technical_analyst` | 技术分析师 | stockx | 1 | general（沿用 agent_technical_analyst） |
| A2 | `fundamental_analyst` | 基本面分析师 | stockx | 1 | general |
| A3 | `money_flow_analyst` | 资金面分析师 | stockx | 1 | general |
| A4 | `news_analyst` | 消息面分析师 | stockx | 1 | general |
| A5 | `sentiment_analyst` | 情绪面分析师 | stockx | 1 | general |
| A6 | `industry_analyst` | 行业分析师 | stockx | 1 | general |
| A7 | `risk_assessor` | 风险评估师 | stockx | 1 | general |
| A8 | `quant_researcher` | 量化研究员 | stockx 扩展 | **4** | factor / backtest / portfolio / ml_alpha |
| A9 | `data_collector` | 数据采集员 | stockx | 1 | general |
| A10 | `report_writer` | 报告撰写员 | stockx | 1 | general |
| A11 | `trading_coordinator` | 交易主控/调度员 | stockx 扩展 | **2** | premarket / intray |

**A8 岗位（stockx `agent_quant_factor` 的一岗多 Agent 化）详细职责**：

```json
{
  "core_responsibilities": [
    "设计与验证量化交易策略（因子选股、统计套利、事件驱动、机器学习 alpha）",
    "因子挖掘：价量因子、基本面因子、另类数据因子（舆情/卫星/供应链）",
    "回测框架使用：Backtrader/Zipline/VnPy，处理前视偏差、存活者偏差",
    "风险模型：Barra 风格因子、波动率建模、相关性矩阵、VaR/CVaR",
    "组合优化：均值-方差、Black-Litterman、风险平价、最大分散化",
    "执行算法：TWAP/VWAP/POV/Implementation Shortfall"
  ],
  "variant_details": {
    "factor": {
      "focus": "因子研究：IC/IR 分析、因子正交、中性化、因子组合",
      "tools": ["factor_compute", "codeexecutor(Python/pandas)", "ic_ir_calculator"],
      "model": "medium"
    },
    "backtest": {
      "focus": "回测执行：参数扫描、样本外验证、过拟合检测、夏普比率优化",
      "tools": ["backtest_engine", "walk_forward_validator", "overfitting_detector"],
      "model": "medium"
    },
    "portfolio": {
      "focus": "组合构建：优化求解、仓位管理、再平衡、冲击成本",
      "tools": ["portfolio_optimizer", "risk_model", "impact_cost_model"],
      "model": "strong"
    },
    "ml_alpha": {
      "focus": "机器学习 alpha：特征工程、模型训练（XGBoost/LightGBM）、集成方法",
      "tools": ["codeexecutor(sklearn/xgboost/lightgbm)", "feature_store", "ml_backtester"],
      "model": "strong"
    }
  }
}
```

#### B. 量化交易部 (`quant_trading`) — ★ 新增部门

| # | Position Key | 岗位名称 | Variant 数 | Agent Variants |
|---|-------------|----------|-----------|----------------|
| B1 | `quant_developer` | 量化开发工程师 | 3 | research_platform / data_pipeline / trading_system |
| B2 | `algo_trading_engineer` | 算法交易工程师 | 2 | execution_algo / market_making |
| B3 | `low_latency_engineer` | 低延迟系统工程师 | 2 | kernel_tuning / network_opt |
| B4 | `quant_devops` | 量化运维工程师 | 1 | operations |

#### C～F 部门概览

| 部门 | Key | 核心岗位 | 约 Agent 数 | 备注 |
|------|-----|---------|-------------|------|
| **C. 固收与衍生品** | `fixed_income` | 固收分析(2)、衍生品定价(2)、信用评级(1) | ~5 | 债券/期权/期货/互换 |
| **D. 合规与风控** | `compliance_risk` | 合规专员(1)、风控经理(2)、反洗钱(1) | ~4 | 金融监管合规 |
| **E. 财富管理** | `wealth_mgmt` | 投顾顾问(2)、资产配置(1)、客户画像(1) | ~4 | 面向个人/机构 |
| **F. 金融科技** | `fintech` | FinTech 产品经理(1)、金融数据工程(2)、区块链(1) | ~4 | 金融 × 科技交叉 |

### 5.3 预置 Team 模板（金融）

| Team Key | 名称 | 来源 | 模式 | 典型用途 |
|----------|------|------|------|---------|
| `team_premarket_brief` | 盘前简报 | stockx | coordinator | 08:30 自选股简报 |
| `team_stock_deep_dive` | 个股深度分析 | stockx | graph/coordinator | 五维并行分析 |
| `team_sector_rotation` | 板块扫描 | stockx | sequential | 周一行业轮动 |
| `team_portfolio_doctor` | 组合诊断 | stockx | parallel+synth | 持仓风险评估 |
| `team_market_recap` | 盘后复盘 | stockx | sequential | 17:00 日报 |
| `team_quant_strategy_research` | 量化策略研发 | finance 新增 | graph | 因子挖掘→回测→优化→评估 |
| `team_investment_committee` | 投委会决策 | finance 新增 | coordinator | 多维度综合决策会议 |
| `team_risk_monitoring` | 实时风控监控 | finance 新增 | parallel | 多维度风险实时扫描 |

---

## 6. 平台变更与实施路径

### 6.1 后端变更

| 变更项 | 类型 | 说明 |
|--------|------|------|
| **Ent Schema** | 新增 | `industry.go`, `department.go`, `position.go` 三张新表 |
| **agents 表扩展** | ALTER | 新增 `position_key`, `agent_variant`, `variant_description` 三字段 |
| **Biz 层** | 新增 | `IndustryUsecase`, `DepartmentUsecase`, `PositionUsecase` + Repo 接口 |
| **Data 层** | 新增 | 对应 Repo 实现（Ent CRUD） |
| **Service 层** | 新增 | `IndustryService`（CRUD + 安装/卸载/浏览） |
| **Proto** | 新增 | `api/kratos/industry/v1/industry.proto` |
| **Server** | 新增 | `register_industry.go` 路由注册 |
| **Scenario Install** | 新增 | 每个行业的 `install.go`（复用 stockx 模式） |
| **Seed CLI** | 扩展 | `cmd/seed-industries` 写入初始 taxonomy 数据 |

### 6.2 前端变更

| 页面/组件 | 说明 |
|----------|------|
| **行业市场页** `/industries` | 行业卡片列表（icon/name/description/Agent 数量）；一键安装按钮 |
| **行业详情页** `/industries/:key` | 部门树形展示 → 岗位列表 → Agent 卡片网格（含 variant 标签） |
| **岗位详情 Drawer** | 岗位职责、技能要求、关联 Agent 列表、所属 Team |
| **Agent 创建向导增强** | 选择行业→部门→岗位→variant，自动填充 prompt 模板和推荐工具集 |
| **Team 创建向导增强** | 从行业预置 Team 模板选择，自动填充成员 Agent |

### 6.3 实施路径建议

```
Phase 0: 平台框架（1 周）
  ├── Ent Schema: industry / department / position
  ├── agents 表扩展字段
  ├── Biz + Data + Service + Proto + Server 骨架
  └── Seed CLI: 写入 3 个行业的 taxonomy 骨架数据

Phase 1: 软件开发行业包（2-3 周）
  ├── 10 个 department + 45+ position 定义
  ├── 重点岗位 prompt 文件（Go/Vue/UE 各 3-4 个 variant）
  ├── 8 个 Team 模板
  ├── ~10 个 Skill 包
  └── 前端行业市场页 MVP

Phase 2: 自媒体行业包（1-2 周）
  ├── 5 个 department + 25+ position 定义
  ├── 小说创作部 prompt 精细化（重点）
  ├── 6 个 Team 模板
  └── ~8 个 Skill 包

Phase 3: 金融行业包（1-2 周）
  ├── 6 个 department + 30+ position 定义
  ├── 复用 stockx + 量化扩展
  ├── 8 个 Team 模板（含 stockx 6 个）
  └── ~8 个 Skill 包（含 stockx 13 个的引用）

Phase 4: 前端完善 + 联调（1 周）
  ├── 行业详情页完整交互
  ├── Agent/Team 创建向导集成
  └── E2E 验证
```

---

## 7. 关键设计决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 四层 vs 三层 | 四层（Industry→Dept→Position→Agent） | 部门层对大行业（如软件开发 10 个部门）有必要；岗位层确保粒度 |
| 一岗多 Agent 方式 | `agent_variant` 字段 | 轻量、无需额外关联表、查询高效 |
| Prompt 存储方式 | 文件系统（`prompts/` 目录）而非数据库 | Prompt 可能很长（上千字）；版本控制友好；复用 stockx 模式 |
| Taxonomy 存储 | SQLite Ent 表 | 与现有 agents/teams 同库；查询快；seed CLI 可批量写入 |
| 金融 vs stockx 关系 | finance 复用 + 扩展 | 避免 stockx 的 27 Tool / 13 Skill 重复定义；finance.install.go 内引用 stockx |
| 首期行业数量 | 3 个 | 覆盖面足够验证框架；预留扩展接口 |

---

## 8. 三行业总览汇总表

| 维度 | 软件开发 | 自媒体 | 金融/投资 | 合计 |
|------|---------|--------|---------|------|
| **部门数** | 10 | 5 | 6 | **21** |
| **岗位数** | ~45+ | ~25+ | ~30+ | **~100+** |
| **Agent 总数** | ~90-110 | ~50-60 | ~55-65 | **~195-235** |
| **Skill 包** | ~15 | ~12 | ~18 | **~45** |
| **Team 模板** | ~8-10 | ~6 | ~8 | **~22-24** |
| **特色** | UE 游戏方向 | 小说创作精细 Prompt | 复用 stockx + 量化扩展 | — |

---

## 9. 未来扩展

| 方向 | 说明 |
|------|------|
| **电商行业** | 运营/客服/供应链/数据分析 |
| **教育培训** | 课程设计/教研/辅导/学员管理 |
| **医疗健康** | 辅助诊断/健康管理/医学研究 |
| **法律服务** | 合同审查/案例分析/法规检索 |
| **用户自定义行业** | 开放 Industry 注册 API，允许第三方开发行业包并上架「行业市场」 |
