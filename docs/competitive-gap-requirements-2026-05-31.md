# Aranea-Agents 需求提升清单

> 基于 2026-05-31 竞品对比分析，对标 OpenClaw / Hermes Agent / Human-Agent。
> 与 `docs/需求/00-总览与路线图.md` 的五阶段路线图互补，本文档聚焦于竞品差距驱动的具体需求项。

---

## 一、需求总览

| 优先级 | 需求数 | 核心目标 |
|--------|--------|----------|
| 🔴 P0 | 3 | 补齐 Skill 生态与浏览器工具，对标 OpenClaw 核心能力 |
| 🟠 P1 | 5 | 增强运行时能力（Profile/Delivery/Outbound/Cron/Persona），缩小与 OpenClaw/Hermes 差距 |
| 🟡 P2 | 6 | 进化与差异化（学习闭环/Skill 市场/Debug/Langfuse/Memory 可见性），超越竞品 |
| **合计** | **14** | |

---

## 二、P0 需求（🔴 核心能力补齐）

### P0-1：内置 Skills 库 + Eligibility 评估

**对标**：OpenClaw 60+ 内置 Skills + `evaluateSkill()` 五维检查

**现状差距**：
- Aranea 无内置 Skills 目录，OpenClaw 有 60+ 内置 Skills（1password/discord/github/notion/slack/spotify 等）
- Aranea 无 Skill eligibility 评估，OpenClaw 有 OS/bins/env/config 五维检查
- Aranea 无 SkillConfig（per-skill API Key/环境变量注入），OpenClaw 有完整 SkillConfig 系统
- Aranea 的 YAML 解析使用手工 `strings.Split`，不支持嵌套结构；OpenClaw 使用标准 `gopkg.in/yaml.v3`

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 移植 OpenClaw 内置 Skills | 从 `pkg/trpc-agent-go/openclaw/skills/` 移植 60+ Skills 到项目 | `internal/skill/builtin/`（新建） |
| 2 | 实现 Eligibility 评估 | 移植 `openclaw/internal/skills/repository.go` 的 `evaluateSkill()` | `internal/skill/trpc/eligibility.go`（新建） |
| 3 | 实现 SkillConfig | 移植 SkillConfig 结构 + API Key/Env 注入 + 安全黑名单 | `internal/skill/trpc/skillconfig.go`（新建） |
| 4 | 升级 YAML 解析 | 从手工 `strings.Split` 迁移到 `gopkg.in/yaml.v3` | `internal/skill/manifest/manifest.go` |
| 5 | 实现状态报告 | 移植 `openclaw/internal/skills/status.go` 的 StatusReport | `internal/skill/trpc/status.go`（新建） |

**验收标准**：
- [ ] 项目包含 30+ 内置 Skills
- [ ] Skill eligibility 自动评估（OS/bins/env/config）
- [ ] per-skill API Key 配置和注入
- [ ] Skill 状态报告可查询

---

### P0-2：浏览器工具实现

**对标**：OpenClaw 25+ action + 双驱动 + 导航安全策略

**现状差距**：
- Aranea 的 `internal/tools/browser/config.go` 仅 60 行，只定义 `PlaywrightMCPConfig` 结构体
- OpenClaw 的 `openclaw/internal/browser/tool.go` 约 2219 行，实现 25+ action + 双驱动 + 导航安全
- Aranea 完全依赖框架 MCP toolset，自身不实现任何浏览器操作逻辑

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 集成 OpenClaw 浏览器工具 | 移植 `openclaw/internal/browser/` 的 tool/driver/navigation_guard | `internal/tools/browser/` |
| 2 | 实现 MCP Profile Driver | 复用框架 `mcptool.ToolSet` 连接 Playwright MCP | `internal/tools/browser/mcp_driver.go` |
| 3 | 实现 Browser Server Driver | HTTP REST API 调用远程 browser-server | `internal/tools/browser/server_driver.go` |
| 4 | 实现导航安全策略 | domain allowlist/blocklist + loopback/private-net 限制 | `internal/tools/browser/navigation_guard.go` |
| 5 | 实现 Profile 管理 | 多浏览器 profile 支持 | `internal/tools/browser/profiles.go` |

**验收标准**：
- [ ] 支持 15+ 浏览器 action（open/navigate/screenshot/click/type/scroll 等）
- [ ] 导航安全策略生效（禁止访问内网/本地文件）
- [ ] 双驱动模式可用（MCP + Browser Server）

---

### P0-3：Skill Eligibility + SkillConfig 系统

