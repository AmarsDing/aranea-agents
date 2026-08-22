package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// --- fakes ---

type governFakeTool struct {
	decl *trpctool.Declaration
}

func (f *governFakeTool) Declaration() *trpctool.Declaration { return f.decl }
func (f *governFakeTool) Call(_ context.Context, args []byte) (any, error) {
	return "ok:" + string(args), nil
}

type governFakeStreamTool struct {
	governFakeTool
}

func (f *governFakeStreamTool) StreamableCall(_ context.Context, _ []byte) (*trpctool.StreamReader, error) {
	return nil, nil
}

type governFakeToolSet struct {
	name   string
	tools  []trpctool.Tool
	closed bool
}

func (s *governFakeToolSet) Name() string { return s.name }
func (s *governFakeToolSet) Tools(_ context.Context) []trpctool.Tool {
	return s.tools
}
func (s *governFakeToolSet) Close() error { s.closed = true; return nil }

func declWithDescription(desc string) *trpctool.Declaration {
	return &trpctool.Declaration{
		Name:        "t",
		Description: desc,
		InputSchema: &trpctool.Schema{Type: "object"},
	}
}

// --- declaration governance ---

func TestGovernMCPToolDeclaration_SmallDeclarationUnchanged(t *testing.T) {
	d := &trpctool.Declaration{
		Name:        "search",
		Description: "short description",
		InputSchema: &trpctool.Schema{
			Type:       "object",
			Properties: map[string]*trpctool.Schema{"q": {Type: "string", Description: "query"}},
		},
		OutputSchema: &trpctool.Schema{Type: "object"},
	}
	g := governMCPToolDeclaration(d)
	if g.Description != d.Description {
		t.Fatalf("description must stay intact, got %q", g.Description)
	}
	if g.OutputSchema == nil {
		t.Fatalf("small declaration must keep output schema")
	}
	if g.InputSchema.Properties["q"].Description != "query" {
		t.Fatalf("schema node description must stay intact")
	}
}

func TestGovernMCPToolDeclaration_LongDescriptionTruncatedAndInputNotMutated(t *testing.T) {
	long := strings.Repeat("描", 500)
	d := declWithDescription(long)
	// push the declaration over the soft cap so governance activates
	d.InputSchema.Properties = map[string]*trpctool.Schema{}
	for i := 0; i < 40; i++ {
		d.InputSchema.Properties[strings.Repeat("p", 20)+string(rune('a'+i%26))+strings.Repeat("x", i%7)] = &trpctool.Schema{
			Type:        "string",
			Description: strings.Repeat("属", 300),
		}
	}
	g := governMCPToolDeclaration(d)
	if got := len([]rune(g.Description)); got > mcpToolDescriptionMaxRunes+1 {
		t.Fatalf("description must be capped at %d runes, got %d", mcpToolDescriptionMaxRunes, got)
	}
	if !strings.HasSuffix(g.Description, "…") {
		t.Fatalf("truncated description must carry ellipsis, got %q", g.Description[len(g.Description)-3:])
	}
	// original must not be mutated (declarations may be shared via the pool)
	if len([]rune(d.Description)) != 500 {
		t.Fatalf("input declaration mutated: desc now %d runes", len([]rune(d.Description)))
	}
	for _, p := range d.InputSchema.Properties {
		if len([]rune(p.Description)) != 300 {
			t.Fatalf("input schema mutated: node desc now %d runes", len([]rune(p.Description)))
		}
	}
}

func TestGovernMCPToolDeclaration_StripsOutputSchemaWhenOverCap(t *testing.T) {
	d := declWithDescription("x")
	d.OutputSchema = &trpctool.Schema{
		Type:        "object",
		Description: strings.Repeat("o", 3000),
		Properties:  map[string]*trpctool.Schema{"r": {Type: "string", Description: strings.Repeat("z", 2000)}},
	}
	g := governMCPToolDeclaration(d)
	if g.OutputSchema != nil {
		t.Fatalf("oversized declaration must drop output schema (model only needs input schema)")
	}
	if d.OutputSchema == nil {
		t.Fatalf("input declaration must not be mutated")
	}
}

