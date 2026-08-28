package deferred

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"aranea-agents/internal/metrics"
	"aranea-agents/internal/tools/alias"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type toolSearchInput struct {
	Query string `json:"query" jsonschema:"description=Search query to find relevant tools by name or capability,required"`
}

type toolSearchResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
}

type toolSearchOutput struct {
	Tools      []toolSearchResult `json:"tools"`
	Suggestion string             `json:"suggestion,omitempty"`
}

// DeferredToolEntry 描述一个延迟工具的静态元数据。
// 不再持有 Factory——工具在装配阶段已完全创建，
// 此条目仅用于 catalog 展示（cue 渲染、tool_search 搜索）。
type DeferredToolEntry struct {
	// Name 是运行时名称（LLM 可见/可调用的名字）。
	// ToolSet 工具带 "{toolset}_" 前缀（框架 NamedTool 命名约定），
	// 独立工具等于 BaseName。
	Name string
	// BaseName 是工具声明的原始名称（DeferredCallableTool 激活门禁检查的名字）。
	// 与 Name 相同表示独立工具（无前缀）。
	BaseName    string
	Description string
	Category    string
}

// deferredView 是延迟工具目录的不可变快照（P0-2B 方案B 热替换间接层）。
// 构建期装配完整后发布；热替换时经 SwapView 整体原子替换，绑定同一
// manager 句柄的 filter/tool_search/tool_load/catalog cue 四件套立即同刻
// 看到新目录，而 agent 实例与框架 options 零改动（框架无这些部件的热替换
// API，间接层在产品侧闭环，不碰 vendored 框架）。
type deferredView struct {
	catalog       []DeferredToolEntry
	catalogIndex  map[string]int    // name → index into catalog for O(1) lookup
	baseToRuntime map[string]string // unique BaseName → runtime Name（tool_load 别名解析用）
	tools         map[string]trpctool.Tool
	categoryIndex map[string][]string
	names         map[string]bool // 运行时名集合（ToolFilter 每次调用读取）
	staticCue     string          // 预渲染静态目录 cue（catalog 为空时为 ""）
}

func newView(catalog []DeferredToolEntry) *deferredView {
	v := &deferredView{
		catalog:       catalog,
		catalogIndex:  buildCatalogIndex(catalog),
		baseToRuntime: buildBaseNameIndex(catalog),
		tools:         make(map[string]trpctool.Tool),
		categoryIndex: buildCategoryIndex(catalog),
		names:         make(map[string]bool, len(catalog)),
		staticCue:     RenderCatalogCue(catalog),
	}
	for _, entry := range catalog {
		v.names[entry.Name] = true
		// BaseName lets ToolFilter hide runtime aliases (shell / shell_exec)
		// whose Declaration().Name is the unprefixed inner tool, not the
		// "{toolset}_{tool}" catalog key. Without this, ApplyRuntimeNameAliases
		// re-exposes the full schema after deferred wrap.
		if entry.BaseName != "" {
			v.names[entry.BaseName] = true
		}
	}
	return v
}

// DeferredToolManager 管理延迟工具的目录和 per-session 激活状态。
//
// 去 factory 化（WP-4 修复版）+ 视图间接层（P0-2B 方案B）：
//   - 不再持有 factory 函数，不再负责工具创建
//   - 本结构是**稳定句柄**：catalog/tools/names 等全部收进不可变 deferredView，
//     经 atomic.Pointer 持有；读路径无锁（每次调用 Load 当前视图）
//   - 激活状态存储在 session state（temp:deferred:activated），per-session 隔离，
//     与视图解耦——热替换换视图后激活状态按名字自然延续
type DeferredToolManager struct {
	view atomic.Pointer[deferredView]
}

func NewDeferredToolManager(catalog []DeferredToolEntry) *DeferredToolManager {
	m := &DeferredToolManager{}
	m.view.Store(newView(catalog))
	return m
}

