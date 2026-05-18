# CodeExecutor 代码执行 — 开发计划

> **版本**：2026-05-19 | **状态**：🟡 Docker 可用（适配层已实现）；❌ E2B/Jupyter/Interactive/Registry 未实现
> **需求**：[32 codeexecutor.md](./32%20codeexecutor.md) · **设计**：[32 codeexecutor.design.md](./32%20codeexecutor.design.md)

---

## 1. 模块定位

CodeExecutor 代码执行：提供安全的代码执行环境，支持 Local 子进程、Docker 容器、E2B 云端沙箱、Jupyter 内核和交互式执行。

**代码锚点**：
- `internal/agent/codeexecutor/executor.go` — 项目 Executor 接口 + Local/Docker 实现
- `internal/agent/codeexecutor/executor_test.go` — 基础测试
- `internal/skill/trpc/executor.go` — 适配层（项目 Executor → 框架 CodeExecutor）
- `internal/skill/trpc/tools.go` — Skill 工具集构建
- `internal/agent/trpc_build.go` — Agent 构建（buildSkillDeps 创建执行器）
- `pkg/trpc-agent-go/codeexecutor/` — 框架 CodeExecutor 完整生态

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| 项目 Executor 接口 | ✅ | `internal/agent/codeexecutor/executor.go`：`Executor` 接口 + `LocalExecutor` + `DockerExecutor` |
| Docker 安全执行 | ✅ | `DockerExecutor.Run()`：`--network none` / `--read-only` / 内存限制 / CPU 限制 / `--rm` |
| Docker 适配层 | ✅ | `internal/skill/trpc/executor.go`：`dockerExecutorAdapter` 适配为框架 `CodeExecutor` |
| 执行器后端选择 | ✅ | `NewExecutor(backend, workDir)`：环境变量 `CODE_EXECUTOR_BACKEND` 控制 |
| Prometheus 指标 | ✅ | `aranea_codeexec_runs_total` / `aranea_codeexec_duration_seconds` / `aranea_codeexec_oom_total` |
| 框架 local 执行器 | ✅ | `skilltrpc.NewLocalExecutor(workDir)` 使用框架 `trpclocal.New()` |
| Skill 工具集成 | ✅ | `trpcllmagent.WithCodeExecutor(exec)` 在 `trpc_build.go:110` |
| Agent 级别执行器配置 | ❌ | `AgentRuntimeSettings` 无 `CodeExecutorType` 字段，仅环境变量控制 |
| ExecutorRegistry | ❌ | 无注册中心，`NewExecutor` 硬编码 local/docker 分支 |
| E2B 沙箱 | ❌ | 框架 `e2b.CodeExecutor` 存在但项目未集成 |
| Jupyter 内核 | ❌ | 框架 `jupyter.CodeExecutor` 存在但项目未集成 |
| Container 执行器 | ❌ | 框架 `container.CodeExecutor` 存在但项目未集成 |
| WorkspaceRegistry | ❌ | 框架 `WorkspaceRegistry` 存在但项目未集成 |
| Interactive 模式 | ❌ | 框架 `InteractiveProgramRunner` 存在但项目未集成 |
| 产出物自动收集 | ❌ | 框架 `ArtifactService` 集成存在但项目未接入 |

---

## 3. 差距分析

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| 1 | Agent 级别执行器配置缺失 | P1 | 无法按 Agent 粒度选择执行器，仅全局环境变量 |
| 2 | ExecutorRegistry 缺失 | P1 | 执行器选择硬编码在 `NewExecutor`，无法扩展 |
| 3 | E2B 沙箱未集成 | P2 | 框架已提供完整实现，项目仅需适配层 |
| 4 | Container 执行器未集成 | P2 | 框架已提供完整实现，项目仅需适配层 |
| 5 | Jupyter 内核未集成 | P3 | 需 Jupyter 服务器依赖 |
| 6 | WorkspaceRegistry 未集成 | P3 | 需配合 Session 管理 |
| 7 | Interactive 模式未集成 | P3 | 需配合前端交互 |
| 8 | 产出物自动收集未接入 | P3 | 需配合 Artifact 服务 |

---

## 4. 开发阶段

### Phase 1：Agent 级别执行器配置 + Registry（EP-BIZ-02）

**目标**：Agent 可按配置选择执行器类型，替换环境变量硬编码方案。

**任务**：

| # | 任务 | 涉及文件 | 依赖 |
|---|------|----------|------|
| 1.1 | `AgentRuntimeSettings` 新增 `CodeExecutorType` 字段 | `internal/biz/agent_types.go` | — |
| 1.2 | 默认值设置 | `internal/biz/agent_defaults.go` | 1.1 |
| 1.3 | Ent Schema 新增 `code_executor_type` 列 | `internal/data/ent/schema/agent_runtime_setting.go` | 1.1 |
| 1.4 | SQL 迁移 | `sql/` | 1.3 |
| 1.5 | `ExecutorRegistry` 实现 | `internal/agent/codeexecutor/registry.go`（新建） | — |
| 1.6 | `trpc_build.go` 接入 Registry + Agent 配置 | `internal/agent/trpc_build.go` | 1.1, 1.5 |
| 1.7 | `skilltrpc.NewExecutor` 重构为 Registry 模式 | `internal/skill/trpc/executor.go` | 1.5 |
| 1.8 | 前端 Agent 设置页面增加执行器类型选择 | `web/` | 1.1 |
| 1.9 | 测试 | `internal/agent/codeexecutor/registry_test.go`（新建） | 1.5 |

