# M14: CodeExecutor 代码执行 — 需求文档

> **模块**：CodeExecutor 代码执行
> **对标**：`pkg/trpc-agent-go/codeexecutor` 包
> **设计**：[32 codeexecutor.design.md](./32%20codeexecutor.design.md)
> **开发计划**：[32-codeexecutor-development.md](./32-codeexecutor-development.md)

---

## 1. 模块定位

CodeExecutor 为 Agent（经 **Skill 工具链**）提供安全的代码执行环境，使模型生成的代码可在隔离沙箱中运行并返回结果。支持多种执行后端（本地子进程、Docker 容器、E2B 云端沙箱、Container 引擎、Jupyter 内核），覆盖从开发调试到生产隔离的全场景需求。

---

## 2. 用户故事

### US-1：安全代码执行

**作为** Agent 运维者，
**我希望** 模型生成的代码在隔离沙箱中执行，
**以便** 恶意或异常代码不会影响宿主系统。

### US-2：多后端选择

**作为** Agent 配置者，
**我希望** 为不同 Agent 选择不同的代码执行后端，
**以便** 开发环境用轻量本地执行，生产环境用 Docker/E2B 隔离执行。

### US-3：交互式代码执行

**作为** 数据分析用户，
**我希望** 代码执行环境可跨轮保持状态（变量、已导入模块等），
**以便** 进行多轮交互式数据分析而无需每次重新初始化。

### US-4：产出物自动收集

**作为** Agent 使用者，
**我希望** 代码执行产生的文件（图表、数据等）自动保存为 Artifact，
**以便** 在对话中引用和下载执行产出物。

### US-5：工作区文件管理

**作为** Agent 运维者，
**我希望** 代码执行前可准备输入文件、执行后可收集输出文件，
**以便** Agent 能处理需要文件输入输出的复杂任务。

### US-6：执行器状态可观测

**作为** 运维人员，
**我希望** 查看各执行器的可用状态和执行历史，
**以便** 监控系统健康和排查问题。

---

## 3. 功能规格

### 3.1 执行后端

系统应支持以下执行后端，按安全级别递增排列：

| 后端类型 | 隔离级别 | 网络访问 | 适用场景 | 当前接入 |
|----------|----------|----------|----------|----------|
| `local` | 无隔离（子进程） | 有 | 开发调试 | ✅ Skill 默认（框架 `trpclocal`） |
| `docker` | 容器隔离 | 无（`--network none`） | 生产环境 | ✅ 项目 `DockerExecutor` + 适配层 |
| `e2b` | 云端沙箱 | 受控 | 云端隔离执行 | 🟡 lazy + `E2B_API_KEY` |
| `container` | 容器引擎 | 可配置 | 框架 Container 后端 | 🟡 lazy + build tag |
| `jupyter` | 内核隔离 | 有 | 交互式数据分析 | ❌ Phase 3 |

### 3.2 支持的语言

| 语言 | 标识名 |
|------|--------|
| Python | `python` / `python3` |
| JavaScript | `javascript` / `js` / `node` |
| Bash | `bash` / `sh` / `shell` |
| Ruby | `ruby` |

### 3.3 执行安全

Docker 后端必须满足以下安全约束：

| 约束 | 说明 |
|------|------|
| 无网络 | 容器禁止网络访问 |
| 只读根文件系统 | 容器根文件系统只读 |
| 内存上限 | 硬限制容器内存使用 |
| CPU 上限 | 限制容器 CPU 配额 |
| 超时控制 | 执行超时自动终止 |
| 临时容器 | 执行完毕自动移除容器 |

### 3.4 Agent 级别执行器配置

- Agent 可独立配置代码执行后端类型（**已实现**：`code_executor_type` + 前端 Skill Tab）
- 未配置时使用系统默认后端 `local`
- 配置项：`code_executor_type`，可选值 `local` / `docker` / `e2b` / `container`（`jupyter` 待 Phase 3）
- 配置优先级：Agent 字段 > 环境变量 `CODE_EXECUTOR_BACKEND` > `local`
- biz 校验非法类型返回 400

### 3.5 交互式执行

- 执行环境可跨多轮对话保持状态（**待 Phase 3**）
- 支持向运行中的程序发送输入
- 支持查询运行中程序的输出
- 支持终止运行中的程序

### 3.6 工作区管理

- 执行前可准备输入文件到工作区（**待 Phase 4**）
- 支持从 Artifact、宿主路径、工作区相对路径导入文件
- 执行后可按 glob 模式收集输出文件
- 工作区可按 Session 复用

### 3.7 产出物收集

- 执行输出目录中的文件可收集并保存为 Artifact（**已实现**：`WrapWithArtifactSave` 装饰器覆盖所有后端路径）
- 产出物自动保存为 Artifact（经 `SaveArtifactHelper`，需 Runner 已注入 ArtifactService）
- 单文件大小限制：`DefaultMaxOutputFileBytes`（10 MiB）+ `MaxUploadBytes`（10 MiB）（**已实现**）
- 支持配置产出物收集规则（glob 模式、文件数量限制）（**待 Phase 3–4**）

### 3.8 可观测性

- 执行次数、状态（成功/失败/超时/OOM）计数 — Skill 全路径经 `metrics_executor` 上报 Prometheus
- 块级计数 — `aranea_codeexec_blocks_total`
- 执行时长直方图 — `aranea_codeexec_duration_seconds`
- OOM Kill 事件计数
- 执行器可用状态查询 — **已实现** `GET /v1/monitor/code-executor-capabilities`

---

## 4. 验收标准

> 实现进度以 [32-codeexecutor-development.md](./32-codeexecutor-development.md) 为准。

### 4.1 基础执行

- [x] 代码可在 Local 后端执行并返回 stdout/stderr（Skill + 框架 `trpclocal`）
- [x] 代码可在 Docker 后端隔离执行并返回 stdout/stderr
- [x] 不支持的语言返回明确错误
- [x] 执行超时返回超时标识

### 4.2 Docker 安全

- [x] Docker 执行的容器无网络访问
- [x] Docker 执行的容器根文件系统只读
- [x] 内存超限返回 OOM 标识（exit 137）
- [x] 执行完毕容器自动移除

### 4.3 多后端支持

- [x] E2B 后端可执行代码（lazy 注册；需 `E2B_API_KEY`；Key 缺失时 fallback local）
- [ ] Jupyter 后端可执行代码并返回结果
- [x] Container 后端可执行代码（build tag `codeexec_container`；默认 stub）

### 4.4 Agent 配置

- [x] Agent 可配置执行后端类型
- [x] 未配置时使用系统默认后端（`local`）
- [x] 配置变更即时生效（新 Agent 构建 / 新会话）

### 4.5 交互式执行

- [ ] 执行环境可跨轮保持状态
- [ ] 可向运行中程序发送输入
- [ ] 可终止运行中程序

### 4.6 工作区与产出物

- [ ] 执行前可准备输入文件
- [ ] 执行后可按规则收集输出文件
- [x] 所有后端路径产出物可自动保存为 Artifact（`WrapWithArtifactSave` 覆盖 local/docker/e2b/container）

### 4.7 可观测性

- [x] Prometheus 指标正确上报（Skill 全路径：local / docker / e2b / container）
- [x] Skill 默认 local 路径指标覆盖
- [x] 执行器可用状态可查询（Monitor capabilities API）