**对标**：OpenClaw `evaluateSkill()` + SkillConfig

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 五维 Eligibility 检查 | OS/bins/anyBins/env/config | `internal/skill/trpc/eligibility.go` |
| 2 | SkillConfig 结构 | Enabled/APIKey/Env + 安全黑名单 | `internal/skill/trpc/skillconfig.go` |
| 3 | Eligibility 集成到 Skill 加载 | 在 `Repository()` 和 `Assemble()` 中调用 | `internal/skill/trpc/filter.go`, `internal/tools/trpc/toolsets.go` |

**验收标准**：
- [ ] Skill 加载时自动评估可用性
- [ ] 不可用的 Skill 在 UI 中显示原因和安装提示
- [ ] per-skill API Key 可配置和注入

---

## 三、P1 需求（🟠 运行时能力增强）

### P1-1：Runtime Profile 系统

**对标**：OpenClaw `openclaw/runtimeprofile/`（~815 行）

**现状差距**：Aranea 无独立 Runtime Profile 包，使用 `biz.AgentRuntimeSetting`（静态配置）。OpenClaw 有完整 per-request 配置系统 + 三个策略执行器 + 三级隔离模式。

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 移植 Profile 结构 | ID/Version/AppName/AgentName/ModelName/Prompt/Tools/Knowledge/Workspace/Credentials/Skills/Isolation | `internal/runtimeprofile/`（新建） |
| 2 | 实现策略执行 | ResolveWorkdir + CheckCredentialRef + SkillVisibilityFilter | `internal/runtimeprofile/policy.go` |
| 3 | 实现隔离模式 | Shared/ProfileCache/Service 三级 | `internal/runtimeprofile/isolation.go` |
| 4 | 实现 CachedResolver | 热重载 Profile | `internal/runtimeprofile/resolver.go` |
| 5 | 集成到 Agent 构建 | Profile → `agent.RunOption` 转换 | `internal/agent/trpc_build.go` |
| 6 | 前端 Profile 管理 | Profile CRUD + 切换 UI | `web/src/components/agents/AgentRuntimeProfileSection.vue`（新建） |

**验收标准**：
- [ ] 可创建/编辑/切换 Runtime Profile
- [ ] Workspace 路径校验生效
- [ ] Skill visibility filter 按 Profile 过滤
- [ ] 前端可管理 Profile

---

### P1-2：SubAgent Delivery 通知

**对标**：OpenClaw `notifyCompletion()` + outbound delivery

**现状差距**：Aranea 的子 Agent 完成后只能通过 `get` 工具轮询结果，无主动通知。

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | SpawnRequest 增加 Delivery 字段 | Channel/Target | `internal/tools/subagent/service.go` |
| 2 | 实现 notifyCompletion | 子 Agent 完成后通过 outbound.Router 发送通知 | `internal/tools/subagent/service.go` |
| 3 | 集成 outbound.ResolveTarget | 自动解析 delivery 目标 | `internal/tools/subagent/service.go` |
| 4 | runtime state 注入 | delivery 的 runtime state 注入子 Agent 运行时 | `internal/tools/subagent/service.go` |

**验收标准**：
- [ ] 子 Agent 完成后自动发送通知到指定渠道
- [ ] spawn 时可指定 delivery 目标
- [ ] 通知消息包含执行结果摘要

---

### P1-3：Outbound Router 增强

**对标**：OpenClaw voice/media/glob/opaque ref + sentTextRecorder

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 增加 voice/media 参数 | as_voice/audio_as_voice 支持 | `internal/outbound/tool.go` |
| 2 | 文件 glob 展开 | expandOutboundGlob | `internal/outbound/tool.go` |
| 3 | 目录展开 | expandOutboundDirectory（max 32 文件） | `internal/outbound/tool.go` |
| 4 | Opaque ref 支持 | host://artifact://workspace:// | `internal/outbound/tool.go` |
| 5 | sentTextRecorder 去重 | 避免重复发送 | `internal/outbound/tool.go` |
| 6 | 自动注册 | `Register(ch)` 自动类型断言 | `internal/outbound/adapter.go` |

**验收标准**：
- [ ] Agent 可发送语音消息
- [ ] Agent 可通过 glob 模式批量发送文件
- [ ] Agent 可引用工作空间文件（workspace://）
- [ ] 重复消息自动去重

---

### P1-4：Cron 调度增强

**对标**：OpenClaw 4 种调度 + ExecutionPolicy + delivery + 模板渲染

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 支持 4 种调度类型 | at/after/every/cron_expr + timezone | `internal/cronrunner/schedule.go`（新建） |
| 2 | ExecutionPolicy | maxRuns/endsAt/overlapPolicy | `internal/cronrunner/policy.go`（新建） |
| 3 | Outbound delivery | cron 执行结果通过 outbound.Router 发送 | `internal/cronrunner/runner.go` |
| 4 | 模板渲染 | `text/template`（RunIndex/MaxRuns/RemainingRuns/IsFinalRun） | `internal/cronrunner/template.go`（新建） |
| 5 | 前端调度配置 UI | 可视化配置 at/after/every/cron | `web/src/components/cron/CronScheduleEditor.vue`（新建） |

