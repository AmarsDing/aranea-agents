# 技能（Skill）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/skill/` + `pkg/trpc-agent-go/tool/skill/`
> 项目实现路径：`internal/biz/skill/`、`internal/data/skill*.go`、`internal/service/skill*.go`、`internal/skill/trpc/`、`internal/tools/skillruntime/`、`internal/agent/skill_guidance_inject.go`
> 当前对齐度：★★★★☆

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `skill.Repository` | `Summaries() []Summary` | 返回所有技能摘要列表 |
| `skill.Repository` | `Get(name string) (*Skill, error)` | 按名称获取完整技能 |
| `skill.Repository` | `Path(name string) (string, error)` | 获取技能的文件系统路径 |
| `skill.RootedRepository` | `Roots() []string` | 返回所有技能根目录（嵌入 Repository） |
| `skill.RefreshableRepository` | `Refresh() error` | 重新扫描文件系统，更新索引（嵌入 Repository） |
| `skill.ContextRepository` | `SummariesForContext(ctx) []Summary` | 按上下文过滤的技能摘要 |
| `skill.ContextRepository` | `GetForContext(ctx, name) (*Skill, error)` | 按上下文过滤获取技能 |
| `skill.ContextRepository` | `PathForContext(ctx, name) (string, error)` | 按上下文过滤获取路径 |
| `SkillStager` | `StageSkill(ctx, SkillStageRequest) (SkillStageResult, error)` | 技能运行前暂存到工作区 |
| `SkillRunEnvProvider` | `SkillRunEnv(ctx, skillName) (map[string]string, error)` | 为技能运行注入环境变量 |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `skill.Summary` | 技能摘要（Name + Description），三层模型第一层 |
| `skill.Skill` | 完整技能（嵌入 Summary + Body + Docs），三层模型第二/三层 |
| `skill.Doc` | 技能附加文档（Path + Content） |
| `skill.FSRepository` | 文件系统仓库实现，支持多根 + URL 根解析 + SHA256 缓存 |
| `skill.VisibilityFilter` | `func(ctx, Summary) bool`，上下文感知的可见性过滤函数 |
| `RunOutputLimits` | 技能运行输出限制（StdoutStderrBytes + PrimaryOutputBytes） |
| `SkillStageRequest` | 暂存请求（SkillName + Repository + Engine + Workspace） |
| `SkillStageResult` | 暂存结果（WorkspaceSkillDir） |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| 实现 `skill.Repository` | 接口实现 | 自定义技能存储后端（如 DB） |
| 实现 `skill.RootedRepository` | 接口实现 | 需要暴露根目录列表的仓库 |
| 实现 `skill.RefreshableRepository` | 接口实现 | 需要刷新索引的仓库 |
| 实现 `skill.ContextRepository` | 接口实现 | 需要上下文感知过滤的仓库 |
| `skill.VisibilityFilter` | 函数类型 | 按上下文过滤技能可见性 |
| `SkillStager` | 接口实现 | 自定义技能暂存行为 |
| `SkillRunEnvProvider` | 接口实现 | 自定义技能运行环境变量注入 |

### 1.4 配置选项