func TestGovernMCPToolDeclaration_TruncatesNestedSchemaDescriptions(t *testing.T) {
	d := declWithDescription(strings.Repeat("d", 400))
	// pad over the soft cap so governance activates
	d.Description += strings.Repeat("p", mcpToolDeclSoftCapChars)
	d.InputSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"items": {
				Type:        "array",
				Description: strings.Repeat("n", 500),
				Items: &trpctool.Schema{
					Type:        "object",
					Description: strings.Repeat("m", 500),
				},
			},
		},
	}
	g := governMCPToolDeclaration(d)
	node := g.InputSchema.Properties["items"]
	if got := len([]rune(node.Description)); got > mcpSchemaNodeDescMaxRunes+1 {
		t.Fatalf("property description must be capped, got %d runes", got)
	}
	if got := len([]rune(node.Items.Description)); got > mcpSchemaNodeDescMaxRunes+1 {
		t.Fatalf("nested items description must be capped, got %d runes", got)
	}
}

func TestGovernMCPToolDeclaration_CapsEnum(t *testing.T) {
	d := declWithDescription(strings.Repeat("d", 400) + strings.Repeat("p", mcpToolDeclSoftCapChars))
	enum := make([]any, 64)
	for i := range enum {
		enum[i] = i
	}
	d.InputSchema = &trpctool.Schema{
		Type:       "string",
		Enum:       enum,
		Properties: map[string]*trpctool.Schema{"a": {Type: "string", Enum: enum}},
	}
	g := governMCPToolDeclaration(d)
	if len(g.InputSchema.Enum) != mcpSchemaEnumMaxItems {
		t.Fatalf("enum must be capped at %d, got %d", mcpSchemaEnumMaxItems, len(g.InputSchema.Enum))
	}
	if len(g.InputSchema.Properties["a"].Enum) != mcpSchemaEnumMaxItems {
		t.Fatalf("nested enum must be capped")
	}
	if len(d.InputSchema.Enum) != 64 {
		t.Fatalf("input enum must not be mutated")
	}
}

// --- toolset-level governance ---

func TestGovernMCPServerToolSets_UnderBudgetKeepsDirectMode(t *testing.T) {
	sets := []trpctool.ToolSet{
		&governFakeToolSet{name: "s1", tools: []trpctool.Tool{
			&governFakeTool{decl: &trpctool.Declaration{Name: "a", Description: "small", InputSchema: &trpctool.Schema{Type: "object"}}},
			&governFakeTool{decl: &trpctool.Declaration{Name: "b", Description: "small2", InputSchema: &trpctool.Schema{Type: "object"}}},
		}},
	}
	rep := GovernMCPServerToolSets(context.Background(), sets, nil)
	if rep.Degraded {
		t.Fatalf("under budget must not degrade")
	}
	if rep.ToolCount != 2 {
		t.Fatalf("ToolCount = %d, want 2", rep.ToolCount)
	}
	if len(rep.Kept) != 1 {
		t.Fatalf("kept toolsets = %d, want 1", len(rep.Kept))
	}
	// tools are still reachable through the governed wrapper
	tools := rep.Kept[0].Tools(context.Background())
	if len(tools) != 2 {
		t.Fatalf("governed toolset must expose 2 tools, got %d", len(tools))
	}
	ct, ok := tools[0].(trpctool.CallableTool)
	if !ok {
		t.Fatalf("callable interface must be preserved")
	}
	res, err := ct.Call(context.Background(), []byte(`{"q":1}`))
	if err != nil || res != `ok:{"q":1}` {
		t.Fatalf("Call must delegate to inner, got %v %v", res, err)
	}
}

func TestGovernMCPServerToolSets_OverBudgetDegrades(t *testing.T) {
	// each tool: 30 properties with 60-char names ≈ 2K chars post-truncation;
	// 12 tools ≈ 24K+ chars — over the aggregate budget even after truncation.
	var tools []trpctool.Tool
	for i := 0; i < 12; i++ {
		props := map[string]*trpctool.Schema{}
		for j := 0; j < 30; j++ {
			// unique 50+ char names: names are never truncated, so the
			// post-governance size stays over the aggregate budget
			props[fmt.Sprintf("p%02d_%02d_%s", i, j, strings.Repeat("x", 60))] = &trpctool.Schema{Type: "string"}
		}
		tools = append(tools, &governFakeTool{decl: &trpctool.Declaration{
			Name:        fmt.Sprintf("tool_%02d", i),
			Description: "x",
			InputSchema: &trpctool.Schema{Type: "object", Properties: props},
		}})
	}
	sets := []trpctool.ToolSet{&governFakeToolSet{name: "big", tools: tools}}
	rep := GovernMCPServerToolSets(context.Background(), sets, nil)
	if !rep.Degraded {
		t.Fatalf("aggregate schema over budget must signal degradation")
	}
	if len(rep.Kept) != 1 {
		t.Fatalf("Kept must always be populated (caller decides close vs keep), got %d", len(rep.Kept))
	}
	if rep.ToolCount != 12 {
		t.Fatalf("ToolCount = %d, want 12", rep.ToolCount)
	}
	if rep.TotalChars <= mcpSchemaTotalBudgetChars {
		t.Fatalf("TotalChars = %d must exceed budget %d", rep.TotalChars, mcpSchemaTotalBudgetChars)
	}
}