// SwapView 将本 manager 的当前视图原子替换为 src 的当前视图（两句柄此后共享
// 同一不可变视图）。P0-2B 热替换专用：src 是新构建面的 manager（其 flat 元工具
// 不安装、句柄随后被丢弃），本句柄是 agent 存活面四件套的绑定者。
func (m *DeferredToolManager) SwapView(src *DeferredToolManager) {
	m.view.Store(src.view.Load())
}

func buildCatalogIndex(catalog []DeferredToolEntry) map[string]int {
	idx := make(map[string]int, len(catalog))
	for i, entry := range catalog {
		idx[entry.Name] = i
	}
	return idx
}

// buildBaseNameIndex 构建「唯一基础名 → 运行时名」索引。
// 仅收录 BaseName != Name 且在目录中唯一的条目——重名基础名（如两个 ToolSet
// 各有 read_file）无法无歧义解析，不建索引，tool_load 按未找到处理并列出候选。
func buildBaseNameIndex(catalog []DeferredToolEntry) map[string]string {
	count := make(map[string]int, len(catalog))
	for _, entry := range catalog {
		if entry.BaseName != "" && entry.BaseName != entry.Name {
			count[entry.BaseName]++
		}
	}
	idx := make(map[string]string, len(count))
	for _, entry := range catalog {
		if entry.BaseName != "" && entry.BaseName != entry.Name && count[entry.BaseName] == 1 {
			idx[entry.BaseName] = entry.Name
		}
	}
	return idx
}

// resolve 将模型给出的工具名解析为目录运行时名。
// 依次尝试：目录运行时名精确匹配 → 唯一基础名 → RuntimeToolNameAliases 别名链
// （如 shell_exec → exec_command → hostexec_exec_command）。解析失败返回 false。
func (v *deferredView) resolve(name string) (string, bool) {
	if _, ok := v.catalogIndex[name]; ok {
		return name, true
	}
	if rt, ok := v.baseToRuntime[name]; ok {
		return rt, true
	}
	target, ok := alias.RuntimeToolNameAliases[name]
	if !ok {
		return "", false
	}
	visited := map[string]bool{name: true}
	for {
		if visited[target] {
			return "", false // 环保护
		}
		visited[target] = true
		if _, ok := v.catalogIndex[target]; ok {
			return target, true
		}
		if rt, ok := v.baseToRuntime[target]; ok {
			return rt, true
		}
		next, ok := alias.RuntimeToolNameAliases[target]
		if !ok {
			return "", false
		}
		target = next
	}
}

func buildCategoryIndex(catalog []DeferredToolEntry) map[string][]string {
	idx := make(map[string][]string)
	for _, entry := range catalog {
		if entry.Category != "" {
			idx[entry.Category] = append(idx[entry.Category], entry.Name)
		}
	}
	return idx
}

// RegisterTool 注册一个已装配的延迟工具引用。
// 供 tool_load 在激活后返回完整 schema。
//
// 契约：仅允许装配期（manager 发布给并发读者之前）调用——视图发布后不可变，
// 热替换对 tools 表的刷新经 SwapView 整体换视图完成。
func (m *DeferredToolManager) RegisterTool(name string, t trpctool.Tool) {
	m.view.Load().tools[name] = t
}

// GetTool 返回已注册的延迟工具引用（装配期 RegisterTool 写入）。
func (m *DeferredToolManager) GetTool(name string) (trpctool.Tool, bool) {
	t, ok := m.view.Load().tools[name]
	return t, ok && t != nil
}

// GetToolDeclaration 返回已注册工具的完整声明。
// 供 tool_load 在激活成功后返回给模型。
func (m *DeferredToolManager) GetToolDeclaration(name string) *trpctool.Declaration {
	if t, ok := m.GetTool(name); ok {
		return t.Declaration()
	}
	return nil
}

