# M14: CodeExecutor 代码执行 — 详细需求

> 对标 `pkg/trpc-agent-go/codeexecutor` 包，实现安全的代码执行环境。
>
> **2026-05-17 现状对齐**：
> - ✅ `internal/agent/codeexecutor/executor.go` 已实现 Docker Sandbox 执行器（含 timeout / 资源限制 / artifact 收集 / 测试 `executor_test.go`）。
> - ❌ **skill 路径仍使用 `internal/skill/trpc/executor.go` 中的 `codeexecutor/local`**；Docker 执行器未替换为默认。
> - ❌ E2B 沙箱、Jupyter 内核、Interactive 模式、WorkspaceRegistry 仍未实现。
>
> 后续以 `guides/execution-plan.md` §3 EP-BIZ-04 + 运维要点 `guides/codeexecutor.md` 为准。

---

## 1. 现状分析（已过期，保留参考）

项目已有 `internal/skill/trpc/executor.go`，使用 `codeexecutor/local` 执行器：
- 仅支持本地执行
- 无 E2B 沙箱
- 无 Jupyter 内核
- 无 Container 隔离
- 无 Interactive 模式
- 无 WorkspaceRegistry
- 无产出物自动收集

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/codeexecutor/
├── codeexecutor.go    # CodeExecutor 接口：ExecuteCode + CodeBlockDelimiter
├── artifacts.go       # 产出物收集
├── manifest.go        # 执行清单
├── metadata.go        # 执行元数据
├── mime.go            # MIME 类型映射
├── registry.go        # WorkspaceRegistry
├── workspace.go       # 工作区管理
├── env_provider.go    # 环境变量提供者
├── interactive.go     # Interactive 模式
├── local/             # 本地执行器
│   └── local.go
├── e2b/               # E2B 沙箱执行器
│   └── e2b.go
├── jupyter/           # Jupyter 内核执行器
│   └── ...
└── container/         # 容器执行器
    └── ...
```

### CodeExecutor 接口

```go
type CodeExecutor interface {
    ExecuteCode(context.Context, CodeExecutionInput) (CodeExecutionResult, error)
    CodeBlockDelimiter() CodeBlockDelimiter
}

type CodeExecutionInput struct {
    CodeBlocks  []CodeBlock
    ExecutionID string
}

type CodeExecutionResult struct {
    Output      string
    OutputFiles []File
}
```

---

## 3. 需求清单

### 3.1 E2B 沙箱执行器

**需求**：支持 E2B 云端沙箱执行

**实现要点**：
- 集成 trpc `codeexecutor/e2b` 包
- 配置 E2B API Key
- 代码在云端沙箱中执行，结果返回

**验收标准**：代码在 E2B 沙箱中安全执行

### 3.2 Jupyter 内核执行器

**需求**：支持 Jupyter 内核执行

**实现要点**：
- 集成 trpc `codeexecutor/jupyter` 包
- 连接 Jupyter 服务器
- 支持 Interactive 模式（保持内核状态）

**验收标准**：代码在 Jupyter 内核中执行，状态可保持

### 3.3 Container 容器执行器

**需求**：支持 Docker 容器隔离执行

**实现要点**：
- 集成 trpc `codeexecutor/container` 包
- 拉取执行镜像
- 在容器中执行代码

**验收标准**：代码在 Docker 容器中隔离执行

### 3.4 WorkspaceRegistry

**需求**：管理工作区文件系统

**实现要点**：
- 集成 trpc `codeexecutor/registry.go`
- 工作区文件自动管理
- 执行前准备文件，执行后收集产出物

**验收标准**：工作区文件正确准备和收集

### 3.5 Interactive 模式

**需求**：支持交互式代码执行

**实现要点**：
- 集成 trpc `codeexecutor/interactive.go`
- 保持执行环境状态
- 支持多轮代码执行

**验收标准**：代码执行环境可跨轮保持状态

### 3.6 产出物自动收集

**需求**：代码执行产出物自动保存为 Artifact

**实现要点**：
- `CodeExecutionResult.OutputFiles` 自动保存
- 通过 ArtifactService 持久化

**验收标准**：代码执行产生的文件自动保存为 Artifact

### 3.7 执行器可配置

**需求**：Agent 级别可配置执行器类型

**实现要点**：
- 在 `AgentRuntimeSetting` 中增加 `code_executor_type` 字段
- 可选值：`local`/`e2b`/`jupyter`/`container`
- 在 `BuildTRPCLLMAgent` 中根据配置选择执行器

**验收标准**：不同 Agent 可配置不同的代码执行器

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/skill/trpc/executor.go` | 修改 | 扩展支持多种执行器 |
| `internal/codeexecutor/trpc/e2b.go` | 新建 | E2B 执行器适配 |
| `internal/codeexecutor/trpc/jupyter.go` | 新建 | Jupyter 执行器适配 |
| `internal/codeexecutor/trpc/container.go` | 新建 | Container 执行器适配 |
| `internal/agent/trpc_build.go` | 修改 | 根据配置选择执行器 |
| `internal/biz/agent_types.go` | 修改 | 增加 code_executor_type 字段 |

---

## 5. 验收标准总览

1. 支持 Local/E2B/Jupyter/Container 四种执行器
2. 代码在沙箱中安全执行
3. 工作区文件正确准备和收集
4. 交互式执行环境可跨轮保持状态
5. 产出物自动保存为 Artifact
6. 执行器类型可在 Agent 设置中配置
