# Aranea-Agents 模块开发计划索引

> **版本**：2026-05-17 | **维护规则**：每个模块一个 `xxx-development.md`，本文件为索引
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 全局优先级说明

| 优先级 | 含义 |
|--------|------|
| P0 | 阻塞性缺失，必须立即修复 |
| P1 | 核心功能缺失，影响主流程 |
| P2 | 重要功能缺失，影响用户体验 |
| P3 | 优化/增强，可排期 |

---

## 模块状态速览

| # | 模块 | 开发计划 | 状态 | 关键差距 |
|---|------|----------|------|----------|
| 0 | System 系统 | [0-system-development.md](./0-system-development.md) | ✅ | SQLite→PG 迁移 |
| 1 | Chat 对话 | [1-chat-development.md](./1-chat-development.md) | ✅ | member_* SSE 事件 |
| 2 | Agent 创建 | [2-agents-create-development.md](./2-agents-create-development.md) | ✅ | Provider 校验 |
| 3 | Agent 列表 | [3-agent-list-development.md](./3-agent-list-development.md) | ✅ | 运行状态聚合 |
| 4 | Agent 分类 | [4-agent-type-development.md](./4-agent-type-development.md) | ✅ | 循环引用校验 |
| 5 | Agent 设置 | [5-agent-setting-development.md](./5-agent-setting-development.md) | 🟡 | **ToolOverride 未实现** |
| 6 | Agent 提示文件 | [6-agent-setting-file-development.md](./6-agent-setting-file-development.md) | ✅ | 版本历史 |
| 7 | Agent 进化 | [7-agent-evolution-development.md](./7-agent-evolution-development.md) | 🟡 | **EvolutionScanner 未实现** |
| 8 | Agent 标题 | [8-agent-title-development.md](./8-agent-title-development.md) | ✅ | 自动生成 |
| 9 | Provider | [9-provider-development.md](./9-provider-development.md) | ✅ | 凭据加密 |
| 10 | Session | [10-session-development.md](./10-session-development.md) | ✅ | 置顶/导出 |
| 11 | Multi-Agent | [11-multi-agent-development.md](./11-multi-agent-development.md) | 🟡 | **call_agent 未注入** |
| 12-16 | Memory 记忆 | [memory-development.md](./memory-development.md) | 🟡 | **L4 知识图谱未实现** |
| 17 | Channel 渠道 | [17-channel-development.md](./17-channel-development.md) | 🟡 | **Webhook/投递未实现** |
| 18 | Monitor 监控 | [18-monitor-development.md](./18-monitor-development.md) | 🟡 | Dashboard/告警 |
| 19 | MCP 协议 | [19-mcp-development.md](./19-mcp-development.md) | ✅ | 健康检查 |
| 20 | Skill 技能 | [20-skill-development.md](./20-skill-development.md) | ✅ | 版本管理 |
| 21 | Cron 定时 | [21-cron-development.md](./21-cron-development.md) | 🟡 | **调度引擎未实现** |
| 22 | Plugin 插件 | [22-plugin-development.md](./22-plugin-development.md) | ✅ | 沙箱隔离 |
| 23 | Tools 工具 | [23-tools-development.md](./23-tools-development.md) | ✅ | ToolOverride |
| 24 | Telemetry 遥测 | [24-telemetry-development.md](./24-telemetry-development.md) | ✅ | 自定义 Span |
| 25 | CLI 命令行 | [25-cli-development.md](./25-cli-development.md) | ❌ | **完全未实现** |
| 26 | A2A 协议 | [26-a2a-development.md](./26-a2a-development.md) | 🟡 | **call_agent 未注入** |
| 27 | Artifact 产出物 | [27-artifact-development.md](./27-artifact-development.md) | 🟡 | 版本管理/预览 |
| 28 | Callback 回调 | [28-callback-development.md](./28-callback-development.md) | 🟡 | **回调投递未实现** |
| 29 | Token 用量 | [29-token-development.md](./29-token-development.md) | ✅ | 用量限额 |
| 30 | Ecosystem 生态 | [30-ecosystem-development.md](./30-ecosystem-development.md) | ❌ | **完全未实现** |
| 32 | CodeExecutor | [32-codeexecutor-development.md](./32-codeexecutor-development.md) | 🟡 | E2B/Jupyter |
| 33 | Evaluation 评估 | [33-evaluation-development.md](./33-evaluation-development.md) | 🟡 | 自动评估 |
| 34 | Event 事件 | [34-event-development.md](./34-event-development.md) | ✅ | 事件持久化 |
| 35 | Gateway 网关 | [35-gateway-development.md](./35-gateway-development.md) | ✅ | API 版本管理 |
| 36 | Graph 工作流 | [36-graph-development.md](./36-graph-development.md) | 🟡 | **执行引擎未实现** |
| 37 | Knowledge 知识库 | [37-knowledge-development.md](./37-knowledge-development.md) | ✅ | PDF 解析 |
| 39 | Planner 规划 | [39-planner-development.md](./39-planner-development.md) | 🟡 | ReAct/A2UI |
| 40 | Runner 运行器 | [40-runner-development.md](./40-runner-development.md) | ✅ | LRU 缓存清理 |
| 50 | Avatar 头像 | [50-avatar-development.md](./50-avatar-development.md) | ✅ | 裁剪功能 |
| — | Message 消息 | [message-development.md](./message-development.md) | ✅ | 消息搜索 |
| — | Admin & Auth | [admin-auth-development.md](./admin-auth-development.md) | 🟡 | **RBAC/多租户未实现** |
| — | Data Platform | [data-platform-development.md](./data-platform-development.md) | ❌ | **完全未实现** |
| — | TTS 语音 | [tts-development.md](./tts-development.md) | ❌ | **完全未实现** |