// Activate 在当前 session 中激活指定工具。
// 写入 session state，供 ToolFilter 在后续请求中放行。
// 返回工具的完整声明（含 InputSchema，Name 为运行时名称），供 tool_load 返回给模型。
//
// 同时写入运行时名和基础名：
//   - 运行时名（如 file_save_file）：filter 在 NamedTool 层直接匹配
//   - 基础名（如 save_file）：DeferredCallableTool.Call 激活门禁检查的名字
func (m *DeferredToolManager) Activate(ctx context.Context, toolName string) (*trpctool.Declaration, error) {
	v := m.view.Load()
	idx, ok := v.catalogIndex[toolName]
	if !ok {
		return nil, fmt.Errorf("deferred tool %q not found in catalog", toolName)
	}
	entry := v.catalog[idx]

	// 写入 session state（运行时名 + 基础名，幂等）
	if !writeActivatedSet(ctx, toolName) {
		return nil, fmt.Errorf("failed to write activation state for tool %q", toolName)
	}
	if entry.BaseName != "" && entry.BaseName != toolName {
		writeActivatedSet(ctx, entry.BaseName)
	}

	// 返回完整声明（Name 覆盖为运行时名，模型按此名调用）
	decl := m.GetToolDeclaration(toolName)
	if decl == nil {
		return nil, fmt.Errorf("deferred tool %q has no registered tool reference", toolName)
	}
	if decl.Name != toolName {
		clone := *decl
		clone.Name = toolName
		decl = &clone
	}
	return decl, nil
}

// IsActivated 检查工具是否在当前 session 中已激活。
func (m *DeferredToolManager) IsActivated(ctx context.Context, toolName string) bool {
	return isActivatedForSession(ctx, toolName)
}

// IsInCatalog 检查工具是否在延迟目录中。
func (m *DeferredToolManager) IsInCatalog(toolName string) bool {
	_, ok := m.view.Load().catalogIndex[toolName]
	return ok
}

// ResolveName 将模型给出的工具名解析为目录运行时名。
// 支持：目录运行时名、唯一基础名（exec_command → hostexec_exec_command）、
// RuntimeToolNameAliases 别名链（shell_exec → exec_command → hostexec_exec_command）。
// 返回的第二个值表示是否解析成功。
func (m *DeferredToolManager) ResolveName(toolName string) (string, bool) {
	return m.view.Load().resolve(toolName)
}

// CatalogNames 返回目录中所有工具名称。
func (m *DeferredToolManager) CatalogNames() []string {
	v := m.view.Load()
	names := make([]string, len(v.catalog))
	for i, entry := range v.catalog {
		names[i] = entry.Name
	}
	return names
}

// Catalog 返回目录条目的副本。
// 供 catalog cue 渲染器生成静态工具目录。
func (m *DeferredToolManager) Catalog() []DeferredToolEntry {
	v := m.view.Load()
	out := make([]DeferredToolEntry, len(v.catalog))
	copy(out, v.catalog)
	return out
}

// CatalogSnapshot 返回当前视图的目录与预渲染静态 cue（单次原子加载）。
// catalog cue hook 每轮调用：热替换 SwapView 后下一轮即渲染新目录。
// 返回的 catalog 是视图共享切片（视图不可变，调用方只读安全）。
func (m *DeferredToolManager) CatalogSnapshot() ([]DeferredToolEntry, string) {
	v := m.view.Load()
	return v.catalog, v.staticCue
}

// CategoryIndex 返回分类索引。
func (m *DeferredToolManager) CategoryIndex() map[string][]string {
	v := m.view.Load()
	result := make(map[string][]string, len(v.categoryIndex))
	for k, val := range v.categoryIndex {
		cp := make([]string, len(val))
		copy(cp, val)
		result[k] = cp
	}
	return result
}

// ToolFilter 返回一个过滤器，隐藏未激活的延迟工具。
//
// 过滤器通过 ctx 中的 invocation 获取 session state，
// 检查每个延迟工具是否已在当前 session 中激活。
// 支持多层包装穿透：aliasTool/ToolDecorator/confirmationCallable 等
// 包装器通过 InnerTool() 或 Original() 暴露内部工具时，
// 递归解包直到找到延迟工具的规范名称。
//
// P0-2B 方案B：闭包绑定本稳定句柄、每次调用读取当前视图的 names——
// 热替换 SwapView 后同一闭包立即按新目录隐藏/放行，框架侧零改动。
func (m *DeferredToolManager) ToolFilter() trpctool.FilterFunc {
	return func(ctx context.Context, t trpctool.Tool) bool {
		name, isDeferred := resolveDeferredName(t, m.view.Load().names)
		if !isDeferred {
			return true
		}
		return isActivatedForSession(ctx, name)
	}
}