**验收标准**：
- [ ] 支持 `cron "0 9 * * *"` 表达式
- [ ] 支持 `at "2026-06-01T09:00:00+08:00"` 一次性调度
- [ ] 支持 maxRuns 限制执行次数
- [ ] cron 结果自动发送到指定渠道

---

### P1-5：Persona 预设系统

**对标**：OpenClaw 5 预设角色 + scope-based 隔离 + alias 切换

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 预设角色模板 | Default/Concise/Coach/Creative 等 | `internal/agent/persona/presets.go`（新建） |
| 2 | Scope-based 隔离 | DM scope / Thread scope | `internal/agent/persona/store.go`（新建） |
| 3 | Alias 快捷切换 | gf→girlfriend, none→default 等 | `internal/agent/persona/alias.go`（新建） |
| 4 | 前端 Persona 管理 | 预设选择 + 自定义编辑 | `web/src/components/agents/AgentPersonaSection.vue`（新建） |

**验收标准**：
- [ ] 用户可在 UI 中切换 Agent 角色
- [ ] 不同对话 scope 可使用不同 persona
- [ ] 支持 alias 快捷切换

---

## 四、P2 需求（🟡 进化与差异化）

### P2-1：学习闭环打通

**对标**：Hermes Learning Loop

**现状**：后端核心已完成（`internal/biz/evolution*.go`），缺 Proto/Service 层和前端。

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | Proto 定义 | EvolutionService proto | `api/kratos/evolution/v1/` |
| 2 | Service 层 | EvolutionService 实现 | `internal/service/evolution.go` |
| 3 | 前端学习闭环面板 | 进化建议列表 + 自动应用 + 节流 | `web/src/components/agents/AgentEvolutionPanel.vue` |

**验收标准**：
- [ ] Agent 可从交互中自动提取进化建议
- [ ] 用户可查看/批准/拒绝进化建议
- [ ] 进化建议可自动应用（带节流）

---

### P2-2：Skill 自创建

**对标**：Hermes Skill Self-Creation

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 技能自创建引擎 | 复杂任务后自动生成 SKILL.md | `internal/skill/autocreate/`（新建） |
| 2 | 技能自改进 | 使用中自动优化 Skill 内容 | `internal/skill/autocreate/improver.go` |
| 3 | 前端自创建管理 | 自创建技能列表 + 编辑 + 发布 | `web/src/components/skills/SkillAutoCreatePanel.vue` |

**验收标准**：
- [ ] Agent 完成复杂任务后自动生成可复用 Skill
- [ ] 生成的 Skill 可在后续对话中使用
- [ ] Skill 在使用中自动改进

---

### P2-3：Skill 市场

**对标**：Hermes agentskills.io / OpenClaw 60+ Skills

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | Skill 市场后端 | Skill 发布/搜索/安装/评分 | `internal/skill/marketplace/`（新建） |
| 2 | agentskills.io 兼容 | 支持 agentskills.io 开放标准 | `internal/skill/marketplace/agentskills.go` |
| 3 | 前端 Skill 商店 | 浏览/搜索/安装/卸载 | `web/src/pages/EcosystemPage.vue` 扩展 |

**验收标准**：
- [ ] 用户可浏览和安装社区 Skills
- [ ] 支持 agentskills.io 格式导入
- [ ] 安装的 Skill 自动通过 eligibility 评估

---

### P2-4：Debug Recorder

**对标**：OpenClaw `openclaw/internal/debugrecorder/`

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 移植 debugrecorder | 请求链路录制 + 回放 | `internal/debug/recorder.go`（新建） |
| 2 | 集成到 Chat 流程 | 在 TurnPipeline 中注入 recorder | `internal/service/turn_pipeline.go` |
| 3 | 前端调试面板 | 录制列表 + 回放 + 对比 | `web/src/components/monitor/DebugRecorderPanel.vue`（新建） |

**验收标准**：
- [ ] 可录制完整请求链路
- [ ] 可回放录制内容
- [ ] 可对比两次录制的差异

---

### P2-5：Langfuse 集成

**对标**：OpenClaw `openclaw/app/langfuse.go`

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | 移植 Langfuse 集成 | Trace + Span + Generation 上报 | `internal/telemetry/langfuse.go`（新建） |
| 2 | 集成到 Agent 运行时 | 在 Runner/Agent 层注入 Langfuse callback | `internal/agent/callback_chain.go` |
| 3 | 前端 Langfuse 配置 | API Key + Host + Project 配置 | `web/src/pages/SystemSettingsPage.vue` 扩展 |

