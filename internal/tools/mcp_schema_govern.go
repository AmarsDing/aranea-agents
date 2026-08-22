package tools

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// MCP schema governance (2026-08-14, 链路优化批次 P1-2).
//
// 背景：阶段 0 台账实测 tools_schema 占单轮输入 93%；MCP 直连模式
// （mcp_tool_set）把每个 server 的全部工具 declaration 原样注入 tools
// block，外部 server 的 schema 规模完全不可控（单个工具 300-500 token
// 起步，描述/枚举/嵌套属性无上限）。
//
// 治理三段（与研究结论一致：RAG-MCP / Hermes 实测 top-8 ≈ 1.4K token）：
//  1. 截断——单 declaration 超软上限：description 截断、剥掉 OutputSchema
//     （模型选型/构参只需要 input schema）、schema 树节点描述与枚举收 cap。
//  2. 预算——同一 agent 所有 MCP server 的治理后 declaration 总量设硬预算
//     （mcpSchemaTotalBudgetChars ≈ 4.6K token，约 8-12 个常规工具）。
//  3. 降级 broker——总量超预算时放弃直连注入，改用 broker 四个 meta tool
//     （list/inspect/call，schema 按需拉取），由装配层负责换装。
//
// 所有治理只作用于 Declaration 的拷贝：pooled toolset 跨 agent 共享，
// 原始 declaration 永不原地修改。

const (
	// mcpToolDeclSoftCapChars 是单工具 declaration 的软上限（JSON 序列化
	// 字符数）。典型工具 schema 300-500 token ≈ 1-1.8K 字符；2.4K 留有余量，
	// 超过才触发截断（大多数规范书写的工具不受影响）。
	mcpToolDeclSoftCapChars = 2400
	// mcpToolDescriptionMaxRunes 是工具 description 的 rune 上限。
	mcpToolDescriptionMaxRunes = 300
	// mcpSchemaNodeDescMaxRunes 是 schema 树每个节点 description 的 rune 上限。
	mcpSchemaNodeDescMaxRunes = 120
	// mcpSchemaEnumMaxItems 是枚举条目上限（过长枚举对模型选型边际收益极低）。
	mcpSchemaEnumMaxItems = 32
	// mcpSchemaTotalBudgetChars 是 MCP 直连模式 declaration 总量硬预算
	// （治理后口径）。≈ 4.6K token：够 8-12 个常规工具，超出说明 server
	// 工具面过大，直连注入性价比低于 broker 按需拉取。
	mcpSchemaTotalBudgetChars = 16000
	// mcpSchemaToolCountDegrade 是直连工具数硬上限（B3）：≥20 个远程工具
	// 时即使字符预算未满也降级 broker，避免首轮 schema 膨胀。
	mcpSchemaToolCountDegrade = 20
	// mcpSchemaToolCountDegradeCoding 是 coding / spirit（及空 profile）
	// 的更紧上限（F4）：对齐「top-8 ≈ 1.4K token」研究结论，默认走 broker。
	mcpSchemaToolCountDegradeCoding = 8
)

// MCPSchemaTotalBudgetChars 导出直连 declaration 总量硬预算，供 P0-2 阶段A
// 分片合并期在治理降级日志中报告（与装配期降级分支同一口径）。
func MCPSchemaTotalBudgetChars() int { return mcpSchemaTotalBudgetChars }

// MCPSchemaGovernanceReport 是 GovernMCPServerToolSets 的结果。
type MCPSchemaGovernanceReport struct {
	// ToolCount 是所有 server 的工具总数（治理前口径）。
	ToolCount int
	// TotalChars 是治理后 declaration 总量（JSON 序列化字符数）。
	TotalChars int
	// TruncatedCount 是被截断改写的工具数。
	TruncatedCount int
	// Degraded 为 true 表示总量超预算，调用方应降级到 broker 模式。
	Degraded bool
	// Kept 是治理包装后的 toolset（与输入一一对应），无论是否降级都会
	// 填充。降级且 broker 可用时，调用方 Close Kept（经包装器释放池引
	// 用）后丢弃；其余情况将 Kept 挂入装配结果。
	Kept []trpctool.ToolSet
}