func TestGovernMCPServerToolSets_ToolCountDegrade(t *testing.T) {
	tools := make([]trpctool.Tool, mcpSchemaToolCountDegrade)
	for i := range tools {
		tools[i] = &governFakeTool{decl: &trpctool.Declaration{
			Name:        fmt.Sprintf("t%02d", i),
			Description: "small",
			InputSchema: &trpctool.Schema{Type: "object"},
		}}
	}
	rep := GovernMCPServerToolSets(context.Background(), []trpctool.ToolSet{&governFakeToolSet{name: "many", tools: tools}}, nil)
	if !rep.Degraded {
		t.Fatal("MCP tool count >= 20 must degrade to broker even when chars are small")
	}
	if rep.ToolCount != mcpSchemaToolCountDegrade {
		t.Fatalf("ToolCount = %d, want %d", rep.ToolCount, mcpSchemaToolCountDegrade)
	}
}

func TestGovernMCPServerToolSets_BoundaryExactlyAtBudgetStaysDirect(t *testing.T) {
	// calibrate: build tools until just under budget using small fixed blocks
	mk := func(desc string) trpctool.Tool {
		return &governFakeTool{decl: declWithDescription(desc)}
	}
	// under soft cap each; aggregate below budget
	sets := []trpctool.ToolSet{&governFakeToolSet{name: "s", tools: []trpctool.Tool{mk("a"), mk("b")}}}
	rep := GovernMCPServerToolSets(context.Background(), sets, nil)
	if rep.Degraded {
		t.Fatalf("well under budget must stay direct")
	}
}

func TestGovernMCPToolWrapper_PreservesStreamable(t *testing.T) {
	inner := &governFakeStreamTool{governFakeTool{decl: declWithDescription(strings.Repeat("d", 500) + strings.Repeat("p", mcpToolDeclSoftCapChars))}}
	wrapped := governMCPToolIfNeeded(inner)
	st, ok := wrapped.(trpctool.StreamableTool)
	if !ok {
		t.Fatalf("streamable capability must be preserved")
	}
	if _, err := st.StreamableCall(context.Background(), nil); err != nil {
		t.Fatalf("StreamableCall must delegate: %v", err)
	}
	if got := len([]rune(wrapped.Declaration().Description)); got > mcpToolDescriptionMaxRunes+1 {
		t.Fatalf("declaration must be governed, desc %d runes", got)
	}
}

func TestGovernMCPToolIfNeeded_SmallToolPassesThrough(t *testing.T) {
	inner := &governFakeTool{decl: declWithDescription("tiny")}
	if got := governMCPToolIfNeeded(inner); got != trpctool.Tool(inner) {
		t.Fatalf("under-cap tool must pass through unwrapped")
	}
}

// --- assembly-level integration (degradation swap) ---

func overBudgetFakeTools() []trpctool.Tool {
	var tools []trpctool.Tool
	for i := 0; i < 12; i++ {
		props := map[string]*trpctool.Schema{}
		for j := 0; j < 30; j++ {
			props[fmt.Sprintf("p%02d_%02d_%s", i, j, strings.Repeat("x", 60))] = &trpctool.Schema{Type: "string"}
		}
		tools = append(tools, &governFakeTool{decl: &trpctool.Declaration{
			Name:        fmt.Sprintf("tool_%02d", i),
			Description: "x",
			InputSchema: &trpctool.Schema{Type: "object", Properties: props},
		}})
	}
	return tools
}

func swapMCPPool(t *testing.T, ts ToolSet) {
	t.Helper()
	testPool := newMCPToolSetPoolWithFactory(func(MCPServerConfig) (ToolSet, error) { return ts, nil }, time.Hour)
	saved := globalMCPToolSetPool
	globalMCPToolSetPool = testPool
	t.Cleanup(func() { globalMCPToolSetPool = saved })
}

func newMCPAssembleContext(servers []MCPServerConfig, broker, fallback *MCPBrokerConfig, brokerEnabled bool) *assembleContext {
	enabled := map[string]bool{}
	if brokerEnabled {
		enabled["mcpbroker"] = true
	}
	return &assembleContext{
		ctx:         context.Background(),
		cfg:         AssemblyConfig{MCP: MCPConfig{Servers: servers, Broker: broker, BrokerFallback: fallback}},
		out:         &AssembledToolsets{},
		enabled:     enabled,
		deferredSet: map[string]bool{},
		lg:          loggateway.NewNoop(),
	}
}