**验收标准**：
- [ ] Agent 运行 trace 自动上报 Langfuse
- [ ] 可在 Langfuse UI 中查看完整调用链
- [ ] 支持 Token 用量和成本追踪

---

### P2-6：Memory 用户可见性

**对标**：OpenClaw MEMORY.md + Hermes FTS5 可搜索

**需求项**：

| # | 需求 | 实现方式 | 涉及文件 |
|---|------|---------|---------|
| 1 | MEMORY.md 导出 | 将 L1 事实 + L2 情景导出为 Markdown | `internal/memory/export.go`（新建） |
| 2 | MEMORY.md 编辑 | 用户可直接编辑 MEMORY.md 并回写 | `internal/memory/import.go`（新建） |
| 3 | GDPR 式删除 | DeleteUserData 按用户清除所有记忆 | `internal/memory/purge.go`（新建） |
| 4 | 前端 Memory 编辑器 | 可视化编辑 + Markdown 预览 | `web/src/components/memory/MemoryEditorPanel.vue`（新建） |

**验收标准**：
- [ ] 用户可导出 MEMORY.md
- [ ] 用户可编辑 MEMORY.md 并回写
- [ ] 用户可一键清除所有记忆数据

---

## 五、与现有路线图的映射

本清单与 `docs/需求/00-总览与路线图.md` 的五阶段路线图映射如下：

| 路线图阶段 | 本清单对应需求 |
|-----------|--------------|
| Phase 1：补齐框架能力缺口 | ✅ 已完成（RalphLoop/Guardrail/Evaluation/多模式/FileTool/WebFetch/Artifact） |
| Phase 2：增强自主性 | ✅ 已完成（Planner/ClaudeCode/浏览器/SubAgent/Outbound） |
| Phase 3：进化能力 | P0-1(Skill库), P1-5(Persona), P2-1(学习闭环), P2-2(Skill自创建), P2-3(Skill市场) |
| Phase 4：生产级增强 | P1-4(Cron), P2-4(Debug), P2-5(Langfuse), P2-6(Memory可见性) |
| Phase 5：差异化创新 | P0-2(浏览器), P1-1(Runtime Profile), P1-2(SubAgent Delivery), P1-3(Outbound增强) |

**新增需求**（路线图中未覆盖）：

| # | 需求 | 说明 |
|---|------|------|
| P0-1 | 内置 Skills 库 + Eligibility | 路线图提到 Skill 市场但未覆盖内置库和 eligibility |
| P0-3 | SkillConfig 系统 | 路线图未覆盖 per-skill 配置 |
| P1-2 | SubAgent Delivery | 路线图提到 SubAgent 但未覆盖 delivery 通知 |
| P1-3 | Outbound 增强 | 路线图提到 Outbound 但未覆盖 voice/media/glob |
| P2-6 | Memory 用户可见性 | 路线图未覆盖 MEMORY.md 导出/编辑 |

---

## 六、开发顺序建议

```
Sprint 1（P0）:  Skill Eligibility + 内置 Skills 库 + 浏览器工具
    ↓
Sprint 2（P1 上半）: Runtime Profile + SubAgent Delivery
    ↓
Sprint 3（P1 下半）: Outbound 增强 + Cron 增强 + Persona 预设
    ↓
Sprint 4（P2 上半）: 学习闭环打通 + Skill 自创建
    ↓
Sprint 5（P2 下半）: Skill 市场 + Debug Recorder + Langfuse + Memory 可见性
```

**关键依赖**：
- P1-2（SubAgent Delivery）依赖 P1-3（Outbound 增强）的 outbound.Router
- P1-4（Cron Delivery）依赖 P1-3（Outbound 增强）的 outbound.Router
- P2-2（Skill 自创建）依赖 P0-1（内置 Skills 库）的 Skill 体系
- P2-3（Skill 市场）依赖 P0-1（内置 Skills 库）+ P2-2（Skill 自创建）

---

## 七、Human-Agent 达标度提升路径

| Human-Agent 特征 | 当前达标度 | 补齐需求 | 预期达标度 |
|------------------|-----------|---------|-----------|
| 自主规划 | 部分 | P1-1(Runtime Profile 场景切换) | 基本达标 |
| 工具使用 | 基本达标 | P0-1(Skill库) + P0-2(浏览器) | 完全达标 |
| 记忆积累 | 完全达标 | P2-6(Memory可见性) | 完全达标+ |
| 自我纠错 | 部分 | P2-1(学习闭环) | 基本达标 |
| 协作能力 | 完全达标 | — | 完全达标+ |
| 持续进化 | 部分 | P2-1(学习闭环) + P2-2(Skill自创建) | 基本达标 |

**综合达标度提升路径**：~60% → Sprint 1 后 ~70% → Sprint 3 后 ~80% → Sprint 5 后 ~90%