#### LLMAgent 级别 Option（`agent/llmagent/option.go`）

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithSkills(repo)` | 启用技能系统，自动注册工具 | 无（不启用） |
| `WithSkillFilter(filter)` | 设置上下文感知的可见性过滤器 | 无 |
| `WithSkillToolProfile(profile)` | 工具配置：`KnowledgeOnly` vs `Full` | `Full` |
| `WithAllowedSkillTools(tools...)` | 细粒度工具白名单 | 全部 |
| `WithSkillLoadMode(mode)` | 加载模式：`turn`/`once`/`session` | `turn` |
| `WithMaxLoadedSkills(max)` | LRU 淘汰上限 | 无限制 |
| `WithSkillsLoadedContentInToolResults(enable)` | 提示缓存优化 | false（系统消息模式） |
| `WithSkillsToolingGuidance(text)` | 自定义工具指导块 | 框架默认 |
| `WithSkillsCapabilityGuidance(text)` | 自定义能力指导 | 框架默认 |
| `WithSkillsProtocolGuidance(text)` | 自定义协议指导 | 框架默认 |
| `WithSkillsDirectoryHints(enable)` | 目录提示开关 | true |
| `WithSkillsFilePathHints(enable)` | 文件路径提示开关 | true |
| `WithSkillLoadToolDescription(desc)` | 自定义 skill_load 工具描述 | 框架默认 |
| `WithSkillRunAllowedCommands(cmds...)` | 允许的命令白名单 | 无限制 |
| `WithSkillRunDeniedCommands(cmds...)` | 禁止的命令黑名单 | 无 |
| `WithSkillRunOutputLimits(limits)` | 输出大小限制 | 无限制 |
| `WithSkillRunForceSaveArtifacts(enable)` | 强制保存产物 | false |
| `WithSkillRunRequireSkillLoaded(enable)` | 要求技能必须先加载才能运行 | false |
| `WithSkillRunStager(stager)` | 自定义技能暂存器 | 默认 copySkillStager |
| `WithWorkspaceExecSurfaceEnabled(enable)` | 启用工作区执行表面 | false |
| `WithToolActivationOnSkillLoad()` | 技能加载时自动激活关联工具 | 无 |

#### RunTool Option（`tool/skill/run.go`）

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithAllowedCommands(cmds...)` | 允许的命令白名单 | 无限制 |
| `WithDeniedCommands(cmds...)` | 禁止的命令黑名单 | 无 |
| `WithForceSaveArtifacts(enable)` | 强制保存产物 | false |
| `WithRunOutputLimits(limits)` | 输出限制 | 无限制 |
| `WithRequireSkillLoaded(enable)` | 要求先加载 | false |
| `WithWorkspaceRegistry(wsr)` | 工作区注册表 | 无 |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| `FSRepository` | `skill/repository.go` | 文件系统仓库，支持多根 + URL 根 + SHA256 缓存 |
| `FilteredRepository` | `skill/context_repository.go` | 基于 VisibilityFilter 的上下文过滤仓库 |
| `copySkillStager` | `tool/skill/stager.go` | 默认暂存器，复制技能文件到工作区 |
| `LoadTool` | `tool/skill/load.go` | skill_load 工具 |
| `RunTool` | `tool/skill/run.go` | skill_run 工具 |
| `ExecTool` | `tool/skill/exec.go` | skill_exec 交互式执行工具 |
| `WriteStdinTool` | `tool/skill/exec.go` | 向执行会话写入 stdin |
| `PollSessionTool` | `tool/skill/exec.go` | 轮询执行会话输出 |
| `KillSessionTool` | `tool/skill/exec.go` | 终止执行会话 |
| `SelectDocsTool` | `tool/skill/select_docs.go` | 选择/切换技能文档 |
| `ListDocsTool` | `tool/skill/list_docs.go` | 列出技能可用文档 |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `skill.Repository` | `DBRepositoryAdapter` + `FSRepositoryAdapter` | ✅ | 完全实现，DB 优先 + FS 回退 |
| `skill.RootedRepository` | `FSRepositoryAdapter` 暴露 `Roots()` 但未声明实现接口 | ⚠️ | 方法存在但未显式实现接口 |
| `skill.RefreshableRepository` | `FSRepositoryAdapter` 暴露 `Refresh()` 但未声明实现接口 | ⚠️ | 方法存在但未显式实现接口 |
| `skill.ContextRepository` | `NewFilteredRepository` 封装框架实现 | ✅ | 委托给 `skill.NewFilteredRepository` |
| `skill.VisibilityFilter` | `AgentVisibilityFilter` | ✅ | 完全实现，Layer A + Layer B 双层过滤 |
| `WithSkills(repo)` | `buildSkillDeps()` 中使用 | ✅ | 完全使用 |
| `WithSkillFilter(filter)` | `buildSkillDeps()` 中使用 | ✅ | 完全使用 |
| `WithCodeExecutor(exec)` | `buildSkillDeps()` 中使用 | ✅ | 完全使用 |
| `WithSkillToolProfile(profile)` | `buildSkillDeps()` 中使用 | ✅ | 完全使用 |
| `WithSkillLoadMode(mode)` | `buildSkillDeps()` 中使用 | ✅ | 完全使用 |
| `WithSkillsDirectoryHints(enable)` | `buildSkillDeps()` 中使用 | ✅ | 完全使用 |
| `WithSkillsLoadedContentInToolResults` | 未使用 | ℹ️ | 未启用提示缓存优化模式 |
| `WithMaxLoadedSkills` | 未使用 | ℹ️ | 未设置 LRU 淘汰上限 |
| `WithSkillRunAllowedCommands` | 未使用 | ℹ️ | 未限制允许的命令 |
| `WithSkillRunDeniedCommands` | 未使用 | ℹ️ | 未限制禁止的命令 |
| `WithSkillRunOutputLimits` | 未使用 | ℹ️ | 未限制输出大小 |
| `WithSkillRunForceSaveArtifacts` | 未使用 | ℹ️ | 通过自定义 artifactSavingExecutor 替代 |
| `WithSkillRunRequireSkillLoaded` | 未使用 | ℹ️ | 未强制先加载 |
| `WithToolActivationOnSkillLoad` | 未使用 | ℹ️ | 未使用工具激活机制 |
| `WithSkillsToolingGuidance` | 未使用 | ℹ️ | 使用框架默认 |
| `WithSkillsCapabilityGuidance` | 未使用 | ℹ️ | 使用框架默认 |
| `WithSkillsProtocolGuidance` | 未使用 | ℹ️ | 使用框架默认 |
| `SkillStager` 接口 | 未使用 | ℹ️ | 使用框架默认 copySkillStager |
| `SkillRunEnvProvider` 接口 | 未使用 | ℹ️ | 未注入自定义环境变量 |
| `skill_exec` 系列工具 | 未使用 | ℹ️ | 未启用交互式执行（ExecTool/WriteStdin/PollSession/KillSession） |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| DBRepositoryAdapter | `internal/skill/trpc/db_repository.go` | 框架无 DB 后端 | 框架仅提供 FSRepository，项目 Skill 存储在数据库中 |
| Skill Body 惰性加载 | `internal/skill/trpc/db_repository.go` | 框架无内置 | 避免一次性加载所有 Skill Body（可能数百 KB），按需从 DB 加载 |
| TTL 缓存 + 原子替换 | `internal/skill/trpc/db_repository.go` | 框架无内置 | DB 仓库需要缓存 + 定期刷新，FSRepository 通过 Refresh() 手动刷新 |
| artifactSavingExecutor | `internal/skill/trpc/artifact_executor.go` | `WithSkillRunForceSaveArtifacts` | 装饰器模式自动保存 artifact，比框架开关更灵活 |
| AgentVisibilityFilter | `internal/tools/skillruntime/filter.go` | `skill.VisibilityFilter` | 框架 VisibilityFilter 是简单函数，项目实现了 Layer A（策略）+ Layer B（意图路由+标签+评分）双层过滤 |
| Skill 解析路由 | `internal/tools/skillruntime/resolve.go` | 框架无内置 | 框架仅做可见性过滤，项目额外实现了意图路由、标签匹配、嵌入评分、历史性能排名 |
| Skill Guidance 注入 | `internal/agent/skill_guidance_inject.go` | 框架无内置 | 在 LLM 调用前注入技能引导信息到系统消息，框架仅通过 skill_load 工具被动加载 |
| Skill 完整 CRUD | `internal/biz/skill/skill.go` + `internal/data/skill.go` | 框架无内置 | 框架仅提供只读 Repository，项目需要用户动态创建/编辑/删除/发布 Skill |
| Skill 版本管理 | `internal/data/skill.go` | 框架无内置 | 支持版本发布、回滚、diff |
| Skill 调用追踪 | `internal/data/skill.go` | 框架无内置 | 记录每次 Skill 调用的输入/输出/耗时/状态 |
| Skill 进化体系 | `internal/biz/skill_evolution*.go` | 框架无内置 | PatternTrigger/HealthTrigger → 进化建议 → 沙箱验证 → 自动改进 |
| Skill 健康评分 | `internal/biz/skill_scoring.go` | 框架无内置 | 基于调用统计的 0-100 评分 |
| Skill 去重/合并 | `internal/biz/skill_dedup.go` + `skill_merge.go` | 框架无内置 | 四维度相似度比较 + 三阶段合并 |
| Skill 导入 | `internal/service/skill.go` | 框架无内置 | ZIP 导入 + 冲突检测 + 候选应用 |
| Skill Intelligence | `internal/biz/skill_intelligence.go` | 框架无内置 | 经验报告 + 根因分析 + Curator Agent |
| 运行时状态注入 | `internal/tools/skillruntime/runtime.go` | 框架无内置 | 通过 `trpcagent.MergeRuntimeState` 将用户查询注入运行时状态，供过滤使用 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `WithSkillsLoadedContentInToolResults` | 未评估提示缓存优化收益 | 评估中 |
| `WithMaxLoadedSkills` | 当前无 LRU 淘汰需求，Skill 数量可控 | 否 |
| `WithSkillRunAllowedCommands` / `WithSkillRunDeniedCommands` | 当前无安全限制需求 | 评估中 |
| `WithSkillRunOutputLimits` | 当前无输出大小限制需求 | 否 |
| `WithSkillRunRequireSkillLoaded` | 当前未强制先加载 | 否 |
| `WithToolActivationOnSkillLoad` | 当前未使用工具激活机制 | 否 |
| `skill_exec` 系列工具 | 当前仅使用 skill_load/skill_run，未启用交互式执行 | 评估中 |
| `SkillStager` 自定义 | 使用框架默认 copySkillStager | 否 |
| `SkillRunEnvProvider` | 当前无需自定义环境变量 | 否 |
| `WithSkillsToolingGuidance` / `CapabilityGuidance` / `ProtocolGuidance` | 使用框架默认指导文本 | 否 |
| `RefreshableRepository` 显式实现 | `FSRepositoryAdapter` 有 `Refresh()` 方法但未声明实现接口 | 是 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | **提示缓存优化**（`WithSkillsLoadedContentInToolResults`） | 技能内容追加到系统消息，每次请求都需重新处理 | 减少 token 消耗，降低延迟和成本 |
| 2 | **LRU 淘汰机制**（`WithMaxLoadedSkills`） | 无淘汰上限，可能加载过多技能 | 控制上下文窗口占用，避免 token 浪费 |
| 3 | **命令安全限制**（`WithSkillRunAllowedCommands`/`WithSkillRunDeniedCommands`） | 无命令限制 | 增强安全性，防止危险命令执行 |
| 4 | **输出大小限制**（`WithSkillRunOutputLimits`） | 无输出限制 | 防止大量输出消耗上下文窗口 |
| 5 | **交互式执行**（skill_exec 系列工具） | 仅支持 skill_run 一次性执行 | 支持长时间运行的交互式技能执行 |
| 6 | **工具激活机制**（`WithToolActivationOnSkillLoad`） | 无动态工具激活 | 技能加载时自动激活关联工具，提升技能执行能力 |
| 7 | **RootedRepository/RefreshableRepository 显式实现** | `FSRepositoryAdapter` 有方法但未声明实现接口 | 提高接口合规性，支持框架按接口类型断言 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | **DBRepositoryAdapter**（DB 后端 + TTL 缓存 + 惰性加载） | 框架仅提供 FSRepository（文件系统只读） | 贡献回框架，作为 skill 扩展包 |
| 2 | **Skill 完整 CRUD + 版本管理** | 框架仅提供只读 Repository | 保持自建（框架定位为运行时，不涉及持久化管理） |
| 3 | **Skill 调用追踪 + 健康评分** | 框架无内置 | 保持自建（业务特有需求） |
| 4 | **Skill 进化体系**（触发器→建议→沙箱→自动改进） | 框架无内置 | 保持自建（业务特有需求） |
| 5 | **Skill 去重/合并** | 框架无内置 | 保持自建（业务特有需求） |
| 6 | **双层过滤**（策略 + 意图路由 + 标签 + 评分） | 框架仅提供 VisibilityFilter 函数 | 评估贡献 VisibilityFilter 增强机制 |
| 7 | **Skill Guidance 注入**（LLM 调用前主动注入） | 框架仅通过 skill_load 工具被动加载 | 评估贡献 BeforeModel Hook 模式 |
| 8 | **artifactSavingExecutor**（装饰器自动保存） | 框架仅有 `WithSkillRunForceSaveArtifacts` 开关 | 贡献回框架，装饰器模式更灵活 |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| DBRepositoryAdapter 自建 | 框架仅提供 FSRepository，项目 Skill 存储在数据库中（用户动态创建/编辑），文件系统无法满足需求 | `internal/skill/trpc/db_repository.go`、Wire 绑定 |
| Skill Body 惰性加载 | 项目可能有大量 Skill，一次性加载所有 Body 会消耗大量内存和 DB 查询时间 | `internal/skill/trpc/db_repository.go` 的 `Get()` 方法 |
| 双层过滤路由 | 框架 VisibilityFilter 是简单布尔函数，项目需要意图路由、标签匹配、嵌入评分、历史性能排名等多维度过滤 | `internal/tools/skillruntime/resolve.go` + `filter.go` |
| Skill Guidance 主动注入 | 框架仅通过 skill_load 工具被动加载技能内容，项目需要在 LLM 调用前主动注入引导信息 | `internal/agent/skill_guidance_inject.go` |
| 未启用提示缓存优化 | 认知缺失，未评估 `WithSkillsLoadedContentInToolResults` 的收益 | `internal/agent/trpc_build.go` |
| 未启用命令/输出安全限制 | 当前为内部平台，安全需求较低；后续多租户场景需要 | `internal/agent/trpc_build.go` |
| 未启用交互式执行 | 项目当前仅需 skill_run 一次性执行，未评估 skill_exec 交互式场景 | 无 |
| FSRepositoryAdapter 未显式实现 RootedRepository/RefreshableRepository | 历史遗留，方法存在但未声明接口实现 | `internal/skill/trpc/repository.go` |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 启用提示缓存优化 | 启用框架功能 | P2 | `internal/agent/trpc_build.go` | 减少 token 消耗 10-30% |
| 2 | 启用命令安全限制 | 启用框架功能 | P3 | `internal/agent/trpc_build.go` | 增强安全性 |
| 3 | 启用输出大小限制 | 启用框架功能 | P3 | `internal/agent/trpc_build.go` | 防止上下文窗口溢出 |
| 4 | FSRepositoryAdapter 显式实现 RootedRepository/RefreshableRepository | 替换自建实现 | P3 | `internal/skill/trpc/repository.go` | 接口合规性提升 |
| 5 | 贡献 DBRepositoryAdapter 回框架 | 贡献回框架 | P2 | `pkg/trpc-agent-go/skill/` | 减少自建代码维护，框架用户受益 |
| 6 | 贡献 artifactSavingExecutor 回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/tool/skill/` | 减少自建代码维护 |
| 7 | 评估交互式执行工具启用 | 启用框架功能 | P3 | `internal/agent/trpc_build.go` | 支持长时间交互式技能 |

### 4.2 对齐项详情

#### 对齐项 #1：启用提示缓存优化

**类型**：启用框架功能

**现状**：
- 项目当前：技能内容通过 skill_load 工具加载后追加到系统消息末尾，每次 LLM 请求都需重新处理这些内容
- 框架提供能力：`WithSkillsLoadedContentInToolResults(true)` 将技能内容作为工具调用结果注入，可被 API 提供商的 prompt cache 命中

**对齐方案**：
1. 在 `buildSkillDeps()` 中添加 `trpcllmagent.WithSkillsLoadedContentInToolResults(true)` Option
2. 验证主流模型提供商（OpenAI/Anthropic）的 prompt cache 行为
3. 监控 token 消耗变化

**代码变更范围**：
- 修改：`internal/agent/trpc_build.go`（添加 1 行 Option）

**兼容性风险**：
- 低：该 Option 仅改变技能内容的注入位置（系统消息 → 工具结果），不影响功能正确性
- 需验证：部分模型提供商可能对工具结果中的长文本处理不同

**回退方案**：
- 移除该 Option 即可回退到系统消息模式

**验证方法**：
- 对比启用前后的 token 消耗（input_tokens）
- 验证技能加载和运行功能正常

**预期收益**：
- 代码减少：0 行（仅添加 1 行配置）
- 性能影响：减少 10-30% input token 消耗（取决于技能数量和内容长度）
- 维护成本：无变化

---

#### 对齐项 #2：启用命令安全限制

**类型**：启用框架功能

**现状**：
- 项目当前：skill_run 工具无命令限制，任何命令均可执行
- 框架提供能力：`WithSkillRunAllowedCommands`（白名单）和 `WithSkillRunDeniedCommands`（黑名单）

**对齐方案**：
1. 在 AgentRuntimeSettings 中添加 `SkillRunDeniedCommands` 配置字段
2. 在 `buildSkillDeps()` 中根据配置添加 `trpcllmagent.WithSkillRunDeniedCommands(...)` Option
3. 默认禁止危险命令（`rm -rf /`、`mkfs`、`dd` 等）

**代码变更范围**：
- 修改：`internal/agent/trpc_build.go`（添加命令限制 Option）
- 修改：`internal/data/ent/schema/agent_runtime_setting.go`（添加配置字段）
- 修改：对应 proto 定义

**兼容性风险**：
- 中：可能影响现有技能的命令执行，需要仔细评估禁止列表
- 需要提供配置覆盖机制

**回退方案**：
- 清空禁止列表即可回退

**验证方法**：
- 测试禁止命令被正确拦截
- 测试允许命令正常执行

**预期收益**：
- 代码减少：0 行
- 性能影响：无
- 维护成本：降低安全审计负担

---

#### 对齐项 #3：启用输出大小限制

**类型**：启用框架功能

**现状**：
- 项目当前：skill_run 工具无输出大小限制，大量输出可能消耗上下文窗口
- 框架提供能力：`WithSkillRunOutputLimits(RunOutputLimits{StdoutStderrBytes, PrimaryOutputBytes})`

**对齐方案**：
1. 在 AgentRuntimeSettings 中添加 `SkillRunOutputLimits` 配置字段
2. 在 `buildSkillDeps()` 中添加 `trpcllmagent.WithSkillRunOutputLimits(...)` Option
3. 设置合理默认值（如 stdout+stderr 32KB，主输出 16KB）

**代码变更范围**：
- 修改：`internal/agent/trpc_build.go`（添加输出限制 Option）
- 修改：`internal/data/ent/schema/agent_runtime_setting.go`（添加配置字段）

**兼容性风险**：
- 低：输出被截断而非报错，不影响功能
- 需要监控截断频率，调整限制值

**回退方案**：
- 设置为 0（无限制）即可回退

**验证方法**：
- 测试大输出被正确截断
- 测试正常输出不受影响

**预期收益**：
- 代码减少：0 行
- 性能影响：减少上下文窗口占用
- 维护成本：降低因输出过大导致的 Agent 异常

---

#### 对齐项 #4：FSRepositoryAdapter 显式实现 RootedRepository/RefreshableRepository

**类型**：替换自建实现

**现状**：
- 项目当前：`FSRepositoryAdapter` 有 `Roots()` 和 `Refresh()` 方法，但未声明实现 `skill.RootedRepository` 和 `skill.RefreshableRepository` 接口
- 框架提供能力：`RootedRepository` 和 `RefreshableRepository` 接口，框架代码可能按接口类型断言

**对齐方案**：
1. 在 `FSRepositoryAdapter` 结构体声明中添加接口注释：`var _ skill.RootedRepository = (*FSRepositoryAdapter)(nil)` 和 `var _ skill.RefreshableRepository = (*FSRepositoryAdapter)(nil)`
2. 确保方法签名完全匹配

**代码变更范围**：
- 修改：`internal/skill/trpc/repository.go`（添加接口断言）

**兼容性风险**：
- 无：仅添加编译期接口断言，不改变运行时行为

**回退方案**：
- 移除接口断言即可

**验证方法**：
- 编译通过即验证成功

**预期收益**：
- 代码减少：0 行
- 性能影响：无
- 维护成本：提高接口合规性，防止方法签名漂移

---

#### 对齐项 #5：贡献 DBRepositoryAdapter 回框架

**类型**：贡献回框架

**现状**：
- 项目当前：`DBRepositoryAdapter` 完全自建，实现 `skill.Repository` 接口，支持 DB 后端 + TTL 缓存 + 惰性加载
- 框架提供能力：仅 `FSRepository`（文件系统只读），无 DB 后端

**对齐方案**：
1. 将 `DBRepositoryAdapter` 抽象为框架级 `skill.DBRepository`，定义数据源接口（`SkillReader`）解耦 biz 层
2. 提取 TTL 缓存 + 惰性加载 + 原子替换为通用能力
3. 在 `pkg/trpc-agent-go/skill/` 中实现，作为框架扩展
4. 项目通过实现 `SkillReader` 接口适配 biz 层

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/skill/db_repository.go`（框架级实现）
- 修改：`internal/skill/trpc/db_repository.go`（改为实现框架 `SkillReader` 接口）
- 删除：`internal/skill/trpc/db_repository.go` 中的缓存/惰性加载逻辑（移入框架）

