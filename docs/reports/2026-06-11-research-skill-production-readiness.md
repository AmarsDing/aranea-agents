# Research: Skill Module Production Readiness & Competitive Gap Analysis

> **日期**：2026-06-11
> **版本**：v1.0
> **范围**：Skill 模块生产就绪性评估 + 竞品（Claude Code / OpenAI Codex / Trae IDE / WorkBuddy）对比

---

## 摘要

对 Aranea-Agents Skill 模块进行系统性生产就绪性评估，并与 4 家竞品（Claude Code、OpenAI Codex、Trae IDE、WorkBuddy）进行功能对比。核心发现：Skill 模块核心流程已达 MVP+ 水平，在自动演化、语义路由、版本管理方面具备独有优势，但在**安全隔离、信任分级、跨客户端互操作、Skill 市场**四个维度存在显著差距。安全隔离和信任分级是上生产的硬阻塞，建议按 P0（安全）→ P1（生态）→ P2（体验）优先级推进。

---

## 一、Skill 模块现状评估

### 1.1 架构总览

Skill 系统采用双框架分层架构：

```
┌─────────────────────────────────────────────────────────────┐
│                     前端 (Vue 3 + Quasar)                    │
│  web/src/features/skills/ (types, api, composables)         │
├─────────────────────────────────────────────────────────────┤
│              Service 层 (Kratos HTTP/gRPC)                   │
│  internal/service/skill.go + skill_intelligence.go + ...    │
├─────────────────────────────────────────────────────────────┤
│               Biz 层 (业务逻辑)                              │
│  internal/biz/skill/skill.go (Usecase + 接口定义)            │
│  internal/biz/skill_health.go / skill_evolution.go          │
│  internal/biz/skill_similarity.go / skill_load_mode.go      │
├─────────────────────────────────────────────────────────────┤
│               Data 层 (Ent ORM + SQLite)                     │
│  internal/data/skill.go (skillRepo)                          │
│  internal/data/ent/schema/skill_version.go                   │
│  internal/data/ent/schema/skill_invocation.go                │
├─────────────────────────────────────────────────────────────┤
│           运行时内核 (pkg/trpc-agent-go)                     │
│  skill/ 包: Repository + FSRepository + ContextRepository   │
│  internal/flow/processor/skills.go (SkillsRequestProcessor) │
│  internal/workspaceprep/skill.go (工作区物化)                │
│  internal/skillstage/ / internal/skillprofile/              │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 核心能力清单

| 能力 | 实现状态 | 关键文件 |
|------|---------|---------|
| SKILL.md 定义格式 | ✅ 已实现 | `pkg/trpc-agent-go/skill/repository.go` |
| 多根目录扫描 | ✅ 已实现 | `FSRepository` 支持 `NewFSRepository(roots...)` |
| URL 下载与缓存 | ✅ 已实现 | `pkg/trpc-agent-go/skill/url_root.go`（ZIP/TAR.GZ，SHA256 缓存） |
| 上下文感知过滤 | ✅ 已实现 | `ContextRepository` + `VisibilityFilter` |
| 三层信息模型 | ✅ 已实现 | 概览 → 正文 → 文档，按需注入 |
| 4 种加载模式 | ✅ 已实现 | turn / once / session / progressive |
| 意图路由 | ✅ 已实现 | `RuntimePolicy` + `IntentRoutingEnabled` |
| Embedding 语义评分 | ✅ 已实现 | 余弦相似度 + 30min 缓存 + 自动失效 |
| CRUD + 权限控制 | ✅ 已实现 | admin/non-admin 权限矩阵 |
| 语义版本 + 回滚 | ✅ 已实现 | `SkillVersion` 表 + `RollbackSkillVersion` |
| 自动演化 | ✅ 已实现 | 模式检测 → LLM 生成 → 审批流 → 注册 |
| 调用审计 | ✅ 已实现 | `SkillInvocation` 表（selection_reason / outcome / token_usage） |
| 文件系统健康监控 | ✅ 已实现 | `FilesystemHealthStats` + `MarkSkillFilesystemMissing` |
| 前端管理 UI | ✅ 已实现 | `web/src/features/skills/`（~1030 行 TS） |

### 1.3 生产就绪性评分

| 维度 | 评分 | 说明 |
|------|------|------|
| 核心功能完整性 | ★★★★☆ | CRUD、版本管理、发布流程、磁盘同步、调用记录均已实现 |
| 运行时集成 | ★★★★☆ | 三层信息模型 + 4 种加载模式 + 意图路由 + Embedding 评分 |
| 并发安全 | ★★★★☆ | RWMutex 保护索引/缓存，但 ID 生成用 `UnixNano()` 有碰撞风险 |
| 错误处理 | ★★★☆☆ | 有补偿回滚但非原子性；运行时错误静默 continue 可能丢失关键信息 |
| 安全性 | ★★★☆☆ | 路径遍历防护有，但缺少沙箱隔离、信任分级、脚本执行安全边界 |
| 可观测性 | ★★★☆☆ | 有 Invocation 记录和健康统计，缺 Skill 级别 Metrics/Traces |
| 测试覆盖 | ★★★☆☆ | Biz 层 ~1300 行单测，但缺集成测试、安全测试、性能测试 |
| 开放标准兼容 | ★★☆☆☆ | SKILL.md 格式兼容，但未实现 `.agents/skills/` 跨客户端互操作 |

**综合评估：MVP+ 级别，核心流程可用但距生产级有 3-5 个关键缺口。**

### 1.4 已知技术债

| 问题 | 严重度 | 位置 | 说明 |
|------|--------|------|------|
| ID 生成碰撞风险 | 高 | `internal/data/skill.go` | `time.Now().UTC().UnixNano()` 高并发下可能碰撞 |
| 状态机缺失 | 高 | `internal/biz/skill/` | draft/published/archived/deleted 转换无显式状态机（违反 AS-FSM-01） |
| 事务外文件写入 | 中 | `PatchSkill` | 文件系统写入在事务外，存在短暂不一致窗口 |
| 缓存一致性 | 中 | `Usecase` | embedding 缓存失效依赖操作后主动调用，遗漏则过期 |
| SkillVersion 缺索引 | 低 | `skill_version.go` | `skill_id + created_at` 复合索引未定义 |
| 运行时日志规范 | 低 | `FSRepository` | 使用 `log.Printf`（运行时内核有独立日志桥接，可接受） |

---

## 二、竞品 Skill 系统深度对比

### 2.1 竞品画像

#### Claude Code（Anthropic）

| 维度 | 详情 |
|------|------|
| Skill 格式 | SKILL.md 开放标准（YAML frontmatter + Markdown 指令体） |
| 核心设计 | **渐进式披露**（Progressive Disclosure）：L0 目录 → L1 指令 → L2 资源，96% 上下文开销削减 |
| 存放位置 | `.claude/skills/`（项目级）、`~/.claude/skills/`（用户级）、`.agents/skills/`（跨客户端） |
| 生命周期 | 目录放入即安装，无内置版本管理（依赖 Git） |
| 安全 | 7 种权限模式 + ML 分类器 + Pre/Post hooks + 3 级权限（ReadOnly/WorkspaceWrite/DangerFullAccess） |
| 子代理 | 6 种专用子代理，单层级委派（防递归爆炸） |
| 生态 | 官方仓库 `github.com/anthropics/skills`，30+ 编码工具采纳开放标准 |

#### OpenAI Codex

| 维度 | 详情 |
|------|------|
| Skill 格式 | SKILL.md 开放标准（与 Claude Code 同一规范） |
| 存放位置 | `.codex/skills/`（项目级）、`~/.codex/skills/`（用户级）、`.agents/skills/`（跨客户端） |
| 安全 | **云容器隔离**（Firecracker microVM），禁用互联网访问，所有操作可追溯 |
| 智能体循环 | 经典 while-loop：推理 → 工具调用 → 结果回填 → 继续推理 |
| 生态 | 官方仓库 `github.com/openai/skills`（19.3k stars），社区活跃 |

#### Trae IDE（字节跳动）

| 维度 | 详情 |
|------|------|
| Skill 格式 | SKILL.md 开放标准 + 内置 skill-creator 工具 |
| 存放位置 | `.trae/skills/`（项目级）、`~/.trae/skills/`（全局）、`.agents/skills/`（跨客户端，v3.5.44+） |
| 特色 | SOLO Agent 系统（Coder/Builder/Plan/Spec 模式）、100+ Expert Teams |
| 安全 | WSL2 沙箱、MCP OAuth、终端命令需用户确认 |
| 生态 | 内置 Skills Gallery（预审查）、社区分享 |

#### WorkBuddy（腾讯）

| 维度 | 详情 |
|------|------|
| Skill 架构 | MCP 连接器 + Skills Gallery 双层 |
| 特色 | 100+ Expert Roles、30+ MCP 连接器、IM 远程控制（Slack/Telegram/微信） |
| 多智能体 | 任务图编排，多专家并行，共享状态存储通信 |
| 安全 | 默认沙箱、文件系统需授权、角色权限控制 |
| 生态 | 内置 Skills Gallery、Miora 设计助手、TokenHub 模型网关 |

### 2.2 功能对比矩阵

| 能力维度 | Aranea-Agents | Claude Code | OpenAI Codex | Trae IDE | WorkBuddy |
|----------|--------------|-------------|--------------|----------|-----------|
| **Skill 定义格式** | SKILL.md ✅ | SKILL.md 开放标准 | SKILL.md 开放标准 | SKILL.md 开放标准 | MCP 连接器 + Gallery |
| **渐进式披露** | 4 种模式 ✅ | 三层（L0/L1/L2）✅ | 三层 ✅ | 三层 ✅ | 无（全量加载） |
| **跨客户端互操作** | ❌ | ✅ `.agents/skills/` | ✅ `.agents/skills/` | ✅ `.agents/skills/` | ❌ |
| **Skill 信任分级** | ❌ | 7 种权限 + ML 分类器 | 云容器隔离 | 沙箱 + 权限确认 | 角色权限 + 沙箱 |
| **版本管理** | ✅ 语义版本 + 回滚 | ❌ 依赖 Git | ❌ 依赖 Git | ❌ | ❌ 平台统一管理 |
| **自动演化** | ✅ 模式检测 + LLM + 审批 | ❌ | ❌ | ❌ | ❌ |
| **Embedding 语义路由** | ✅ 余弦相似度 + 缓存 | ❌ | ❌ | ❌ | ❌ |
| **调用记录/审计** | ✅ Invocation 表 | ❌ | ✅ 终端日志 | ❌ | ❌ |
| **沙箱隔离** | ❌ | Docker + 权限门控 | Firecracker microVM | WSL2 沙箱 | 默认沙箱 |
| **Skill 市场/共享** | ❌ | 官方仓库 + 社区 | 官方仓库 + 社区 | Skills Gallery | Skills Gallery |
| **安全审查** | ❌ | ML 分类器 + 人工审查 | 云端隔离 | 预审查 Gallery | 预审查 Gallery |
| **脚本执行安全** | workspace_exec 无边界 | 3 级权限 + hooks | 完全隔离 | 用户确认 | 沙箱隔离 |
| **Hooks 机制** | ❌ | Pre/Post hooks | ❌ | ❌ | ❌ |
| **Skill 组合/依赖** | ❌ | Skill + MCP 互补 | ❌ | ❌ | 多专家并行 |

### 2.3 行业开放标准：Agent Skills（agentskills.io）

2025 年 12 月由 Anthropic 发布为开放标准，2026 年 4 月已被 30+ 编码智能体工具采纳，成为事实标准。

**核心规范**：

```
skill-name/
├── SKILL.md          # 必须：YAML frontmatter + Markdown 指令
├── scripts/          # 可选：可执行代码
├── references/       # 可选：附加文档
└── assets/           # 可选：模板、数据文件
```

**渐进式披露三层架构**：

| 层级 | 内容 | 加载时机 | Token 成本 |
|------|------|---------|-----------|
| L1 Catalog | name + description | 会话启动 | ~50-100 tokens/skill |
| L2 Instructions | SKILL.md 完整体 | 技能激活时 | <5000 tokens |
| L3 Resources | scripts/references/assets | 指令引用时 | 不定 |

**跨客户端互操作**：`.agents/skills/` 目录约定，各客户端均扫描此目录实现 Skill 共享。

---

## 三、差距分析与提升方案

### 3.1 P0 — 必须补齐（安全/可靠性，生产硬阻塞）

#### P0-1：沙箱隔离

| 项 | 说明 |
|----|------|
| **现状** | `workspace_exec` 可执行任意脚本，无隔离边界 |
| **竞品做法** | Codex 用 Firecracker microVM；Claude Code 用 Docker + 3 级权限 + hooks |
| **风险** | 恶意 Skill 可通过脚本执行窃取数据、破坏文件系统、发起网络攻击 |
| **方案** | |
| 短期 | 命令白名单 + 资源限制（CPU/内存/超时）+ 禁止网络访问 |
| 中期 | gVisor 系统调用级隔离（适合计算密集多租户） |
| 长期 | Firecracker microVM（最强隔离，适合合规场景） |
| **验证** | 沙箱逃逸测试 + 路径遍历测试 + 网络出口测试 |

#### P0-2：Skill 信任分级

| 项 | 说明 |
|----|------|
| **现状** | 所有 Skill 同等信任，无来源区分 |
| **竞品做法** | Claude Code 7 种权限模式 + ML 风险分类；学术建议 T1-T4 四层信任 |
| **风险** | 26.1% 的社区 Skill 包含漏洞（Xu & Yan, 2026），无分级等于全部信任 |
| **方案** | |
| 数据模型 | `PlatformSkill` 增加 `trust_level` 字段（official / verified / community / unverified） |
| 加载策略 | T1(official)：全部加载；T2(verified)：指令 + 受限脚本；T3(community)：仅指令；T4(unverified)：仅元数据 |
| 对应渐进式披露 | L1 元数据 T1-T4 可见；L2 指令需 T3+；L3 可执行脚本需 T1-T2 |
| **验证** | 信任等级越级加载被拒绝 + 各等级加载内容符合预期 |

#### P0-3：状态机显式化

| 项 | 说明 |
|----|------|
| **现状** | draft/published/archived/deleted 状态转换散落在各方法中，无显式状态机（违反 AS-FSM-01） |
| **方案** | |
| 文件 | `internal/biz/skill/skill_state_machine.go` |
| 内容 | 状态枚举（const）+ 合法转换表（var transitions）+ 转换校验函数 + 可选守卫条件 |
| 转换表 | `draft → published`（需审批）、`published → archived`、`published → draft`（内容变更自动回退）、`archived → published`（重新发布）、`* → deleted`（软删除） |
| **验证** | 非法转换返回 error + 守卫条件生效 |

#### P0-4：ID 生成碰撞修复

| 项 | 说明 |
|----|------|
| **现状** | `time.Now().UTC().UnixNano()` 高并发下可能碰撞 |
| **方案** | 改用 UUID v7（时间有序 + 碰撞安全），兼容现有 int64 字段可用 snowflake |
| **验证** | 并发创建 10000 个 Skill 无 ID 碰撞 |

### 3.2 P1 — 重要提升（生态/兼容）

#### P1-1：跨客户端互操作

| 项 | 说明 |
|----|------|
| **现状** | 不支持 `.agents/skills/` 目录约定 |
| **竞品做法** | Claude Code / Codex / Trae 均已实现，这是 2025-12 发布的事实标准 |
| **方案** | |
| 扫描扩展 | `FSRepository` 增加 `.agents/skills/` 根目录自动扫描 |
| 优先级 | 项目级 `.trae/skills/` > 跨客户端 `.agents/skills/` > 用户级 `~/.trae/skills/` |
| 名称冲突 | 确定性优先级规则（项目级 > 用户级 > 全局级） |
| **验证** | `.agents/skills/` 中的 Skill 被正确发现和加载 |

#### P1-2：Skill 市场/共享机制

| 项 | 说明 |
|----|------|
| **现状** | 无 |
| **竞品做法** | Claude Code / Codex 有官方仓库 + 社区生态；Trae / WorkBuddy 有内置 Gallery |
| **方案** | |
| 短期 | 完善 ZIP 导入（已有 `uploadSkillZip` API），增加冲突检测和合并策略 |
| 中期 | 建设 Gallery 服务 + 安全扫描流水线 + 版本签名 |
| 长期 | 社区贡献 + 评分 + 信任等级自动升降 |
| **验证** | ZIP 导入端到端测试 + 冲突检测覆盖 |

#### P1-3：安全扫描流水线

| 项 | 说明 |
|----|------|
| **现状** | 无 |
| **行业数据** | 26.1% 的社区 Skill 包含漏洞 |
| **方案** | |
| 扫描内容 | 路径遍历模式、命令注入模式、敏感信息泄露、异常网络调用 |
| 触发时机 | Skill 发布前 + ZIP 导入时 |
| 输出 | `validation_status` 字段（pass / warning / fail）+ 详细报告 |
| **验证** | 包含已知漏洞模式的 Skill 被正确拦截 |

### 3.3 P2 — 体验优化（差异化竞争力）

#### P2-1：可观测性增强

| 项 | 说明 |
|----|------|
| **现状** | 有 Invocation 记录，缺 Skill 级 Metrics |
| **方案** | |
| 指标 | Skill 激活率、Token 消耗/调用、错误率、P95 延迟、缓存命中率 |
| 接入 | 通过 loggateway Pipeline 统一输出 |
| 展示 | 前端 Skill 详情页增加 Metrics 面板 |
| **验证** | Metrics 数据可查询 + 异常检测告警 |

#### P2-2：Skill 组合/依赖声明

| 项 | 说明 |
|----|------|
| **现状** | 单 Skill 加载，无组合机制 |
| **竞品做法** | Claude Code Skill + MCP 互补；WorkBuddy 多专家并行 |
| **方案** | |
| 声明 | SKILL.md frontmatter 增加 `requires: [skill-a, skill-b]` 字段 |
| 解析 | 运行时自动解析依赖图，循环依赖检测 |
| 加载 | 拓扑排序后按序加载，失败时整体回滚 |
| **验证** | 依赖解析正确 + 循环依赖报错 + 部分加载失败回滚 |

#### P2-3：Hooks 机制

| 项 | 说明 |
|----|------|
| **现状** | 无 |
| **竞品做法** | Claude Code Pre/Post hooks 可拦截/修改/拒绝任何工具操作 |
| **方案** | |
| Hook 点 | BeforeLoad / AfterLoad / BeforeExec / AfterExec |
| 注册 | `RuntimePolicy` 增加 `Hooks` 字段 |
| 能力 | 拦截（返回 error 阻止）、修改（替换参数）、审计（记录日志） |
| **验证** | Hook 拦截生效 + 修改参数传递正确 |

---

## 四、独有优势保持与强化

| 优势 | 竞品对比 | 强化建议 |
|------|---------|---------|
| **自动演化系统** | 独有，竞品均无 | 增加演化效果评估闭环：Skill 创建后追踪使用率/成功率，低效 Skill 自动降级或归档 |
| **Embedding 语义路由** | 独有 | 增加用户反馈信号（thumbs up/down）调优路由权重，实现路由自优化 |
| **语义版本 + 回滚** | 独有，竞品依赖 Git | 保持：这是企业级特性，竞品缺失。可增加 diff 视图和变更日志 |
| **调用审计** | 最详细 | 增加 Token 级别的成本追踪，支持按 Skill/Agent/Session 维度聚合 |

---

## 五、实施路线图

### Phase 1：安全加固（P0，生产前置条件）

| 任务 | 依赖 | 验证标准 |
|------|------|---------|
| P0-1 沙箱隔离（短期：命令白名单 + 资源限制） | 无 | 恶意命令被拦截 |
| P0-2 信任分级（数据模型 + 加载策略） | 无 | 越级加载被拒绝 |
| P0-3 状态机显式化 | 无 | 非法转换返回 error |
| P0-4 ID 生成修复 | 无 | 并发无碰撞 |

### Phase 2：生态兼容（P1，竞争力提升）

| 任务 | 依赖 | 验证标准 |
|------|------|---------|
| P1-1 跨客户端互操作 | 无 | `.agents/skills/` Skill 被发现加载 |
| P1-2 ZIP 导入完善 | P0-3 | 端到端导入 + 冲突检测 |
| P1-3 安全扫描流水线 | P0-2 | 漏洞 Skill 被拦截 |

### Phase 3：体验优化（P2，差异化竞争力）

| 任务 | 依赖 | 验证标准 |
|------|------|---------|
| P2-1 可观测性增强 | 无 | Metrics 可查询 |
| P2-2 Skill 组合/依赖 | P0-3 | 依赖解析 + 循环检测 |
| P2-3 Hooks 机制 | P0-1 | Hook 拦截/修改生效 |

---

## 六、参考资料

### 学术论文

- Piskala, M. (2026). "Agent, Sub-Agent, Skill, or Tool? A Taxonomy for AI Agent Abstractions." TechRxiv.
- Xu, C. & Yan, Q. (2026). "Agent Skills for LLMs: Architecture, Acquisition, Security." arXiv:2602.12430.
- Zhang, M. (2026). "AI Agents: Engineering Over Intelligence." marvinzhang.dev.

### 行业标准

- Agent Skills Open Standard — agentskills.io（2025-12 发布，30+ 工具采纳）
- OWASP AI Agent Security Guidelines (2026)
- NVIDIA AI Agent Sandboxing Best Practices (2026)

### 竞品文档

- Anthropic. "Equipping agents for the real world with Agent Skills." claude.com/blog
- OpenAI. "Unrolling the Codex Agent Loop." openai.com
- Trae IDE 技能文档. docs.trae.ai/ide/skills
- Tencent WorkBuddy 公告. trotons.com / news.lavx.hu

### 内部文档

- `docs/reports/2026-06-11-review-architecture-runtime-pain-points.md` — 架构痛点评审（AS-FSM-01 状态机标准）
- `docs/reports/2026-05-31-analysis-competitive.md` — 竞品深度对比分析
- `pkg/trpc-agent-go/docs/mkdocs/zh/skill.md` — Skill 运行时文档（~1060 行）
