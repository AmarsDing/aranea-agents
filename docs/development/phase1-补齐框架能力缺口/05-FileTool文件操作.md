# FileTool 文件操作

## 一、需求文档

### 1.1 背景

trpc-agent-go 框架提供了完整的文件操作工具集（`tool/file`），包含读/写/搜索/补丁/差异编辑等 9 种工具。当前项目 `internal/tools/` Registry 已有 `file` 注册，但 `ToolSetFactory` 返回 `nil, nil`，即注册了但未实际构建。Agent 在需要操作文件时缺乏完整的文件工具能力。

### 1.2 目标

- 完整集成框架 `file.NewToolSet`，替换当前的空实现
- 支持 9 种文件工具：save_file / read_file / read_multiple_files / list_file / search_file / search_content / replace_content / diff_edit / patch_file
- 支持配置 base_dir（工作目录）和各工具的启用/禁用
- 与现有 `AssemblyConfig.FilesystemDir` 配置对齐

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | file ToolSet 实际构建 | P0 | 替换 Registry 中 `file` 的空 ToolSetFactory |
| F2 | save_file 写文件 | P0 | 保存文件到 base_dir |
| F3 | read_file 读文件 | P0 | 读取 base_dir 下的文件内容 |
| F4 | read_multiple_files 批量读 | P1 | 一次读取多个文件 |
| F5 | list_file 列目录 | P0 | 列出 base_dir 下的文件和目录 |
| F6 | search_file 搜索文件 | P1 | 按模式搜索文件名 |
| F7 | search_content 搜索内容 | P1 | 在文件内容中搜索 |
| F8 | replace_content 替换内容 | P1 | 替换文件中的指定内容 |
| F9 | diff_edit 差异编辑 | P2 | 基于 diff 的精确编辑 |
| F10 | patch_file 补丁应用 | P2 | 应用 unified patch |

### 1.4 非功能需求

- 文件操作限制在 base_dir 内，防止目录遍历攻击
- 最大文件大小可配置（默认 1MB）
- 文件权限可配置（目录 0755，文件 0644）
- 工具可按需启用/禁用

### 1.5 验收标准

- Agent 启用 file 工具后，可读写 base_dir 下的文件
- 目录遍历攻击被阻止（`..` 和绝对路径被拒绝）
- 文件大小超限时有明确错误提示
- 各工具可独立启用/禁用
- 与现有 `AssemblyConfig.FilesystemDir` 配置一致

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go）

**核心包路径**：`pkg/trpc-agent-go/tool/file/file.go`

**核心类型和函数**：

```go
// fileToolSet 实现了 tool.ToolSet 接口
type fileToolSet struct {
    baseDir                  string
    hasInputsDir             bool
    saveFileEnabled          bool
    readFileEnabled          bool
    readMultipleFilesEnabled bool
    listFileEnabled          bool
    searchFileEnabled        bool
    searchContentEnabled     bool
    replaceContentEnabled    bool
    diffEditEnabled          bool
    patchFileEnabled         bool
    createDirMode            os.FileMode
    createFileMode           os.FileMode
    maxFileSize              int64
    tools                    []tool.Tool
    name                     string
}

// NewToolSet 创建文件操作工具集
func NewToolSet(opts ...Option) (tool.ToolSet, error)

// Option 函数
func WithBaseDir(baseDir string) Option
func WithSaveFileEnabled(e bool) Option
func WithReadFileEnabled(e bool) Option
func WithReadMultipleFilesEnabled(e bool) Option
func WithListFileEnabled(e bool) Option
func WithSearchFileEnabled(e bool) Option
func WithSearchContentEnabled(e bool) Option
func WithReplaceContentEnabled(e bool) Option
func WithDiffEditEnabled(e bool) Option
func WithPatchFileEnabled(e bool) Option
func WithCreateDirMode(m os.FileMode) Option
func WithCreateFileMode(m os.FileMode) Option
func WithMaxFileSize(s int64) Option
func WithName(name string) Option
```

**9 种工具**（`fileToolSet` 内部构建）：

| 工具名 | 方法 | 说明 |
|--------|------|------|
| `save_file` | `saveFileTool()` | 保存文件 |
| `read_file` | `readFileTool()` | 读取单个文件 |
| `read_multiple_files` | `readMultipleFilesTool()` | 批量读取文件 |
| `list_file` | `listFileTool()` | 列出目录内容 |
| `search_file` | `searchFileTool()` | 按模式搜索文件名 |
| `search_content` | `searchContentTool()` | 搜索文件内容 |
| `replace_content` | `replaceContentTool()` | 替换文件内容 |
| `diff_edit` | `diffEditTool()` | 差异编辑 |
| `patch_file` | `patchFileTool()` | 应用补丁 |

**安全机制**：
- `resolvePath()` 验证路径，拒绝绝对路径和 `..`
- `normalizeInputsAlias()` 处理 `inputs/` 前缀别名
- `missingFileHint()` 文件不存在时提供目录提示
- `matchFiles()` 使用 `doublestar.Glob` 模式匹配

### 2.2 当前项目现状

