# Aranea-Agents 竞品深度对比分析报告

> 基于 2026-05-31 全景评估，对标 OpenClaw / Hermes Agent / Human-Agent 等 claw 系开源产品。
> 产品定位：面向一人开发公司的统一应用（服务+客户端一体），非 SaaS 平台。

---

## 一、竞品画像

### 1.1 OpenClaw（GitHub ⭐ 360k+）

**定位**：本地自托管 AI 助手平台，"你电脑上的超级秘书"。

| 维度 | 详情 |
|------|------|
| 核心架构 | Hub-and-Spoke：Gateway（WebSocket 控制面）+ Agent Runtime（AI 交互循环） |
| 技术栈 | Go（`trpc-agent-go/openclaw/`） |
| 渠道 | Telegram（原生）+ 插件扩展 |
| Skills | 60+ 内置 Skills（1password/discord/github/notion/slack/spotify 等） |
| 浏览器 | 完整 Playwright 自动化（25+ action + 导航安全策略） |
| 记忆 | 文件型 MEMORY.md + FTS5 全文搜索 |
| Persona | 5 个预设角色 + scope-based 隔离 |
| Runtime Profile | 完整 per-request 配置系统（模型/工具/知识/工作空间/凭证/技能/隔离） |
| Cron | 4 种调度（at/after/every/cron）+ ExecutionPolicy + outbound delivery |
| SubAgent | 后台派生 + completion delivery 通知 |
| Outbound | Router + voice/media/glob/opaque ref |
| 部署 | 单机 Docker / VPS / 本地 |
| 开源协议 | MIT |

### 1.2 Hermes Agent（GitHub ⭐ 90k+）

**定位**：自进化 AI 智能体，"the AI agent that grows with you"。

| 维度 | 详情 |
|------|------|
| 核心架构 | Python 单进程 + Gateway 消息网关 |
| 技术栈 | Python 88.5% + TypeScript 8.5% |
| 渠道 | Telegram/Discord/Slack/WhatsApp/Signal/CLI + 微信/QQ/飞书/钉钉 |
| Skills | 40+ 内置 + agentskills.io 开放标准 + **自创建技能** |
| 学习闭环 | 核心差异化——自主生成技能、记忆沉淀、跨会话检索、Honcho 用户建模 |
| 记忆 | FTS5 全文搜索 + LLM 摘要 + 跨会话持久化 |
| Cron | 内置 cron 调度 + 多平台 delivery |
| SubAgent | 并行子 Agent + RPC 工具调用 |
| 执行环境 | 6 种后端（Local/Docker/SSH/Singularity/Modal/Daytona） |
| 模型 | 200+ 模型无锁定（OpenRouter/Nous Portal/自定义端点） |
| 部署 | $5 VPS / GPU 集群 / Serverless（Modal/Daytona 休眠唤醒） |
| 浏览器 | 全浏览器自动化 + Vision + 图片生成 + TTS |
| 安全 | 容器加固（read-only root/dropped capabilities/PID limits） |
| 开源协议 | MIT |

### 1.3 Human-Agent

**定位**：具备自主规划、工具使用、记忆积累、自我纠错、协作能力、持续进化六大特征的 AI Agent。

| 特征 | 定义 |
|------|------|
| 自主规划 | Agent 能自主分解任务、制定执行计划 |
| 工具使用 | Agent 能选择和使用合适的工具完成任务 |
| 记忆积累 | Agent 能跨会话积累知识和偏好 |
| 自我纠错 | Agent 能检测和修正自身错误 |
| 协作能力 | Agent 能与其他 Agent 或人类协作 |
| 持续进化 | Agent 能从经验中学习、自我改进 |

---

## 二、项目自身竞品评分（来自 00-总览与路线图.md）

| 平台 | 编排 | 记忆 | 渠道 | 工具 | 自主性 | 可观测 | UI | 总分 |
|------|------|------|------|------|--------|--------|-----|------|
| **Aranea** | 9 | 9 | 10 | 7 | 5 | 8 | 9 | **57/70** |
| OpenClaw | 5 | 5 | 4 | 8 | 5 | 7 | 2 | 36/70 |
| Hermes | 4 | 8 | 2 | 9 | 9 | 3 | 2 | 37/70 |

**Human-Agent 达标度：~60%**（自主规划和自我纠错仅部分达标，持续进化部分达标）

---

## 三、代码级功能对比矩阵

