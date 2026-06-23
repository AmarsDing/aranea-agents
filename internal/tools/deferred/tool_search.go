package deferred

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"aranea-agents/pkg/apierror"

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

type DeferredToolEntry struct {
	Name        string
	Description string
	Category    string
	Factory     func(ctx context.Context) (trpctool.Tool, error)
}

type DeferredToolManager struct {
	mu            sync.RWMutex
	catalog       []DeferredToolEntry
	catalogIndex  map[string]int // name → index into catalog for O(1) lookup
	discovered    map[string]bool
	activated     map[string]trpctool.Tool
	activateCount map[string]int
	categoryIndex map[string][]string
}

func NewDeferredToolManager(catalog []DeferredToolEntry) *DeferredToolManager {
	m := &DeferredToolManager{
		catalog:       catalog,
		catalogIndex:  buildCatalogIndex(catalog),
		discovered:    make(map[string]bool),
		activated:     make(map[string]trpctool.Tool),
		activateCount: make(map[string]int),
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

func (m *DeferredToolManager) Activate(ctx context.Context, toolName string) (trpctool.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if t, ok := m.activated[toolName]; ok {
		return t, nil
	}
	idx, ok := m.catalogIndex[toolName]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainTool, "deferred tool %q not found in catalog", toolName)
	}
	entry := m.catalog[idx]
	if entry.Factory == nil {
		return nil, apierror.Internal(apierror.DomainTool, "deferred tool %q has no factory", toolName)
	}
	t, err := entry.Factory(ctx)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainTool, "deferred tool factory failed for %q: %v", toolName, err)
	}
	m.activated[toolName] = t
	m.activateCount[toolName]++
	return t, nil
}

func (m *DeferredToolManager) Discover(toolName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.discovered[toolName] = true
}

func (m *DeferredToolManager) IsActivated(toolName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.activated[toolName]; ok {
		return true
	}
	return m.discovered[toolName]
}

func (m *DeferredToolManager) ActivatedTools() []trpctool.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tools := make([]trpctool.Tool, 0, len(m.activated))
	for _, t := range m.activated {
		tools = append(tools, t)
	}
	return tools
}

func (m *DeferredToolManager) CatalogNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, len(m.catalog))
	for i, entry := range m.catalog {
		names[i] = entry.Name
	}
	return names
}

func (m *DeferredToolManager) DeferredToolNames() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make(map[string]bool, len(m.catalog))
	for _, entry := range m.catalog {
		names[entry.Name] = true
	}
	return names
}

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

func (m *DeferredToolManager) ActivateStats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := make(map[string]int, len(m.activateCount))
	for k, v := range m.activateCount {
		stats[k] = v
	}
	return stats
}

func (m *DeferredToolManager) ToolFilter() trpctool.FilterFunc {
	deferredNames := m.DeferredToolNames()
	return func(_ context.Context, t trpctool.Tool) bool {
		if t == nil || t.Declaration() == nil {
			return true
		}
		name := t.Declaration().Name
		if !deferredNames[name] {
			return true
		}
		return m.IsActivated(name)
	}
}

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
		trpcfunction.WithDescription("Search and discover available tools. Use this tool when you need a capability not listed in your current tool set. Returns matching tools with their names and descriptions. Discovered tools will be automatically available for use in subsequent requests."),
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
	tokens := strings.Fields(strings.ToLower(in.Query))
	type scoredResult struct {
		result toolSearchResult
		score  int
	}
	var scored []scoredResult
	for _, entry := range t.manager.catalog {
		nameLower := strings.ToLower(entry.Name)
		descLower := strings.ToLower(entry.Description)
		catLower := strings.ToLower(entry.Category)
		score := 0
		for _, token := range tokens {
			if nameLower == token {
				score += 10
			} else if strings.Contains(nameLower, token) {
				score += 5
			}
			if strings.Contains(catLower, token) {
				score += 3
			}
			if strings.Contains(descLower, token) {
				score += 2
			}
		}
		if score > 0 {
			scored = append(scored, scoredResult{
				result: toolSearchResult{
					Name:        entry.Name,
					Description: entry.Description,
					Category:    entry.Category,
				},
				score: score,
			})
			t.manager.Discover(entry.Name)
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
	if len(results) == 0 {
		return toolSearchOutput{
			Tools:      []toolSearchResult{},
			Suggestion: fmt.Sprintf("No tools found matching %q. Try broader search terms or check available categories.", in.Query),
		}, nil
	}
	return toolSearchOutput{Tools: results}, nil
}

func (t *ToolSearchTool) FindAndCreate(ctx context.Context, toolName string) (trpctool.Tool, error) {
	return t.manager.Activate(ctx, toolName)
}

func (t *ToolSearchTool) CatalogNames() []string {
	return t.manager.CatalogNames()
}