**验收**：
- [ ] `AgentRuntimeSettings.CodeExecutorType` 可配置
- [ ] `ExecutorRegistry` 支持 local/docker 注册和查询
- [ ] `buildSkillDeps` 优先使用 Agent 配置，回退到环境变量
- [ ] `go test ./internal/agent/codeexecutor/...` 通过
- [ ] `make wire && make api && make build` 通过

### Phase 2：E2B + Container 执行器集成

**目标**：集成框架 E2B 和 Container 执行器，提供云端和容器化隔离执行。

**任务**：

| # | 任务 | 涉及文件 | 依赖 |
|---|------|----------|------|
| 2.1 | E2B 执行器适配 | `internal/agent/codeexecutor/e2b_adapter.go`（新建） | Phase 1 |
| 2.2 | Container 执行器适配 | `internal/agent/codeexecutor/container_adapter.go`（新建） | Phase 1 |
| 2.3 | Registry 注册 E2B / Container | `internal/agent/codeexecutor/registry.go` | 2.1, 2.2 |
| 2.4 | E2B 配置项（API Key 等） | 配置文件 / 环境变量 | 2.1 |
| 2.5 | Container 配置项（Docker client 等） | 配置文件 | 2.2 |
| 2.6 | 测试 | `internal/agent/codeexecutor/e2b_adapter_test.go`（新建） | 2.1 |

**验收**：
- [ ] E2B 后端可执行代码并返回结果
- [ ] Container 后端可执行代码并返回结果
- [ ] Agent 可配置 `code_executor_type: e2b` / `container`
- [ ] `go test ./internal/agent/codeexecutor/...` 通过

### Phase 3：Jupyter 内核 + Interactive 模式

**目标**：集成框架 Jupyter 执行器和 Interactive 模式，支持交互式数据分析。

**任务**：

| # | 任务 | 涉及文件 | 依赖 |
|---|------|----------|------|
| 3.1 | Jupyter 执行器适配 | `internal/agent/codeexecutor/jupyter_adapter.go`（新建） | Phase 1 |
| 3.2 | Registry 注册 Jupyter | `internal/agent/codeexecutor/registry.go` | 3.1 |
| 3.3 | Jupyter 配置项（IP/Port/Token 等） | 配置文件 | 3.1 |
| 3.4 | Interactive 模式集成 | `internal/agent/codeexecutor/interactive.go`（新建） | 3.1 |
| 3.5 | ProgramSession 生命周期管理 | `internal/agent/codeexecutor/` | 3.4 |
| 3.6 | 测试 | `internal/agent/codeexecutor/jupyter_adapter_test.go`（新建） | 3.1 |

**验收**：
- [ ] Jupyter 后端可执行代码并返回结果
- [ ] 执行环境可跨轮保持状态
- [ ] Agent 可配置 `code_executor_type: jupyter`
- [ ] `go test ./internal/agent/codeexecutor/...` 通过

### Phase 4：WorkspaceRegistry + 产出物收集

**目标**：集成框架 Workspace 生态和 Artifact 产出物自动收集。

**任务**：

| # | 任务 | 涉及文件 | 依赖 |
|---|------|----------|------|
| 4.1 | WorkspaceRegistry 集成 | `internal/agent/codeexecutor/` | Phase 1 |
| 4.2 | Session 级工作区复用 | `internal/agent/codeexecutor/` | 4.1 |
| 4.3 | ArtifactService 产出物收集 | `internal/agent/codeexecutor/` | 4.1 |
| 4.4 | InputSpec 文件准备 | `internal/agent/codeexecutor/` | 4.1 |
| 4.5 | OutputSpec 产出物收集 | `internal/agent/codeexecutor/` | 4.1 |
| 4.6 | 测试 | `internal/agent/codeexecutor/` | 4.1 |

**验收**：
- [ ] 工作区可按 Session 复用
- [ ] 执行前可准备输入文件
- [ ] 执行后可按规则收集输出文件
- [ ] 产出物自动保存为 Artifact
- [ ] `go test ./internal/agent/codeexecutor/...` 通过

---

## 5. 依赖与风险

| 依赖/风险 | 说明 | 缓解措施 |
|-----------|------|----------|
| Docker daemon | Docker 后端需宿主机 Docker 运行 | 不可用时回退到 Local + 告警日志 |
| E2B API Key | E2B 后端需 API Key 和网络访问 | Key 缺失时注册为不可用 |
| Jupyter 服务器 | Jupyter 后端需运行中的 Jupyter 服务 | 连接失败时注册为不可用 |
| Docker client 依赖 | Container 后端使用 `docker/docker/client` | 需确保依赖版本兼容 |
| Ent Schema 变更 | 新增 `code_executor_type` 列需数据库迁移 | 提供 ALTER TABLE SQL |
| 红线 R2 | `internal/biz` 不得 import `pkg/trpc-agent-go` | 适配层在 `internal/agent` / `internal/skill` |

---

## 6. 验收标准总览

- [ ] Agent 可配置执行器类型（local/docker/e2b/jupyter/container）
- [ ] ExecutorRegistry 支持多执行器注册和查询
- [ ] Skill 代码执行默认使用 Docker Sandbox（生产环境）
- [ ] E2B 沙箱可执行代码并返回结果
- [ ] Container 执行器可执行代码并返回结果
- [ ] Jupyter 内核可执行代码并保持状态
- [ ] 工作区文件正确准备和收集
- [ ] 产出物自动保存为 Artifact
- [ ] `go test ./internal/agent/codeexecutor/...` 通过
- [ ] `make wire && make api && make build && make test` 通过