| 位置 | 现状 |
|------|------|
| `internal/tools/toolset.go` | Registry 中 `file` 注册存在，但 `ToolSetFactory` 返回 `nil, nil` |
| `internal/tools/toolset.go` | `AssemblyConfig` 有 `FilesystemDir` 字段 |
| `internal/tools/trpc/toolsets.go` | `ToolsetConfig` 有 `Filesystem` 和 `FilesystemDir` 字段 |
| `internal/tools/trpc/toolsets.go` | `BuildToolsets` 中 `Filesystem` 启用时将 `filesystem` 加入 enabled 列表 |
| `internal/tools/assemble.go` | `Assemble()` 根据 enabled 列表构建工具 |

### 2.3 架构设计

**模块在四层架构中的位置**：

```
internal/tools           ← 修改 Registry file 注册 + Assemble 逻辑
        ↓
internal/agent           ← tool_assembly.go 中 FilesystemDir 传递
        ↓
internal/service         ← Runner 装配时传入 FilesystemDir
```

**新增/修改的文件清单**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/tools/toolset.go` | 修改 | Registry `file` 注册的 `ToolSetFactory` 替换为实际构建 |
| `internal/tools/assemble.go` | 修改 | `Assemble()` 中 `file` 工具集构建传入 `FilesystemDir` |
| `internal/tools/trpc/toolsets.go` | 修改 | `BuildToolsets` 传递 `FilesystemDir` 到 `Assemble` |

**接口设计**：

```go
// internal/tools/toolset.go — Registry 修改

{
    Name:        "file",
    Description: "File operation ToolSet (read, write, search, replace, list)",
    Category:    "filesystem",
    Tags:        []string{"filesystem", "read", "write", "search"},
    ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
        return nil, nil  // 当前空实现，需替换
    },
    EnabledByDefault:    true,
    RiskLevel:           "low",
    SupportsConcurrency: true,
}

// 替换为：

{
    Name:        "file",
    Description: "File operation ToolSet (read, write, search, replace, list)",
    Category:    "filesystem",
    Tags:        []string{"filesystem", "read", "write", "search"},
    ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
        baseDir := filesystemDirFromContext(ctx)
        opts := []trpcfile.Option{
            trpcfile.WithBaseDir(baseDir),
        }
        return trpcfile.NewToolSet(opts...)
    },
    EnabledByDefault:    true,
    RiskLevel:           "low",
    SupportsConcurrency: true,
}
```

**数据流图**：

```
Agent 配置 FilesystemDir
  → ToolsetConfig.FilesystemDir
    → BuildToolsets()
      → tools.Assemble(ctx, AssemblyConfig{FilesystemDir: dir})
        → Registry()["file"].ToolSetFactory(ctx)
          → trpcfile.NewToolSet(WithBaseDir(dir))
            → 返回包含 9 种工具的 ToolSet
              → 挂载到 Agent 的工具列表
```

### 2.4 与框架的集成方式

1. **直接使用**：`trpcfile.NewToolSet(WithBaseDir(dir))` 直接构建框架工具集
2. **配置传递**：`AssemblyConfig.FilesystemDir` → `ToolSetFactory` → `WithBaseDir`
3. **工具过滤**：框架 `NewToolSet` 的 `With*Enabled` 选项控制各工具启用/禁用
4. **安全限制**：框架内置 `resolvePath()` 防止目录遍历，项目无需额外实现
5. **运行时别名**：现有 `runtime_alias.go` 中 `save_file → write_file` 别名继续生效

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| base_dir 不存在 | `NewToolSet` 返回错误 `"base directory does not exist"` |
| base_dir 不是目录 | `NewToolSet` 返回错误 `"is not a directory"` |
| 路径包含 `..` | `resolvePath` 返回 `"invalid path"` 错误 |
| 绝对路径 | `resolvePath` 拒绝 |
| 文件不存在 | 返回错误 + 目录提示（`missingFileHint`） |
| 文件超过 maxFileSize | 截断或返回错误 |
| 写文件目录不存在 | 自动创建（`createDirMode` 权限） |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| FT-01 | `internal/tools/toolset.go`：替换 `file` 注册的 `ToolSetFactory` 为实际构建 | 无 | S |
| FT-02 | `internal/tools/assemble.go`：`Assemble()` 传递 `FilesystemDir` 到 file ToolSetFactory | FT-01 | S |
| FT-03 | `internal/tools/trpc/toolsets.go`：`BuildToolsets` 传递 `FilesystemDir` | FT-02 | S |
| FT-04 | context 传递 FilesystemDir 机制 | FT-01 | S |
| FT-05 | 单元测试：file ToolSet 构建 | FT-01 | S |
| FT-06 | 集成测试：Agent + file 工具端到端 | FT-03 | M |
| FT-07 | 验证安全限制（路径遍历等） | FT-05 | S |

### 3.2 开发顺序

```
FT-01 → FT-04 → FT-02 → FT-03 → FT-05 → FT-06 → FT-07
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| ToolSet 构建 | `go test ./internal/tools/... -run TestFileToolSet -count=1` |
| Assemble 集成 | `go test ./internal/tools/... -run TestAssemble -count=1` |
| BuildToolsets | `go test ./internal/tools/trpc/... -run TestBuildToolsets -count=1` |
| 安全限制 | `go test ./internal/tools/... -run TestFileSecurity -count=1` |
| 全量验证 | `make build && make test` |