// MCPSchemaToolCountDegradeForProfile returns the direct-mount tool-count
// cap used before falling back to broker. coding / spirit / empty follow
// the tighter top-8 budget; other profiles keep the global 20-tool cap.
func MCPSchemaToolCountDegradeForProfile(profile string) int {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "coding", "spirit", "":
		return mcpSchemaToolCountDegradeCoding
	default:
		return mcpSchemaToolCountDegrade
	}
}

// MCPSchemaPreferBrokerWithoutAllow reports whether a profile should drop
// direct MCP mounts as soon as any remote tool exists, unless the agent
// explicitly allow-listed mcp:<server> (F4).
func MCPSchemaPreferBrokerWithoutAllow(profile string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "coding", "spirit", "":
		return true
	default:
		return false
	}
}

// GovernMCPServerToolSets 对一组 MCP toolset 执行截断+预算治理。
// lg 可为 nil。调用方获得 Kept 的所有权。
func GovernMCPServerToolSets(ctx context.Context, sets []trpctool.ToolSet, lg loggateway.Logger) MCPSchemaGovernanceReport {
	return GovernMCPServerToolSetsAt(ctx, sets, lg, mcpSchemaToolCountDegrade)
}

// GovernMCPServerToolSetsAt is GovernMCPServerToolSets with an explicit
// tool-count degrade threshold. maxTools <= 0 falls back to the global cap.
func GovernMCPServerToolSetsAt(ctx context.Context, sets []trpctool.ToolSet, lg loggateway.Logger, maxTools int) MCPSchemaGovernanceReport {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if maxTools <= 0 {
		maxTools = mcpSchemaToolCountDegrade
	}
	rep := MCPSchemaGovernanceReport{}
	for _, ts := range sets {
		if ts == nil {
			continue
		}
		for _, t := range ts.Tools(ctx) {
			if t == nil || t.Declaration() == nil {
				continue
			}
			rep.ToolCount++
			g := governMCPToolDeclaration(t.Declaration())
			if g != t.Declaration() {
				rep.TruncatedCount++
			}
			rep.TotalChars += mcpDeclarationChars(g)
		}
	}
	rep.Degraded = rep.TotalChars > mcpSchemaTotalBudgetChars || rep.ToolCount >= maxTools
	rep.Kept = make([]trpctool.ToolSet, 0, len(sets))
	for _, ts := range sets {
		if ts == nil {
			continue
		}
		rep.Kept = append(rep.Kept, &mcpSchemaGovernedToolSet{inner: ts})
	}
	return rep
}

// mcpDeclarationChars 计量 declaration 注入 tools block 的字符开销。
// 口径与 context_budget.go 的 tools_schema 计量一致（json.Marshal 长度）。
func mcpDeclarationChars(d *trpctool.Declaration) int {
	raw, err := json.Marshal(d)
	if err != nil {
		return 0
	}
	return len(raw)
}

// governMCPToolDeclaration 返回治理后的 declaration。未超软上限时原样
// 返回输入指针（零分配）；超限时返回截断拷贝，输入不被修改。
func governMCPToolDeclaration(d *trpctool.Declaration) *trpctool.Declaration {
	if d == nil {
		return nil
	}
	if mcpDeclarationChars(d) <= mcpToolDeclSoftCapChars {
		return d
	}
	out := *d
	out.Description = strutil.TruncateRunesEllipsis(d.Description, mcpToolDescriptionMaxRunes)
	// OutputSchema 对工具选型/构参无贡献（模型只需要 input schema），剥掉。
	out.OutputSchema = nil
	out.InputSchema = governMCPSchemaNode(d.InputSchema)
	return &out
}

// governMCPSchemaNode 深拷贝 schema 树并截断节点描述/枚举。输入不被修改。
func governMCPSchemaNode(s *trpctool.Schema) *trpctool.Schema {
	if s == nil {
		return nil
	}
	out := *s
	out.Description = strutil.TruncateRunesEllipsis(s.Description, mcpSchemaNodeDescMaxRunes)
	if len(s.Enum) > mcpSchemaEnumMaxItems {
		out.Enum = append([]any(nil), s.Enum[:mcpSchemaEnumMaxItems]...)
	}
	if s.Properties != nil {
		props := make(map[string]*trpctool.Schema, len(s.Properties))
		for k, v := range s.Properties {
			props[k] = governMCPSchemaNode(v)
		}
		out.Properties = props
	}
	if s.Items != nil {
		out.Items = governMCPSchemaNode(s.Items)
	}
	if s.Defs != nil {
		defs := make(map[string]*trpctool.Schema, len(s.Defs))
		for k, v := range s.Defs {
			defs[k] = governMCPSchemaNode(v)
		}
		out.Defs = defs
	}
	if ap, ok := s.AdditionalProperties.(*trpctool.Schema); ok {
		out.AdditionalProperties = governMCPSchemaNode(ap)
	}
	return &out
}

