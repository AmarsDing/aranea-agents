# CodeExecutor 代码执行 — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 P0–P2 + Review 修复已落地；❌ Jupyter / Workspace / Interactive（P3）
> **需求**：[32 codeexecutor.md](./32%20codeexecutor.md) · **设计**：[32 codeexecutor.design.md](./32%20codeexecutor.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **变更**：[changelog/2026-05-21-CodeExecutor-Review-Fixes.md](../changelog/2026-05-21-CodeExecutor-Review-Fixes.md) · [架构图 DocSync](../changelog/2026-05-21-CodeExecutor-DocSync-Architecture.md)
> **关联**：[20-skill-development.md](./20-skill-development.md) · [27-artifact-development.md](./27-artifact-development.md)

---

## 1. 模块定位

CodeExecutor 为 **Skill 运行时**提供安全代码执行：Local 子进程、Docker 容器隔离，以及（规划中）E2B / Container / Jupyter / Interactive / Workspace 生态。

**装配门控**：仅当 Agent 启用 Skill（`deps.SkillUC != nil`）时，`trpc_build.go` 才注入 `WithCodeExecutor`；无 Skill 的 Agent 不挂载代码执行器。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| Factory | `internal/agent/codeexecutor/factory.go` — Wire 单例 + Resolve + lazy E2B/Container |
| 能力探测 | `capabilities.go` · `docker_fallback.go` · `metrics_executor.go` |
| 项目 Executor | `executor.go` — `DockerExecutor` + 项目 `LocalExecutor`（非 Skill 主路径） |
| Skill 适配 | `internal/skill/trpc/executor.go` — `NewExecutorForAgent` |
| 产出物 | `internal/skill/trpc/artifact_executor.go` — `WrapWithArtifactSave` |
| Agent 装配 | `internal/agent/trpc_build.go` → `buildSkillDeps` → `WithCodeExecutor` |
| Wire | `cmd/admin/wire.go` — `provideCodeExecutorFactory` |
| 运维 | `docker-compose.executor.yml` — `CODE_EXECUTOR_BACKEND=docker` |
| 框架生态 | `pkg/trpc-agent-go/codeexecutor/` — local / docker / e2b / jupyter / container / Workspace |

---

## 2. 现状评估（2026-05-21）

| 项 | 状态 | 证据 |
|----|------|------|
| 项目 Executor 接口 | ✅ | `internal/agent/codeexecutor/executor.go` |
| Docker 安全执行 | ✅ | `--network none` / `--read-only` / 内存+CPU 限制 / `--rm` / OOM 137 |
| Docker 适配层 | ✅ | `dockerExecutorAdapter` → 框架 `CodeExecutor` |
| 全局后端选择 | ✅ | `Factory.Resolve` + `CODE_EXECUTOR_BACKEND` 回退 |
| Skill 默认 local 路径 | ✅ | 非 docker 时使用框架 `trpclocal.New()`（**非**项目 `LocalExecutor`） |
| Skill 工具集成 | ✅ | `buildSkillDeps` → `NewExecutorForAgent` → `WithCodeExecutor` |
| 产出物自动收集 | ✅ | `WrapWithArtifactSave` + Docker 输出目录 / framework `OutputFiles` |
| Agent 级别执行器配置 | ✅ | `CodeExecutorType` + 前端 Skill Tab + `buildSkillDeps` |
| ExecutorRegistry / Factory | ✅ | `internal/agent/codeexecutor/factory.go`（Wire 单例 + lazy E2B/Container） |
| Docker 不可用回退 | ✅ | probe 回退 + 运行时 `dockerRuntimeFallback` |
| Prometheus 全路径 | ✅ | metrics 装饰器 + blocks_total + exit code → error |
| E2B 沙箱 | 🟡 | lazy 注册；需 `E2B_API_KEY` |
| Container 执行器 | 🟡 | `codeexec_container` build tag |
| 执行器 capabilities API | ✅ | `GET /v1/monitor/code-executor-capabilities` |
| biz 类型校验 | ✅ | `ValidateCodeExecutorType` → API 400 |

---

## 3. 差距分析（剩余）

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| 1 | 产出物全路径 + OutputSpec | P2 | Local 路径收集；Workspace glob 规则 |
| 2 | Jupyter + Interactive | P3 | 需 Jupyter 服务 + 前端多轮交互 |
| 3 | WorkspaceRegistry + Session 复用 | P3 | 配合 Session 管理 |
| 4 | `NewEnvInjectingCodeExecutor` | P3 | identity 场景 per-user env |
| 5 | E2B init 失败可重试 / capabilities 动态 | P2 | `sync.Once` 失败后 capabilities 仍可能显示 available |

---

## 4. 开发阶段

### Phase 1：Agent 配置 + Registry + 观测（P1）

**目标**：Agent 粒度选择执行器；Registry 替代环境变量硬编码；metrics 全覆盖。

| # | 任务 | 涉及文件 |
|---|------|----------|
| 1.1 | `AgentRuntimeSettings.CodeExecutorType` | `internal/biz/agent_types.go` |
| 1.2 | 默认值 `"local"` | `internal/biz/agent_defaults.go` |
| 1.3 | Ent `code_executor_type` | `internal/data/ent/schema/agent_runtime_setting.go` |
| 1.4 | SQL 迁移 | `docs/sql/` |
| 1.5 | `Factory`（原 Registry 计划） | `internal/agent/codeexecutor/factory.go` |
| 1.6 | `buildSkillDeps` 优先级：Agent → env → default | `internal/agent/trpc_build.go` |
| 1.7 | `NewExecutorForAgent` + Factory 注入 | `internal/skill/trpc/executor.go` |
| 1.8 | metrics 装饰器（含 framework local） | `internal/agent/codeexecutor/metrics_executor.go` |
| 1.9 | Docker 不可用 → local 回退 | `factory.go` + `docker_fallback.go` |
| 1.10 | 前端 Agent 设置：执行器类型 | `web/.../AgentSettingsSkillsTab.vue` |
| 1.11 | 测试 | `factory_test.go` · `code_executor_test.go` |

**验收**：
- [x] `CodeExecutorType` 可配置并持久化
- [x] Factory 支持 `local` / `docker` 注册与查询
- [x] 配置优先级：Agent > `CODE_EXECUTOR_BACKEND` > `local`
- [x] Skill 路径 Prometheus 指标完整
- [x] `go test ./internal/agent/codeexecutor/...` 通过

### Phase 2：E2B + Container + 产出物增强（P2）

**目标**：扩展隔离后端；补齐 Local 产出物与 OutputSpec。

| # | 任务 | 涉及文件 |
|---|------|----------|
| 2.1 | Factory lazy 注册框架 `e2b.CodeExecutor` | `factory.go` + `E2B_API_KEY` |
| 2.2 | Factory lazy 注册框架 `container.CodeExecutor` | `factory.go` + build tag |
| 2.3 | Local 路径产出物收集 | `artifact_executor.go` / framework local |
| 2.4 | OutputSpec / glob 规则（可选配置） | `internal/agent/codeexecutor/` |
| 2.5 | 集成测试（mock E2B） | `*_test.go` |

**验收**：
- [x] Agent 可配置 `e2b` / `container`（保存校验；运行时 lazy + fallback）
- [x] E2B Key 缺失时不 panic、不 eager 建沙箱
- [x] Local 与 Docker 产出物均可落 Artifact

### Phase 3：Jupyter + Interactive（P3）

| # | 任务 | 涉及文件 |
|---|------|----------|
| 3.1 | Factory lazy 注册 `jupyter.CodeExecutor` | `factory.go` |
| 3.2 | Interactive `ProgramSession` 生命周期 | `internal/agent/codeexecutor/interactive.go`（新建） |
| 3.3 | 前端多轮交互配合 | `web/` |

### Phase 4：WorkspaceRegistry + 输入准备（P3）

| # | 任务 | 涉及文件 |
|---|------|----------|
| 4.1 | 框架 `WorkspaceRegistry` + Session 绑定 | `internal/agent/codeexecutor/` |
| 4.2 | `InputSpec` / `StageInputs` | 同上 |
| 4.3 | `CollectOutputs` + Artifact 联动 | 同上 |

---

## 5. 依赖与风险

| 依赖/风险 | 说明 | 缓解 |
|-----------|------|------|
| Docker daemon | Docker 后端需宿主机 Docker | Phase 1 回退 local + FlowLog 告警 |
| E2B API Key | 需网络与密钥 | Key 缺失时不注册或标记不可用 |
| Jupyter 服务 | 需运行中 Jupyter | 连接失败时不注册 |
| 双 Local 实现 | 项目 `LocalExecutor` vs 框架 `trpclocal` | Factory Skill 入口；项目 LocalExecutor 仅底层/单测 |
| Ent Schema 变更 | `code_executor_type` 列 | ALTER TABLE SQL |
| 红线 R2 | `internal/biz` 不 import `trpc-agent-go` | 组装在 `internal/agent` / `internal/skill` |

---

## 6. 验收标准总览

| 项 | 状态 |
|----|------|
| Local / Docker 基础执行（Skill 路径） | ✅ |
| Docker 安全约束 + OOM/超时标识 | ✅ |
| Docker 产出物 → Artifact（部分） | 🟡 |
| Agent 可配置执行器类型 | ✅ |
| Factory（Wire 单例） | ✅ |
| 生产环境**建议** Docker（非默认） | 📋 运维配置 + `AllowLocalInProd` 告警 |
| E2B / Container | 🟡 lazy + capabilities |
| Workspace + InputSpec/OutputSpec | ❌ Phase 4 |
| 执行器可用状态查询 | ✅ Monitor capabilities API |
| `go test ./internal/agent/codeexecutor/...` | ✅ |
| `make wire && make api && make build && make test` | ✅ |