### 3.1 核心运行时能力

| 能力 | Aranea | OpenClaw | Hermes | 代码级差距分析 |
|------|--------|----------|--------|---------------|
| Agent 编排 | ✅ LLM/A2A/Chain/Cycle/Parallel | ✅ LLM only | ✅ LLM only | **Aranea 领先**——5 种 Agent 类型 vs 竞品 1 种 |
| Graph 工作流 | ✅ DAG+BSP 双引擎+子图+熔断+容错+HITL+TimeTravel | ❌ 无 | ❌ 无 | **Aranea 大幅领先**——竞品均无图编排 |
| Team 协作 | ✅ Swarm/Graph 模式 | ❌ 无 | ⚠️ SubAgent only | **Aranea 领先**——多 Agent 编排能力 |
| A2A 协议 | ✅ Google A2A 完整实现 | ❌ 无 | ❌ 无 | **Aranea 独有** |
| Ralph Loop | ✅ Agent 自主迭代循环 | ❌ 无 | ⚠️ 类似能力 | **Aranea 领先** |
| Planner | ⚠️ A2UI Planner 已集成 | ❌ 无 | ⚠️ 类似能力 | 基本持平 |

### 3.2 记忆系统

| 能力 | Aranea | OpenClaw | Hermes | 代码级差距分析 |
|------|--------|----------|--------|---------------|
| 记忆层级 | ✅ L0-L4 五层 | ⚠️ MEMORY.md 文件 | ✅ FTS5+LLM 摘要 | **Aranea 领先**——五层架构远超竞品 |
| 向量检索 | ✅ pgvector + 混合检索 | ❌ 无 | ❌ 无 | **Aranea 领先** |
| 自动提取 | ✅ AutoMemoryQueue 异步 | ❌ 无 | ✅ 自主 nudge | 基本持平 |
| 用户可见性 | ❌ 仅 DB 存储 | ✅ MEMORY.md 可编辑 | ✅ FTS5 可搜索 | **Aranea 落后**——用户无法直接查看/编辑记忆 |
| GDPR 删除 | ❌ 无 | ✅ DeleteUser() | ✅ 有 | **Aranea 落后** |

### 3.3 工具/Skill 系统

| 能力 | Aranea | OpenClaw | Hermes | 代码级差距分析 |
|------|--------|----------|--------|---------------|
| 注册工具数 | 25+ | 60+ Skills | 40+ Skills | **Aranea 落后**——内置 Skills 数量不足 |
| MCP 支持 | ✅ MCP + MCP Broker | ✅ MCP | ✅ MCP | 持平 |
| Skill Eligibility | ❌ 无 | ✅ OS/bins/env/config 五维检查 | ✅ 有 | **Aranea 落后**——无 Skill 可用性评估 |
| Skill 自创建 | ⚠️ 后端核心已完成 | ❌ 无 | ✅ 核心差异化 | **Aranea 落后于 Hermes** |
| Skill 市场 | ❌ 无 | ❌ 无 | ✅ agentskills.io | **Aranea 落后于 Hermes** |
| SkillConfig | ❌ 无 | ✅ per-skill API Key/Env | ✅ 有 | **Aranea 落后** |
| 浏览器工具 | ⚠️ 仅 MCP 配置 | ✅ 25+ action + 双驱动 + 导航安全 | ✅ 完整浏览器自动化 | **Aranea 大幅落后** |

### 3.4 渠道集成

| 能力 | Aranea | OpenClaw | Hermes | 代码级差距分析 |
|------|--------|----------|--------|---------------|
| 平台数 | 12+ | 1（Telegram） | 10+（含微信/QQ/飞书/钉钉） | **Aranea 领先于 OpenClaw**，与 Hermes 持平 |
| 运行时管理 | ✅ supervisor/credentials/preview | ❌ 无 | ⚠️ 基础 | **Aranea 领先** |
| 框架接口标准化 | ⚠️ 自定义接口 | ✅ `channel.Channel` 标准接口 | ❌ 无 | **OpenClaw 领先** |

### 3.5 自主性与进化

