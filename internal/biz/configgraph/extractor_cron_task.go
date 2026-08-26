package configgraph

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// errMissingCronTarget 标记 cron 目标类型要求的目标 id 缺失（配置错误，
// 以 broken 标记边 surfaced，不中断抽取）。
var errMissingCronTarget = errors.New("target id required by target_type is empty")

// cronTargetConfig 同构 cronrunner.cronTaskConfig 的建图子集（design §3.2
// #25 允许同构复制 + 表驱动测试）；target 归一化复刻 cronTargetType 语义。
type cronTargetConfig struct {
	TargetType string `json:"target_type"`
	TeamID     string `json:"team_id"`
}

func (cfg cronTargetConfig) normalizedTarget() string {
	target := strings.ToLower(strings.TrimSpace(cfg.TargetType))
	if target == "" && strings.TrimSpace(cfg.TeamID) != "" {
		target = "team"
	}
	if target == "" {
		target = "agent"
	}
	return target
}

// cronTaskExtractor: cron_task 节点 + runs（target_type 决定 dst 类型：
// team → config_json.team_id；agent → agent_id 直读列；model_registry_sync
// 等系统任务无边）。
type cronTaskExtractor struct{}

func (cronTaskExtractor) NodeType() string { return NodeTypeCronTask }

func (cronTaskExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListCronTasks(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeCronTask, r.ID, r.TaskKey, r.Name, r.WorkspaceID,
			statusFromDeletedAt(r.DeletedAt), nil))
	}
	return nodes, nil
}

func (cronTaskExtractor) ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error) {
	rows, err := src.ListCronTasks(ctx)
	if err != nil {
		return nil, err
	}
	var edges []Edge
	for _, r := range rows {
		if r.ID == "" || statusFromDeletedAt(r.DeletedAt) == NodeStatusDeleted {
			continue
		}
		var cfg cronTargetConfig
		if raw := strings.TrimSpace(r.ConfigJSON); raw != "" {
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				edges = append(edges, extractErrorEdge(NodeTypeCronTask, r.ID, EdgeTypeRuns, r.WorkspaceID,
					"cron_task", "config_json", "config_json", err))
				continue
			}
		}
		target := cfg.normalizedTarget()
		ev := withExtra(evidence("cron_task", "config_json", "config_json.target_type"), "target_type", target)
		switch target {
		case "team":
			teamID := strings.TrimSpace(cfg.TeamID)
			if teamID == "" {
				edges = append(edges, extractErrorEdge(NodeTypeCronTask, r.ID, EdgeTypeRuns, r.WorkspaceID,
					"cron_task", "config_json", "config_json.team_id", errMissingCronTarget))
				continue
			}
			edges = append(edges, Edge{
				SrcType: NodeTypeCronTask, SrcRef: r.ID,
				DstType: NodeTypeTeam, DstRef: teamID,
				Type:        EdgeTypeRuns,
				Evidence:    ev,
				WorkspaceID: r.WorkspaceID,
			})
		case "agent":
			agentID := strings.TrimSpace(r.AgentID)
			if agentID == "" {
				edges = append(edges, extractErrorEdge(NodeTypeCronTask, r.ID, EdgeTypeRuns, r.WorkspaceID,
					"cron_task", "agent_id", "agent_id", errMissingCronTarget))
				continue
			}
			edges = append(edges, Edge{
				SrcType: NodeTypeCronTask, SrcRef: r.ID,
				DstType: NodeTypeAgent, DstRef: agentID,
				Type:        EdgeTypeRuns,
				Evidence:    ev,
				WorkspaceID: r.WorkspaceID,
			})
		default:
			// model_registry_sync 等系统目标：无配置资产引用，不成边。
		}
	}
	return edges, nil
}