// governMCPToolIfNeeded 包装超软上限的工具使其 Declaration 返回治理后
// 拷贝；未超限的工具原样返回（零包装开销）。Callable/Streamable 接口保留。
func governMCPToolIfNeeded(t trpctool.Tool) trpctool.Tool {
	if t == nil {
		return nil
	}
	d := t.Declaration()
	if d == nil {
		return t
	}
	g := governMCPToolDeclaration(d)
	if g == d {
		return t
	}
	if ct, ok := t.(trpctool.CallableTool); ok {
		gt := &mcpSchemaGovernedTool{inner: ct, decl: g}
		if st, ok := t.(trpctool.StreamableTool); ok {
			return &mcpSchemaGovernedStreamTool{mcpSchemaGovernedTool: gt, streamable: st}
		}
		return gt
	}
	return &mcpSchemaGovernedMetaTool{Tool: t, decl: g}
}

// mcpSchemaGovernedTool 包装 CallableTool：Declaration 返回治理后拷贝，
// Call 原样委托（治理只改描述，不改调用语义）。
type mcpSchemaGovernedTool struct {
	inner trpctool.CallableTool
	decl  *trpctool.Declaration
}

var _ trpctool.CallableTool = (*mcpSchemaGovernedTool)(nil)

func (g *mcpSchemaGovernedTool) Declaration() *trpctool.Declaration { return g.decl }
func (g *mcpSchemaGovernedTool) Call(ctx context.Context, args []byte) (any, error) {
	return g.inner.Call(ctx, args)
}

// mcpSchemaGovernedStreamTool 保留 StreamableTool 能力（与
// streamableToolDecorator 同模式：只有内部工具可流式才满足接口）。
type mcpSchemaGovernedStreamTool struct {
	*mcpSchemaGovernedTool
	streamable trpctool.StreamableTool
}

var _ trpctool.StreamableTool = (*mcpSchemaGovernedStreamTool)(nil)

func (g *mcpSchemaGovernedStreamTool) StreamableCall(ctx context.Context, args []byte) (*trpctool.StreamReader, error) {
	return g.streamable.StreamableCall(ctx, args)
}

// mcpSchemaGovernedMetaTool 包装不可调用的纯元数据工具。
type mcpSchemaGovernedMetaTool struct {
	trpctool.Tool
	decl *trpctool.Declaration
}

func (g *mcpSchemaGovernedMetaTool) Declaration() *trpctool.Declaration { return g.decl }

// mcpSchemaGovernedToolSet 在每次 Tools(ctx) 时对成员工具做截断治理
// （与 decoratedToolSet 同模式：每次调用新鲜包装，感知 tools/list 刷新）。
type mcpSchemaGovernedToolSet struct {
	inner trpctool.ToolSet
}

var _ trpctool.ToolSet = (*mcpSchemaGovernedToolSet)(nil)

func (s *mcpSchemaGovernedToolSet) Name() string { return s.inner.Name() }
func (s *mcpSchemaGovernedToolSet) Close() error { return s.inner.Close() }

// InvalidateToolsCache forwards to the inner MCP ToolSet so mid-turn
// catalog refresh (E8) can expire the 5-minute tools/list cache.
func (s *mcpSchemaGovernedToolSet) InvalidateToolsCache() {
	if s == nil {
		return
	}
	if v, ok := s.inner.(MCPCacheInvalidator); ok {
		v.InvalidateToolsCache()
	}
}

func (s *mcpSchemaGovernedToolSet) Tools(ctx context.Context) []trpctool.Tool {
	raw := s.inner.Tools(ctx)
	if len(raw) == 0 {
		return raw
	}
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		out[i] = governMCPToolIfNeeded(t)
	}
	return out
}