| 能力 | Aranea | OpenClaw | Hermes | 代码级差距分析 |
|------|--------|----------|--------|---------------|
| 学习闭环 | ⚠️ 后端核心已完成 | ❌ 无 | ✅ 核心差异化 | **Aranea 落后于 Hermes** |
| Persona 系统 | ⚠️ 知识图谱动态身份 | ✅ 5 预设 + scope 隔离 | ⚠️ 有 | 各有千秋 |
| Runtime Profile | ❌ 无 | ✅ 完整 Profile + 策略 + 隔离 | ⚠️ 有 | **Aranea 大幅落后于 OpenClaw** |
| SubAgent Delivery | ❌ 无通知 | ✅ outbound delivery | ✅ 有 | **Aranea 落后** |
| Outbound Router | ⚠️ 基础实现 | ✅ voice/media/glob/opaque ref | ✅ 丰富 | **Aranea 落后** |

### 3.6 调度与运维

| 能力 | Aranea | OpenClaw | Hermes | 代码级差距分析 |
|------|--------|----------|--------|---------------|
| Cron 调度 | ⚠️ interval only | ✅ at/after/every/cron + ExecutionPolicy | ✅ cron + delivery | **Aranea 落后** |
| Cron Delivery | ❌ 无 | ✅ outbound delivery | ✅ 多平台 delivery | **Aranea 落后** |
| 死信机制 | ✅ maxDeadFailures + event bus | ❌ 无 | ❌ 无 | **Aranea 领先** |
| Prometheus | ✅ 完整 metrics | ❌ 无 | ❌ 无 | **Aranea 领先** |
| Debug Trace | ❌ 无 | ✅ debugrecorder | ❌ 无 | **OpenClaw 领先** |

### 3.7 可观测性

| 能力 | Aranea | OpenClaw | Hermes | 代码级差距分析 |
|------|--------|----------|--------|---------------|
| FlowLog | ✅ 完整事件日志 | ⚠️ 基础 | ❌ 无 | **Aranea 领先** |
| Trace | ✅ Trace + Alert | ⚠️ debugrecorder | ❌ 无 | **Aranea 领先** |
| Langfuse | ❌ 未集成 | ✅ 已集成 | ❌ 无 | **OpenClaw 领先** |
| Prometheus | ✅ 完整 metrics | ❌ 无 | ❌ 无 | **Aranea 领先** |

---

## 四、核心差距深度分析（代码级）

### 差距 #1：Skill 生态与可用性（🔴 P0）

**代码证据**：

- Aranea 的 `internal/skill/manifest/manifest.go` 使用手工 YAML 解析（`strings.Split`），不支持嵌套结构
- OpenClaw 的 `openclaw/internal/skills/frontmatter.go` 使用标准 `gopkg.in/yaml.v3`，支持 `openclaw` metadata 块
- OpenClaw 有 `evaluateSkill()` 五维检查（OS/bins/env/config），Aranea 无此概念
- OpenClaw 有 60+ 内置 Skills（`openclaw/skills/`），Aranea 无内置 Skills 目录
- Hermes 有 agentskills.io 开放标准和 Skill 自创建

**影响**：Skill 是 Agent 能力扩展的核心机制。缺少内置 Skills 和 eligibility 评估意味着用户需要手动配置每个 Skill 的可用性，体验远不如 OpenClaw 的"开箱即用"。

**OpenClaw eligibility 五维检查**：

1. OS 检查：`evaluateOpenClawOS(meta.OS)` —— 只在允许的操作系统上启用
2. 二进制依赖检查：`evaluateRequiredBins(meta.Requires.Bins)` —— `exec.LookPath` 搜索
3. 任一二进制检查：`evaluateRequiredAnyBins` —— 满足任一即可
4. 环境变量检查：`evaluateRequiredEnv` —— 含 `SkillConfig.APIKey` 回退
5. 配置键检查：`evaluateRequiredConfig` —— 前缀匹配

---

### 差距 #2：浏览器工具（🔴 P0）

**代码证据**：

- Aranea 的 `internal/tools/browser/config.go` 仅 60 行，只定义 `PlaywrightMCPConfig` 结构体
- OpenClaw 的 `openclaw/internal/browser/tool.go` 约 2219 行，实现 25+ action + 双驱动 + 导航安全策略
- Aranea 完全依赖框架 MCP toolset，自身不实现任何浏览器操作逻辑

**OpenClaw 浏览器工具能力**：

- 25+ action：status/start/stop/profiles/tabs/open/focus/close/snapshot/screenshot/navigate/console/cookies/storage/pdf/download/upload/dialog/offline/headers/credentials/geolocation/media/timezone/locale/device/act
- 双驱动：MCP Profile Driver + Browser Server Driver（HTTP REST API）
- 导航安全：domain allowlist/blocklist、loopback/private-net 限制、file URL 控制

