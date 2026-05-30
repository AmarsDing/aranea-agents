# WebFetch 与搜索工具

## 一、需求文档

### 1.1 背景

trpc-agent-go 框架提供了丰富的网页抓取和搜索工具：WebFetch（HTTP/Claude/Gemini 三种后端）、Google Search、DuckDuckGo Search、ArXiv Search、Wikipedia Search。当前项目 `internal/tools/` 已有 `duckduckgo` 和 `google_search` 注册，但 WebFetch 的 HTTP 后端注册了空实现，ArXiv 和 Wikipedia 未集成。Agent 缺乏完整的网页抓取和学术搜索能力。

### 1.2 目标

- 补全 WebFetch HTTP 后端的实际构建（替换当前空实现）
- 集成 ArXiv Search 工具
- 集成 Wikipedia Search 工具
- 所有搜索工具在 Registry 中正确注册并可按需启用

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | WebFetch HTTP 后端实际构建 | P0 | 替换 Registry 中 `httpfetch` 的空 Factory |
| F2 | ArXiv Search 工具集成 | P1 | 学术论文搜索，支持 PDF 内容读取 |
| F3 | Wikipedia Search 工具集成 | P1 | 百科搜索，支持多语言 |
| F4 | WebFetch 域名过滤 | P2 | allowed_domains / blocked_domains 配置 |
| F5 | 搜索工具统一配置 | P1 | HTTP 超时、代理等统一配置入口 |

### 1.4 非功能需求

- WebFetch 默认超时 30s，可配置
- ArXiv 搜索默认返回 5 条，可配置
- Wikipedia 默认英文，可配置语言
- 所有 HTTP 请求走统一 HTTP Client（可配置代理）
- 域名过滤防止 SSRF

### 1.5 验收标准

- WebFetch 可抓取网页内容并转为 Markdown
- ArXiv Search 可搜索学术论文
- Wikipedia Search 可搜索百科条目
- 各工具可独立启用/禁用
- 与现有 DuckDuckGo/Google Search 共存

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go）

**核心包路径**：

| 工具 | 包路径 |
|------|--------|
| WebFetch HTTP | `pkg/trpc-agent-go/tool/webfetch/httpfetch/fetch.go` |
| WebFetch Claude | `pkg/trpc-agent-go/tool/webfetch/claudefetch/fetch.go` |
| WebFetch Gemini | `pkg/trpc-agent-go/tool/webfetch/geminifetch/fetch.go` |
| Google Search | `pkg/trpc-agent-go/tool/google/search/toolset.go` |
| DuckDuckGo | `pkg/trpc-agent-go/tool/duckduckgo/duckduckgo.go` |
| ArXiv Search | `pkg/trpc-agent-go/tool/arxivsearch/arxiv_search.go` |
| Wikipedia | `pkg/trpc-agent-go/tool/wikipedia/wikipedia_search.go` |

**WebFetch HTTP 核心类型和函数**：

```go
// httpfetch.NewTool 创建 HTTP 网页抓取工具
func NewTool(opts ...Option) tool.CallableTool

// Option
type Option func(*config)
func WithHTTPClient(c *http.Client) Option
func WithMaxContentLength(limit int) Option
func WithMaxTotalContentLength(limit int) Option
func WithAllowedDomains(domains []string) Option
func WithBlockedDomains(domains []string) Option
func WithTimeout(timeout time.Duration) Option

// 工具名：web_fetch
// 输入：{ urls: string[] }
// 输出：{ results: [{retrieved_url, status_code, content_type, content, error}], summary }
```

**ArXiv Search 核心类型和函数**：

```go
// arxivsearch.NewToolSet 创建 ArXiv 搜索工具集
func NewToolSet(opts ...Option) (*ToolSet, error)

// Option
type Option func(config *arxiv.ClientConfig)
func WithBaseURL(baseURL string) Option
func WithPageSize(pageSize int) Option
func WithDelaySeconds(delaySeconds time.Duration) Option
func WithNumRetries(numRetries int) Option
func WithHTTPClient(c *http.Client) Option
func WithTimeout(timeout time.Duration) Option

// ToolSet 名称：arxiv_search
// 工具名：search
// 输入：{ search: { query, id_list, max_results }, read_arxiv_papers: bool }
// 输出：[{ title, id, entry_id, authors, primary_category, categories, published, pdf_url, summary, content }]
```