---

## P0/P1 关键缺失（必须立即修复）

| # | 模块 | 缺失 | EP |
|---|------|------|-----|
| 1 | A2A / Multi-Agent | `call_agent` 工具未注入 Agent 工具集 | EP-BIZ-05 |
| 2 | Admin & Auth | RBAC 权限 + 多租户隔离 | — |
| 3 | Graph 工作流 | 执行引擎未实现 | EP-BIZ-10 |
| 4 | Cron 定时 | 调度引擎未实现 | EP-BIZ-09 |
| 5 | Callback 回调 | 回调投递未实现 | — |
| 6 | Channel 渠道 | Webhook 接收 + 消息投递未实现 | EP-BIZ-08 |

---

## 迭代路线

### M1（当前）— 核心可用
- Chat / Agent CRUD / Team / Session / Memory L0-L3 / Knowledge / Skill / Plugin / MCP / Tool / Event / Runner

### M2 — 企业级基础
- Admin & Auth（RBAC + 多租户） / Provider 凭据加密 / Token 用量限额 / Gateway API 版本管理

### M3 — 编排增强
- A2A call_agent 注入 / Graph 执行引擎 / Cron 调度引擎 / Callback 投递 / Channel Webhook

### M4 — 智能进化
- Memory L4 知识图谱 / Agent Evolution Scanner / Evaluation 自动评估 / Planner ReAct/A2UI

### M5 — 生态扩展
- Ecosystem 市场 / CLI 工具 / CodeExecutor E2B / Data Platform / TTS

---

## 开发计划模板

每个 `xxx-development.md` 包含以下 7 个标准章节：

1. **模块定位**：一句话定位 + 代码锚点
2. **现状评估**：功能项 + 状态 + 证据
3. **差距与优化**：按优先级排列的差距
4. **开发阶段**：Phase 划分
5. **任务清单**：可执行任务 + 优先级 + EP
6. **验收标准**：检查项
7. **依赖与风险**：外部依赖和风险
8. **错误处理规格**：场景 → HTTP 状态码 → 错误码 → 前端行为

---

## 跨模块依赖图（模块 2-8）

```
                    ┌─────────────┐
                    │ 模块9       │
                    │ Provider    │
                    └──────┬──────┘
                           │ Provider/Model 列表 + 校验
              ┌────────────┼────────────────┐
              ▼            ▼                ▼
     ┌────────────┐ ┌────────────┐  ┌────────────┐
     │ 模块2      │ │ 模块5      │  │ 模块8      │
     │ Agent创建  │ │ Agent设置  │  │ Agent标题  │
     └─────┬──────┘ └─────┬──────┘  └──────┬─────┘
           │              │                │
           │ 创建 Agent   │ 设置页         │ 顶栏+预览
           ▼              │                │
     ┌────────────┐       │                │
     │ 模块3      │       │                │
     │ Agent列表  │       │                │
     └─────┬──────┘       │                │
           │              │                │
           │ 分类筛选     │                │
           ▼              │                │
     ┌────────────┐       │                │
     │ 模块4      │       │                │
     │ Agent分类  │       │                │
     └────────────┘       │                │
                          │                │
              ┌───────────┼────────────────┤
              ▼           ▼                ▼
     ┌────────────┐ ┌────────────┐  ┌────────────┐
     │ 模块6      │ │ 模块7      │  │ 模块23     │
     │ Agent文件  │ │ Agent进化  │  │ Tools      │
     └────────────┘ └─────┬──────┘  └────────────┘
                          │
                    ┌─────┼─────┐
                    ▼     ▼     ▼
              ┌──────┐ ┌──────┐ ┌──────────┐
              │模块10│ │模块12│ │ 模块17   │
              │Session│ │Memory│ │ Channel  │
              └──────┘ └──────┘ └──────────┘
```

**依赖说明**：

| 源模块 | 目标模块 | 依赖性质 |
|--------|----------|----------|
| 模块2 创建 | 模块9 Provider | 创建时需校验 Provider/Model 可用性 |
| 模块3 列表 | 模块4 分类 | 列表按分类筛选 |
| 模块5 设置 | 模块2 创建 | 复用 Provider/Model 校验 |
| 模块5 设置 | 模块6 文件 | "文件"Tab 数据源 |
| 模块5 设置 | 模块7 进化 | "进化"Tab 数据源 |
| 模块5 设置 | 模块8 标题 | 设置页顶栏组件 |
| 模块5 设置 | 模块9 Provider | 模型下拉数据源 |
| 模块5 设置 | 模块23 Tools | 工具策略 allow/deny 数据源 |
| 模块6 文件 | 模块5 设置 | 系统提示模式影响文件过滤 |
| 模块6 文件 | 模块7 进化 | 进化自动修改 SOUL.md |
| 模块6 文件 | 模块8 标题 | Prompt 预览展示 |
| 模块6 文件 | 模块9 Provider | AI 编辑需调用 LLM |
| 模块7 进化 | 模块5 设置 | 读取进化开关 |
| 模块7 进化 | 模块6 文件 | 自动演化修改 SOUL.md |
| 模块7 进化 | 模块10 Session | 指标聚合查询 |
| 模块7 进化 | 模块23 Tools | 工具成功率查询 |
| 模块7 进化 | 模块12-16 Memory | 检索质量查询 |
| 模块8 标题 | 模块5 设置 | 标签 chips 读取模式 |
| 模块8 标题 | 模块7 进化 | "进化中"标签 |
| 模块8 标题 | 模块9 Provider | 高级设置级联 |
| 模块8 标题 | 模块17 Channel | 高级设置级联 |