**影响**：浏览器自动化是"Agent 能动手做事"的关键能力。OpenClaw 和 Hermes 都有完整的浏览器控制，Aranea 仅能通过 MCP 间接使用，功能受限。

---

### 差距 #3：Runtime Profile（🟠 P1）

**代码证据**：

- Aranea 无独立 Runtime Profile 包，使用 `biz.AgentRuntimeSetting`（静态配置）
- OpenClaw 的 `openclaw/runtimeprofile/profile.go` 约 815 行，实现完整 per-request 配置系统
- OpenClaw 有 `ResolveWorkdir()`/`CheckCredentialRef()`/`SkillVisibilityFilter()` 三个策略执行器
- OpenClaw 有三级隔离模式（Shared/ProfileCache/Service）

**OpenClaw Profile 结构**：

```go
type Profile struct {
    ID          string           // Profile ID
    Version     string           // 版本号
    AppName     string           // 应用名
    AgentName   string           // Agent 名
    ModelName   string           // 模型名
    Prompt      Prompt           // 提示词策略
    Tools       ToolPolicy       // 工具策略
    Knowledge   KnowledgePolicy  // 知识库策略
    Workspace   WorkspacePolicy  // 工作空间策略
    Credentials CredentialPolicy // 凭证策略
    Skills      SkillPolicy      // 技能策略
    Isolation   IsolationPolicy  // 隔离策略
    State       map[string]any   // 运行时状态
    ExtraModel  map[string]any   // 模型额外参数
}
```

**影响**：Runtime Profile 是"同一 Agent 在不同场景下表现不同"的关键机制。缺少此能力意味着无法实现"工作模式/休闲模式"等场景切换。

---

### 差距 #4：SubAgent Delivery 通知（🟠 P1）

**代码证据**：

- Aranea 的 `internal/tools/subagent/service.go` 无 delivery 通知——子 Agent 完成后只能通过 `get` 工具轮询
- OpenClaw 的 `openclaw/internal/subagentrun/service.go` 有 `notifyCompletion()` 方法，通过 `outbound.Router` 主动推送结果

**OpenClaw 的 Delivery 通知**：

```go
func (s *Service) notifyCompletion(record *runRecord) {
    if s.router == nil || record == nil { return }
    if record.Delivery.Channel == "" || record.Delivery.Target == "" { return }
    message := formatNotification(record)
    s.router.SendText(notifyCtx, outbound.DeliveryTarget{
        Channel: record.Delivery.Channel,
        Target:  record.Delivery.Target,
    }, message)
}
```

**影响**：没有 delivery 通知，用户派生子 Agent 后无法得知完成状态，必须主动轮询。这是"Agent 主动通知"能力的核心缺失。

---

### 差距 #5：Outbound Router 功能丰富度（🟠 P1）

**代码证据**：

- Aranea 的 `internal/outbound/tool.go` 只支持 `text/file/files/channel/target`
- OpenClaw 的 `openclaw/internal/outbound/tool.go` 支持 `media/as_voice/audio_as_voice` + 文件 glob 展开 + 目录展开 + opaque ref（`host://`/`artifact://`/`workspace://`）+ sentTextRecorder 去重

**影响**：Outbound 是 Agent 主动触达用户的核心通道。缺少 voice/media/glob 意味着 Agent 无法发送语音消息、无法批量发送文件、无法引用工作空间文件。

---

### 差距 #6：Cron 调度能力（🟡 P2）

**代码证据**：

- Aranea 的 `internal/cronrunner/runner.go` 使用简单 interval 调度
- OpenClaw 的 `openclaw/internal/cron/types.go` 支持 4 种调度（at/after/every/cron_expr + timezone）+ ExecutionPolicy（maxRuns/endsAt/overlapPolicy）+ outbound delivery + 模板渲染

**OpenClaw 的 Schedule 类型**：

```go
type Schedule struct {
    Kind     string  // at/after/every/cron
    At       string  // 绝对时间
    Every    string  // 间隔
    EveryMS  int64   // 毫秒间隔
    CronExpr string  // cron 表达式
    Timezone string  // 时区
}
```

**影响**：Cron 是"Agent 无人值守运行"的关键。缺少灵活调度和 delivery 意味着无法实现"每天早上 9 点发日报到 Telegram"等场景。