func toolNames(tools []Tool) map[string]bool {
	names := map[string]bool{}
	for _, t := range tools {
		if t != nil && t.Declaration() != nil {
			names[t.Declaration().Name] = true
		}
	}
	return names
}

// brokerToolNames 为 broker 工具族全量清单：4 个框架元工具 + 2 个业务层
// resources 工具（调用契约 7.4，mcp_resources.go，命名服务器非空即随族挂载）。
var brokerToolNames = []string{"mcp_list_servers", "mcp_list_tools", "mcp_inspect_tools", "mcp_call", "mcp_list_resources", "mcp_read_resource"}

func TestAssembleMCPTools_OverBudgetDegradesToBrokerFallback(t *testing.T) {
	swapMCPPool(t, &governFakeToolSet{name: "big-srv", tools: overBudgetFakeTools()})
	servers := []MCPServerConfig{{Name: "big-srv"}}
	ac := newMCPAssembleContext(servers, nil, &MCPBrokerConfig{Servers: servers}, false)

	if err := ac.assembleMCPTools(); err != nil {
		t.Fatalf("assembleMCPTools: %v", err)
	}
	if len(ac.out.ToolSets) != 0 {
		t.Fatalf("over-budget direct toolsets must be dropped, got %d", len(ac.out.ToolSets))
	}
	names := toolNames(ac.out.Tools)
	for _, want := range brokerToolNames {
		if !names[want] {
			t.Fatalf("broker tool %q must be mounted after degradation, got %v", want, names)
		}
	}
}

func TestAssembleMCPTools_DegradedWithExplicitBrokerNotDuplicated(t *testing.T) {
	swapMCPPool(t, &governFakeToolSet{name: "big-srv", tools: overBudgetFakeTools()})
	servers := []MCPServerConfig{{Name: "big-srv"}}
	ac := newMCPAssembleContext(servers, &MCPBrokerConfig{Servers: servers}, nil, true)

	if err := ac.assembleMCPTools(); err != nil {
		t.Fatalf("assembleMCPTools: %v", err)
	}
	if len(ac.out.ToolSets) != 0 {
		t.Fatalf("direct toolsets must be dropped")
	}
	if got := len(ac.out.Tools); got != len(brokerToolNames) {
		names := toolNames(ac.out.Tools)
		t.Fatalf("broker tools must be mounted exactly once, got %d tools: %v", got, names)
	}
}

func TestAssembleMCPTools_UnderBudgetKeepsGovernedToolSet(t *testing.T) {
	small := []trpctool.Tool{
		&governFakeTool{decl: &trpctool.Declaration{Name: "a", Description: "small", InputSchema: &trpctool.Schema{Type: "object"}}},
	}
	swapMCPPool(t, &governFakeToolSet{name: "s1", tools: small})
	servers := []MCPServerConfig{{Name: "s1"}}
	ac := newMCPAssembleContext(servers, nil, &MCPBrokerConfig{Servers: servers}, false)

	if err := ac.assembleMCPTools(); err != nil {
		t.Fatalf("assembleMCPTools: %v", err)
	}
	if len(ac.out.ToolSets) != 1 {
		t.Fatalf("under-budget must keep the governed toolset, got %d", len(ac.out.ToolSets))
	}
	if len(ac.out.Tools) != 0 {
		t.Fatalf("no broker tools expected when direct mode stays, got %v", toolNames(ac.out.Tools))
	}
	got := ac.out.ToolSets[0].Tools(context.Background())
	if len(got) != 1 || got[0].Declaration().Name != "a" {
		t.Fatalf("governed toolset must expose the original tools")
	}
}

func TestAssembleMCPTools_OverBudgetWithoutBrokerKeepsTruncatedDirect(t *testing.T) {
	swapMCPPool(t, &governFakeToolSet{name: "big-srv", tools: overBudgetFakeTools()})
	ac := newMCPAssembleContext([]MCPServerConfig{{Name: "big-srv"}}, nil, nil, false)

	if err := ac.assembleMCPTools(); err != nil {
		t.Fatalf("assembleMCPTools: %v", err)
	}
	if len(ac.out.ToolSets) != 1 {
		t.Fatalf("no broker available: truncated direct toolset must be kept, got %d", len(ac.out.ToolSets))
	}
}