// resolveDeferredName 递归解包工具包装链，返回命中的延迟工具规范名称。
// 解包约定：优先 InnerTool()（deferred/alias 包装器），其次 Original()
// （ToolDecorator 等框架装饰器约定）。最多解包 8 层防循环。
func resolveDeferredName(t trpctool.Tool, deferredNames map[string]bool) (string, bool) {
	for i := 0; i < 8 && t != nil; i++ {
		decl := t.Declaration()
		if decl == nil {
			return "", false
		}
		if deferredNames[decl.Name] {
			return decl.Name, true
		}
		t = unwrapTool(t)
	}
	return "", false
}

// unwrapTool 解包一层工具包装器，无法解包时返回 nil。
func unwrapTool(t trpctool.Tool) trpctool.Tool {
	if u, ok := t.(interface{ InnerTool() trpctool.Tool }); ok {
		if inner := u.InnerTool(); inner != nil {
			return inner
		}
	}
	if u, ok := t.(interface{ Original() trpctool.Tool }); ok {
		return u.Original()
	}
	return nil
}

// ToolSearchTool 提供延迟工具搜索能力。
// 搜索仅返回匹配的工具信息，不再自动激活（WP-4 修复版）。
// 模型需要通过 tool_load 显式激活所需的工具。
type ToolSearchTool struct {
	tool    trpctool.CallableTool
	manager *DeferredToolManager
}

func NewToolSearchTool(catalog []DeferredToolEntry) *ToolSearchTool {
	manager := NewDeferredToolManager(catalog)
	t := &ToolSearchTool{
		manager: manager,
	}
	t.tool = trpcfunction.NewFunctionTool(
		t.execute,
		trpcfunction.WithName("tool_search"),
		trpcfunction.WithDescription("Search and discover available tools. Use this tool when you need a capability not listed in your current tool set. Returns matching tools with their names and descriptions. To use a discovered tool, call tool_load with the tool name."),
	)
	return t
}

func (t *ToolSearchTool) Manager() *DeferredToolManager {
	return t.manager
}

func (t *ToolSearchTool) Declaration() *trpctool.Declaration {
	return t.tool.Declaration()
}

func (t *ToolSearchTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	return t.tool.Call(ctx, jsonArgs)
}

func (t *ToolSearchTool) execute(ctx context.Context, in toolSearchInput) (toolSearchOutput, error) {
	queryLower := strings.ToLower(in.Query)
	tokens := strings.Fields(queryLower)
	catalog := t.manager.view.Load().catalog
	stats := buildCatalogStats(catalog)
	type scoredResult struct {
		result toolSearchResult
		score  float64
	}
	var scored []scoredResult
	for _, entry := range catalog {
		// P1-4 / B3：与语义预激活共享同一 BM25 打分，保证「搜索看到的」与
		// 「推荐的」一致。
		if score := scoreEntryAgainstQuery(entry, queryLower, tokens, stats); score > 0 {
			scored = append(scored, scoredResult{
				result: toolSearchResult{
					Name:        entry.Name,
					Description: entry.Description,
					Category:    entry.Category,
				},
				score: score,
			})
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].result.Name < scored[j].result.Name
	})
	results := make([]toolSearchResult, len(scored))
	for i, s := range scored {
		results[i] = s.result
	}
	// P1-4 漏斗度量（发现段）：搜索调用按是否有结果分桶。
	metrics.DeferredToolSearchTotal.WithLabelValues(strconv.FormatBool(len(results) > 0)).Inc()
	if len(results) == 0 {
		return toolSearchOutput{
			Tools:      []toolSearchResult{},
			Suggestion: fmt.Sprintf("No tools found matching %q. Try broader search terms or check available categories.", in.Query),
		}, nil
	}
	return toolSearchOutput{Tools: results}, nil
}

func (t *ToolSearchTool) CatalogNames() []string {
	return t.manager.CatalogNames()
}
