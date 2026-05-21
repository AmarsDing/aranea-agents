# 40 Runner Review

> **评分**：82 / 100 | **风险等级**：P1  
> **文档**：[40-runner-development.md](../需求/40-runner-development.md)  
> **代码锚点**：`internal/agent/trpc_runtime.go` · `internal/agent/trpc_build.go` · `internal/agent/builder_deps.go` · `internal/runtime/runner_registry.go` · `internal/runtime/runner_manager.go`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | RunRegistry + RunnerManager 核心已落地；`40-runner-development.md` 标注"待本轮同步"，P1/P2 验收项已通 |
| 架构一致性 | 22 | 25 | `internal/agent/trpc_build.go` 正确通过 `pkg/trpc-agent-go` 接口装配；不复制框架内部实现 ✅ |
| 后端实现质量 | 18 | 20 | `TRPCBuilderDeps` + `TRPC*Deps` 分组类型 ✅；`system.agent.build` FlowLog ✅；Builder 汇聚 Provider/Tool/Skill/Memory/Callback |
| 前端实现质量 | 13 | 15 | RunStatus WS 驱动 ✅；Cancel 路径完整；GetRunStatus HTTP 快照一次 |
| 测试与验证 | 6 | 10 | Builder 路径有单测；Runner 生命周期（Start→Run→Cancel）集成测试待补 |
| 文档一致性 | 6 | 10 | `40-runner-development.md` 标注待同步；开发计划条目已通 |

---

## 模块定位

Runner 模块是 trpc-agent-go 框架的装配与生命周期管理层：
- `BuildTRPCLLMAgent`：从 `biz.Agent` 行构建 LLM Agent（Provider + Tools + MCP + Skill + Planner + Memory + Plugin）
- `NewTRPCRunner`：创建 ManagedRunner / SteerableRunner（含 Session / Memory / Plugin 注入）
- `RunRegistry`：会话级运行控制（cancel/status/enqueue）
- `RunnerManager`：统一 Runner 装配，注入 ArtifactService / SessionIngestor / AgentFactory / AwaitUserReplyRouting

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| BuildTRPCLLMAgentCached | ✅ |
| TRPCBuilderDeps 分组类型 | ✅ I8 |
| system.agent.build FlowLog | ✅ I8 |
| RunRegistry（cancel/status/enqueue/artifact/ingest） | ✅ M1 |
| RunnerManager（ArtifactService/SessionIngestor 注入） | ✅ M1 |
| SteerableRunner（Follow-up Queue） | ✅ |
| AwaitUserReplyRouting 注入 | ✅ |
| RalphLoop | 🟡 OpenClaw 侧实现，Aranea 侧待对齐 |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| RNDR-P1-01 | `40-runner-development.md` 开发计划待同步（文档注明"待本轮同步"） | 本迭代补全验收项与代码状态对照 |
| RNDR-P1-02 | Runner 生命周期（Start→Run→Cancel→终态写库）缺乏完整集成测试 | 补 Runner 生命周期测试 |
| RNDR-P1-03 | RalphLoop（自进化评估循环）当前仅 OpenClaw 侧有实现，Aranea 侧未对齐 | 规划 RalphLoop 接入或文档降级 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| RNDR-P2-01 | Builder 编译面较大（汇聚大量依赖），修改任一依赖需完整重新装配 | 引入 Catalog DTO 稳定 Builder 依赖 |
| RNDR-P2-02 | `ManagedRunner.Cancel` 终态写 RunRegistry 路径需再次确认一致性 | 统一 Cancel → publishRunStatus(cancelled) 路径 |

---

## 工具挂载链验收

```
biz.Agent + RuntimeSettings
    → BuildTRPCLLMAgent
        ├─ provider.TRPCModelForProviderModel
        ├─ agent/planner.Select
        ├─ WithSkills + CodeExecutor
        ├─ loadEffectiveToolKeys → BuildToolsets
        │   ├─ Builtin (file/hostexec/webfetch/search/email/todo)
        │   ├─ MCP ToolSet / Broker
        │   ├─ Knowledge search tool
        │   ├─ call_agent (A2A)
        │   └─ await_user_reply
        ├─ Memory tools (when MemoryService enabled)
        └─ Callback chain (Tool/retry/filter)
    → trpc llmagent.New
```

**状态**：完整装配链路已可用 ✅

---

## 建议优化路径

1. 同步 `40-runner-development.md` 文档（标注已通验收项）。
2. 补 Runner 生命周期集成测试。
3. 规划 RalphLoop 接入方案。
