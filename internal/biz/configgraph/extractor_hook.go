package configgraph

import (
	"context"
	"encoding/json"
	"strings"
)

// hookConfig 同构 internal/biz/hook Config 的 condition 子集（design §3.2 #27
// 同构反序列化），仅保留建图所需字段。
type hookConfig struct {
	Condition struct {
		AgentID  string `json:"agent_id"`
		ToolName string `json:"tool_name"`
	} `json:"condition"`
}

// hookExtractor: hooks 节点 + hook_ref（config_json.condition.agent_id →
// agent，值 uuid/agent_key 双解；condition.tool_name → tool，按 tool_key
// 双解）。condition 为空 = 全局 hook，不产生边。
type hookExtractor struct{}

func (hookExtractor) NodeType() string { return NodeTypeHook }

func (hookExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListHooks(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeHook, r.ID, r.HookKey, r.Name, "",
			statusFromDeletedAt(r.DeletedAt), nil))
	}
	return nodes, nil
}

func (hookExtractor) ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error) {
	rows, err := src.ListHooks(ctx)
	if err != nil {
		return nil, err
	}
	var edges []Edge
	for _, r := range rows {
		if r.ID == "" || statusFromDeletedAt(r.DeletedAt) == NodeStatusDeleted {
			continue
		}
		raw := strings.TrimSpace(r.ConfigJSON)
		if raw == "" {
			continue
		}
		var cfg hookConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			edges = append(edges, extractErrorEdge(NodeTypeHook, r.ID, EdgeTypeHookRef, "",
				"hooks", "config_json", "config_json.condition", err))
			continue
		}
		if agentRef := strings.TrimSpace(cfg.Condition.AgentID); agentRef != "" {
			edges = append(edges, Edge{
				SrcType: NodeTypeHook, SrcRef: r.ID,
				DstType: NodeTypeAgent, DstRef: agentRef, DstKey: agentRef,
				Type:     EdgeTypeHookRef,
				Evidence: evidence("hooks", "config_json", "config_json.condition.agent_id"),
			})
		}
		if toolName := strings.TrimSpace(cfg.Condition.ToolName); toolName != "" {
			edges = append(edges, Edge{
				SrcType: NodeTypeHook, SrcRef: r.ID,
				DstType: NodeTypeTool, DstRef: toolName, DstKey: toolName,
				Type:     EdgeTypeHookRef,
				Evidence: evidence("hooks", "config_json", "config_json.condition.tool_name"),
			})
		}
	}
	return edges, nil
}