---

### 差距 #7：学习闭环与 Skill 自创建（🟡 P2）

**代码证据**：

- Aranea 的学习闭环后端核心已完成（`internal/biz/evolution*.go`），但缺 Proto/Service 层和前端
- Hermes 的学习闭环是核心差异化——自主生成技能、记忆沉淀、跨会话检索、Honcho 用户建模
- Aranea 的 Skill 自创建（`docs/需求/phase3-进化能力/02-技能自创建.md`）尚在需求阶段

**影响**：学习闭环是 Hermes 的核心卖点（"the AI agent that grows with you"），也是 Human-Agent 达标的关键维度（持续进化）。Aranea 在此维度落后于 Hermes。

---

## 五、Aranea 领先领域

### 领先 #1：编排能力（9/10 vs 竞品最高 5/10）

- 5 种 Agent 类型（LLM/A2A/Chain/Cycle/Parallel）vs 竞品 1 种
- Graph DAG+BSP 双引擎 + 子图 + 熔断 + 容错 + HITL + TimeTravel——竞品均无
- Team Swarm/Graph 模式——竞品均无
- A2A 协议——竞品均无

### 领先 #2：记忆系统（9/10 vs 竞品最高 8/10）

- L0-L4 五层记忆架构——竞品最精细
- pgvector 向量检索 + 混合检索 + 自适应路由 + 检索评估——竞品无
- AutoMemoryQueue 异步提取——竞品无

### 领先 #3：渠道覆盖（10/10 vs 竞品最高 4/10）

- 12+ 平台适配（含 dingtalk/discord/lark/line/mattermost/onebot/qq/slack/teams/telegram/wechat/wecom）
- 完整运行时管理（supervisor/credentials/preview）
- OpenClaw 仅 1 个平台（Telegram），Hermes 约 10 个

### 领先 #4：可观测性（8/10 vs 竞品最高 7/10）

- FlowLog + Trace + Alert + Prometheus——完整可观测性栈
- 30+ Envelope 类型的事件系统
- 竞品中仅 OpenClaw 有 debugrecorder，Hermes 几乎无可观测性

### 领先 #5：UI/管理界面（9/10 vs 竞品最高 2/10）

- 45+ 页面、200+ 组件、44 个 Store
- Graph 可视化编辑器 + Time Travel 面板
- OpenClaw/Hermes 几乎无 Web UI（仅 CLI + 消息平台）

---

## 六、框架能力利用率分析

| 维度 | 数据 |
|------|------|
| 框架子系统总数 | 62 |
| 已使用 | 34（~55%） |
| 未使用但框架已具备 | 28 |
| 框架未具备需自研 | 浏览器工具、学习闭环、技能自创建、Skill 市场 |

**未使用的 28 个框架子系统**是快速补齐差距的关键杠杆——不需要从零开发，只需集成框架已有能力。

---

## 七、改进建议与优先级

### P0 — 核心能力补齐（对标 OpenClaw/Hermes 的必备能力）

| # | 改进项 | 对标竞品 | 代码级工作 | 收益 |
|---|--------|---------|-----------|------|
| 1 | 内置 Skills 库 | OpenClaw 60+ Skills | 移植 `openclaw/skills/` 到项目 + 实现 eligibility 评估 | 开箱即用能力 |
| 2 | 浏览器工具实现 | OpenClaw browser tool | 集成 `openclaw/internal/browser/` 或基于框架 MCP toolset 扩展 | Agent 可操作网页 |
| 3 | Skill Eligibility + SkillConfig | OpenClaw evaluateSkill | 移植 `openclaw/internal/skills/repository.go` 的五维检查 | Skill 可用性自动评估 |

### P1 — 运行时能力增强

| # | 改进项 | 对标竞品 | 代码级工作 | 收益 |
|---|--------|---------|-----------|------|
| 4 | Runtime Profile | OpenClaw runtimeprofile | 移植 `openclaw/runtimeprofile/` 包 + 策略执行 + 隔离模式 | 场景化配置 |
| 5 | SubAgent Delivery | OpenClaw notifyCompletion | 在 `internal/tools/subagent/service.go` 增加 outbound delivery | 子 Agent 主动通知 |
| 6 | Outbound 增强 | OpenClaw outbound tool | 增加 voice/media/glob/opaque ref 支持 | Agent 主动触达能力 |
| 7 | Cron 调度增强 | OpenClaw cron 4 种调度 | 移植 `openclaw/internal/cron/types.go` + ExecutionPolicy + delivery | 灵活定时任务 |
| 8 | Persona 预设系统 | OpenClaw persona store | 增加预设模板 + scope 隔离 + alias 切换 | 用户可切换角色 |

