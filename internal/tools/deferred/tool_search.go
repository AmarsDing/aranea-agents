package deferred

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"aranea-agents/internal/metrics"

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

// DeferredToolManager 管理延迟工具的目录和 per-session 激活状态。
//
// 去 factory 化（WP-4 修复版）：
//   - 不再持有 factory 函数，不再负责工具创建
//   - catalog 仅包含静态元数据（name/description/category）
//   - tools map 持有已装配的 DeferredCallableTool 引用，供 tool_load 查询完整 schema
//   - 激活状态存储在 session state（temp:deferred:activated），per-session 隔离
type DeferredToolManager struct {
	mu            sync.RWMutex
	catalog       []DeferredToolEntry
	catalogIndex  map[string]int // name → index into catalog for O(1) lookup
	tools         map[string]trpctool.Tool
	categoryIndex map[string][]string
}

func NewDeferredToolManager(catalog []DeferredToolEntry) *DeferredToolManager {
	m := &DeferredToolManager{
		catalog:      catalog,
		catalogIndex: buildCatalogIndex(catalog),
		tools:        make(map[string]trpctool.Tool),
	}
	m.categoryIndex = buildCategoryIndex(catalog)
	return m
}

func buildCatalogIndex(catalog []DeferredToolEntry) map[string]int {
	idx := make(map[string]int, len(catalog))
	for i, entry := range catalog {
		idx[entry.Name] = i
	}
	return idx
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
func (m *DeferredToolManager) RegisterTool(name string, t trpctool.Tool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[name] = t
}

// GetToolDeclaration 返回已注册工具的完整声明。
// 供 tool_load 在激活成功后返回给模型。
func (m *DeferredToolManager) GetToolDeclaration(name string) *trpctool.Declaration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.tools[name]; ok && t != nil {
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
	m.mu.RLock()
	idx, ok := m.catalogIndex[toolName]
	var entry DeferredToolEntry
	if ok {
		entry = m.catalog[idx]
	}
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("deferred tool %q not found in catalog", toolName)
	}

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
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.catalogIndex[toolName]
	return ok
}

// CatalogNames 返回目录中所有工具名称。
func (m *DeferredToolManager) CatalogNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, len(m.catalog))
	for i, entry := range m.catalog {
		names[i] = entry.Name
	}
	return names
}

// Catalog 返回目录条目的副本。
// 供 catalog cue 渲染器生成静态工具目录。
func (m *DeferredToolManager) Catalog() []DeferredToolEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]DeferredToolEntry, len(m.catalog))
	copy(out, m.catalog)
	return out
}

// DeferredToolNames 返回延迟工具名称集合。
func (m *DeferredToolManager) DeferredToolNames() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make(map[string]bool, len(m.catalog))
	for _, entry := range m.catalog {
		names[entry.Name] = true
	}
	return names
}

// CategoryIndex 返回分类索引。
func (m *DeferredToolManager) CategoryIndex() map[string][]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string][]string, len(m.categoryIndex))
	for k, v := range m.categoryIndex {
		cp := make([]string, len(v))
		copy(cp, v)
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
func (m *DeferredToolManager) ToolFilter() trpctool.FilterFunc {
	deferredNames := m.DeferredToolNames()
	return func(ctx context.Context, t trpctool.Tool) bool {
		name, isDeferred := resolveDeferredName(t, deferredNames)
		if !isDeferred {
			return true
		}
		return isActivatedForSession(ctx, name)
	}
}

// resolveDeferredName 递归解包工具包装链，返回命中的延迟工具规范名称。
// 解包约定：优先 InnerTool()（deferred/alias 包装器），其次 Original()
//（ToolDecorator 等框架装饰器约定）。最多解包 8 层防循环。
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
	type scoredResult struct {
		result toolSearchResult
		score  int
	}
	var scored []scoredResult
	for _, entry := range t.manager.catalog {
		// P1-4：与语义预激活共享同一打分逻辑，保证「搜索看到的」与
		// 「推荐的」一致。
		if score := scoreEntryAgainstQuery(entry, queryLower, tokens); score > 0 {
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