**Wikipedia Search 核心类型和函数**：

```go
// wikipedia.NewToolSet 创建 Wikipedia 搜索工具集
func NewToolSet(opts ...Option) (*WikipediaToolSet, error)

// Option
type Option func(*config)
func WithLanguage(language string) Option
func WithMaxResults(maxResults int) Option
func WithHTTPClient(c *http.Client) Option
func WithTimeout(timeout time.Duration) Option
func WithUserAgent(userAgent string) Option

// ToolSet 名称：wikipedia
// 工具名：wikipedia_search
// 输入：{ query, limit, include_all }
// 输出：{ query, results: [{title, url, description, page_id, word_count, size_bytes, last_modified}], total_hits, summary }
```

**Google Search 核心类型和函数**：

```go
// search.NewToolSet 创建 Google 搜索工具集
func NewToolSet(ctx context.Context, opts ...Option) (*ToolSet, error)

// Option
func WithAPIKey(key string) Option
func WithEngineID(id string) Option
func WithBaseURL(url string) Option
func WithSize(size int) Option
func WithOffset(offset int) Option
func WithLanguage(lang string) Option

// ToolSet 名称：google
// 工具名：search
```

**DuckDuckGo 核心类型和函数**：

```go
// duckduckgo.NewTool 创建 DuckDuckGo 搜索工具
func NewTool(opts ...Option) tool.CallableTool

// Option
func WithBaseURL(baseURL string) Option
func WithUserAgent(userAgent string) Option
func WithHTTPClient(httpClient *http.Client) Option

// 工具名：duckduckgo_search
```

### 2.2 当前项目现状

| 位置 | 现状 |
|------|------|
| `internal/tools/toolset.go` Registry | `httpfetch` 注册存在，Factory 返回 `trpchttpfetch.NewTool()` ✓ |
| `internal/tools/toolset.go` Registry | `claudefetch` 注册存在，标记为 stub |
| `internal/tools/toolset.go` Registry | `geminifetch` 注册存在 |
| `internal/tools/toolset.go` Registry | `duckduckgo` 注册存在 |
| `internal/tools/toolset.go` Registry | `google_search` 注册存在 |
| `internal/tools/toolset.go` Registry | `arxiv_search` **未注册** |
| `internal/tools/toolset.go` Registry | `wikipedia` **未注册** |
| `internal/tools/trpc/toolsets.go` | `ToolsetConfig` 有 `WebFetch`/`WebSearch`/`GoogleSearch`/`ArxivSearch`/`Wikipedia` 字段 |
| `internal/tools/assemble.go` | `Assemble()` 根据 enabled 列表构建工具 |

### 2.3 架构设计

**模块在四层架构中的位置**：

```
internal/tools           ← Registry 新增 arxiv_search/wikipedia + 修复 httpfetch
        ↓
internal/agent           ← tool_assembly.go 传递配置
        ↓
internal/service         ← Runner 装配时传入搜索配置
```

**新增/修改的文件清单**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/tools/toolset.go` | 修改 | Registry 新增 `arxiv_search` 和 `wikipedia` 注册 |
| `internal/tools/assemble.go` | 修改 | `Assemble()` 支持 ArXiv/Wikipedia 工具构建 |
| `internal/tools/trpc/toolsets.go` | 修改 | `BuildToolsets` 传递 ArXiv/Wikipedia 配置 |
| `internal/data/builtin_tools_seed.go` | 修改 | 新增 `arxiv_search`/`wikipedia` 种子数据 |

**接口设计**：

```go
// internal/tools/toolset.go — 新增 Registry 条目

{
    Name:        "arxiv_search",
    Description: "ArXiv scholarly article search tool",
    Category:    "search",
    Tags:        []string{"search", "academic", "arxiv"},
    ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
        return trpcarxiv.NewToolSet()
    },
    EnabledByDefault: false,
    RiskLevel:        "low",
},