### P2 — 进化与差异化

| # | 改进项 | 对标竞品 | 代码级工作 | 收益 |
|---|--------|---------|-----------|------|
| 9 | 学习闭环打通 | Hermes Learning Loop | 补齐 Proto/Service 层 + 前端 | Agent 从经验中学习 |
| 10 | Skill 自创建 | Hermes Skill Self-Creation | 实现需求文档 `02-技能自创建.md` | 能力自动增长 |
| 11 | Skill 市场 | Hermes agentskills.io | 实现需求文档 `03-Skill市场生态.md` | 社区驱动扩展 |
| 12 | Debug Recorder | OpenClaw debugrecorder | 移植 `openclaw/internal/debugrecorder/` | 开发调试效率 |
| 13 | Langfuse 集成 | OpenClaw langfuse | 集成 `openclaw/app/langfuse.go` | 生产级 Trace |
| 14 | Memory 用户可见性 | OpenClaw MEMORY.md | 增加 MEMORY.md 导出/编辑能力 | 用户可查看/编辑记忆 |

---

## 八、战略定位

### 当前竞争格局

```
                    编排能力
                      ↑
         Aranea ●     |
                     |
                     |        ● OpenClaw
                     |
                     |
         Hermes ●    |
                     |
                    ───────────────→ 自主性/进化
```

- **Aranea**：编排能力最强（9/10），但自主性不足（5/10）
- **OpenClaw**：工具生态最强（8/10），但编排弱（5/10）
- **Hermes**：自主性最强（9/10），但编排弱（4/10）

### 差异化策略

**短期**（补齐 P0）：将 Aranea 从"编排强但工具弱"变为"编排+工具双强"

**中期**（补齐 P1）：将 Aranea 从"工具强但自主性弱"变为"编排+工具+运行时三强"

**长期**（P2 进化）：将 Aranea 从"运行时三强"变为"编排+工具+运行时+进化四强"——这是超越 Hermes 的路径

### 核心竞争壁垒

1. **编排深度**：Graph 双引擎 + Team + A2A 是竞品短期内无法复制的
2. **渠道广度**：12+ 平台适配是 OpenClaw/Hermes 不具备的
3. **管理界面**：45+ 页面的 Web UI 是竞品（仅 CLI）无法比拟的
4. **架构纪律**：Kratos/Wire/kerrors/safego 的工程规范是竞品（裸 `go func()`/`fmt.Errorf`）不具备的

---

## 九、总结

| 维度 | 评价 |
|------|------|
| 编排能力 | ⭐⭐⭐⭐⭐ — 远超竞品，Graph/Team/A2A 是核心壁垒 |
| 记忆系统 | ⭐⭐⭐⭐⭐ — 五层架构 + 向量检索，远超竞品 |
| 渠道集成 | ⭐⭐⭐⭐⭐ — 12+ 平台，远超竞品 |
| 可观测性 | ⭐⭐⭐⭐ — FlowLog/Trace/Prometheus，领先竞品 |
| UI/管理 | ⭐⭐⭐⭐⭐ — 45+ 页面，竞品几乎无 UI |
| Skill 生态 | ⭐⭐⭐ — 无内置 Skills、无 eligibility、无市场，落后 OpenClaw/Hermes |
| 浏览器工具 | ⭐⭐ — 仅 MCP 配置，大幅落后 OpenClaw/Hermes |
| 自主性/进化 | ⭐⭐⭐ — 学习闭环核心已完成但未打通，落后 Hermes |
| Runtime Profile | ⭐⭐ — 完全缺失，落后 OpenClaw |
| Outbound 能力 | ⭐⭐⭐ — 基础实现，落后 OpenClaw |

**一句话总结**：Aranea-Agents 在编排深度、记忆架构、渠道覆盖、管理界面四个维度已建立对 OpenClaw/Hermes 的结构性优势，但在 Skill 生态、浏览器工具、自主性进化、Runtime Profile 四个维度存在明显差距。好消息是，大部分差距可通过集成框架已有能力（28 个未使用子系统）或移植 OpenClaw 代码快速补齐，无需从零开发。