**兼容性风险**：
- 中：需要定义清晰的 `SkillReader` 接口，确保与 biz 层解耦
- 需要框架维护者审核接受

**回退方案**：
- 保持现有自建实现

**验证方法**：
- 框架单元测试覆盖 DBRepository
- 项目集成测试验证功能不变

**预期收益**：
- 代码减少：约 80 行（缓存/惰性加载逻辑移入框架）
- 性能影响：无
- 维护成本：减少框架升级时的适配工作量
- 功能增强：框架其他用户可使用 DB 后端

---

#### 对齐项 #6：贡献 artifactSavingExecutor 回框架

**类型**：贡献回框架

**现状**：
- 项目当前：`artifactSavingExecutor` 是 `codeexecutor.CodeExecutor` 的装饰器，自动将代码执行输出文件保存为 session artifact
- 框架提供能力：`WithSkillRunForceSaveArtifacts` 开关，但不如装饰器灵活

**对齐方案**：
1. 将 `artifactSavingExecutor` 贡献为框架 `tool/skill/` 包的 Option
2. 添加 `WithArtifactSavingExecutor(helper SaveArtifactHelper)` Option
3. 项目改用框架 Option，移除自建装饰器

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/tool/skill/artifact_executor.go`（框架级实现）
- 修改：`internal/skill/trpc/executor.go`（改用框架 Option）
- 删除：`internal/skill/trpc/artifact_executor.go`

**兼容性风险**：
- 低：装饰器模式是通用模式，框架可无缝集成

**回退方案**：
- 保持现有自建实现

**验证方法**：
- 测试 artifact 自动保存功能正常

**预期收益**：
- 代码减少：约 60 行
- 性能影响：无
- 维护成本：减少自建代码维护

---

#### 对齐项 #7：评估交互式执行工具启用

**类型**：启用框架功能

**现状**：
- 项目当前：仅使用 skill_load/skill_run，未启用 skill_exec 系列交互式执行工具
- 框架提供能力：`ExecTool`/`WriteStdinTool`/`PollSessionTool`/`KillSessionTool`，支持长时间运行的交互式技能执行

**对齐方案**：
1. 评估业务场景是否需要交互式执行（如 REPL、调试、长时间运行脚本）
2. 如需要，在 `buildSkillDeps()` 中注册交互式执行工具
3. 配合 `WithSkillToolProfile` 调整工具集

**代码变更范围**：
- 修改：`internal/agent/trpc_build.go`（添加交互式工具注册）

**兼容性风险**：
- 中：交互式执行涉及会话状态管理，需要评估资源消耗和安全性
- 需要考虑并发会话数限制

**回退方案**：
- 不注册交互式工具即可

**验证方法**：
- 测试交互式执行会话的创建/写入/轮询/终止

**预期收益**：
- 代码减少：0 行
- 性能影响：增加会话状态管理开销
- 功能增强：支持交互式技能执行场景

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #1（提示缓存优化）、#4（接口断言） | 无 | 小 |
| Phase 2 | #2（命令安全限制）、#3（输出大小限制） | Phase 1 | 中 |
| Phase 3 | #5（贡献 DBRepository）、#6（贡献 artifactSavingExecutor） | 框架维护者审核 | 大 |
| Phase 4 | #7（交互式执行评估） | Phase 2 | 中 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 提示缓存优化导致部分模型行为异常 | 低 | 中 | 逐模型验证，提供配置开关 |
| 命令限制影响现有技能执行 | 中 | 中 | 默认仅禁止极端危险命令，提供配置覆盖 |
| DBRepository 贡献被框架拒绝 | 中 | 低 | 保持自建实现，不影响项目功能 |
| 交互式执行资源消耗过大 | 中 | 中 | 设置并发会话数限制，添加超时机制 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| GAIA 基准测试 | `examples/skill/` | `skill.NewFSRepository`、`llmagent.WithSkills`、`WithSkillLoadMode`、`WithMaxLoadedSkills` | 创建 FSRepository → WithSkills 注入 Agent | 项目使用 DBRepositoryAdapter 替代 FSRepository，添加了 VisibilityFilter 双层过滤 |
| 交互式聊天 | `examples/skillrun/` | `skill.NewFSRepository`、`llmagent.WithCodeExecutor`、`WithSkillLoadMode("session")` | 创建 FSRepository + 选择 Executor → WithSkills + WithCodeExecutor 注入 Agent | 项目通过 `buildSkillDeps()` 动态选择 DB/FS + Executor，额外包装 artifactSavingExecutor |
| Knowledge Search | `examples/skill/`（helper.go） | `skill.Summary`、`skill.Skill`、`skill.Doc` | 通过 Repository 接口查询技能摘要和全文 | 项目通过 DBRepositoryAdapter 从数据库加载，框架示例从文件系统加载 |

**对齐方案必须以示例代码的用法为目标状态**：
- 示例中 `WithSkills(repo)` + `WithSkillLoadMode("session")` + `WithMaxLoadedSkills(5)` 的模式是标准用法
- 项目已遵循此模式，仅需补充 `WithMaxLoadedSkills` 和 `WithSkillsLoadedContentInToolResults` 等未使用的 Option

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Skill 模块 README | `pkg/trpc-agent-go/skill/README.md`（如存在） |
| Skill Tool 文档 | `pkg/trpc-agent-go/tool/skill/README.md`（如存在） |
| 框架主文档 | `pkg/trpc-agent-go/docs/mkdocs/zh/`（Skill 相关章节） |