{
    Name:        "wikipedia",
    Description: "Wikipedia encyclopedia search tool",
    Category:    "search",
    Tags:        []string{"search", "encyclopedia", "wikipedia"},
    ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
        return trpcwiki.NewToolSet()
    },
    EnabledByDefault: false,
    RiskLevel:        "low",
},
```

**数据流图**：

```
Agent 配置启用搜索工具
  → ToolsetConfig.WebFetch / .ArxivSearch / .Wikipedia
    → BuildToolsets()
      → tools.Assemble(ctx, AssemblyConfig{...})
        → Registry()["httpfetch"].Factory → trpchttpfetch.NewTool()
        → Registry()["arxiv_search"].ToolSetFactory → trpcarxiv.NewToolSet()
        → Registry()["wikipedia"].ToolSetFactory → trpcwiki.NewToolSet()
          → 挂载到 Agent 的工具列表
```

### 2.4 与框架的集成方式

1. **WebFetch HTTP**：`httpfetch.NewTool()` 直接构建，传入 `WithTimeout`/`WithAllowedDomains` 等配置
2. **ArXiv Search**：`arxivsearch.NewToolSet()` 构建，返回 `ToolSet` 接口
3. **Wikipedia**：`wikipedia.NewToolSet()` 构建，返回 `ToolSet` 接口
4. **Google Search**：`search.NewToolSet(ctx, WithAPIKey, WithEngineID)` 构建，需要 API Key
5. **DuckDuckGo**：`duckduckgo.NewTool()` 构建，无需 API Key
6. **统一配置**：`AssemblyConfig` 扩展搜索相关配置字段，`Assemble()` 统一构建

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| WebFetch URL 被域名过滤阻止 | 返回 `"URL matches blocked pattern"` 错误 |
| WebFetch HTTP 请求失败 | 返回 `"error"` 字段描述失败原因 |
| WebFetch 非 2xx 状态码 | 返回 `"HTTP status NNN"` 错误 |
| WebFetch 不支持的 Content-Type | 返回 `"unsupported content type"` 错误 |
| ArXiv 搜索无结果 | 返回空列表 |
| ArXiv PDF 读取失败 | 返回 `"failed to read PDF from URL"` 错误 |
| Wikipedia 搜索失败 | 返回 `"Error: ..."` summary |
| Google Search API Key 缺失 | `NewToolSet` 返回 `"api key is required"` 错误 |
| Google Search Engine ID 缺失 | `NewToolSet` 返回 `"search engine id is required"` 错误 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| WS-01 | `internal/tools/toolset.go`：Registry 新增 `arxiv_search` 注册 | 无 | S |
| WS-02 | `internal/tools/toolset.go`：Registry 新增 `wikipedia` 注册 | 无 | S |
| WS-03 | `internal/tools/assemble.go`：`Assemble()` 支持 ArXiv/Wikipedia 构建 | WS-01, WS-02 | S |
| WS-04 | `internal/tools/trpc/toolsets.go`：`BuildToolsets` 传递 ArXiv/Wikipedia 配置 | WS-03 | S |
| WS-05 | `internal/data/builtin_tools_seed.go`：新增 `arxiv_search`/`wikipedia` 种子 | WS-01, WS-02 | S |
| WS-06 | WebFetch HTTP 配置增强（域名过滤/超时） | 无 | S |
| WS-07 | 单元测试：新工具注册和构建 | WS-03 | S |
| WS-08 | 集成测试：搜索工具端到端 | WS-04 | M |

### 3.2 开发顺序

```
WS-01 ─┐
WS-02 ─┤→ WS-03 → WS-04 → WS-07 → WS-08
WS-05 ─┘              ↑
WS-06 ────────────────┘
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| ArXiv 注册 | `go test ./internal/tools/... -run TestRegistry -count=1` |
| Wikipedia 注册 | `go test ./internal/tools/... -run TestRegistry -count=1` |
| Assemble 集成 | `go test ./internal/tools/... -run TestAssemble -count=1` |
| BuildToolsets | `go test ./internal/tools/trpc/... -run TestBuildToolsets -count=1` |
| 全量验证 | `make build && make test` |
