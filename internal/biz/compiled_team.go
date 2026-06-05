package biz

import (
	"context"
	"time"
)

type NodeTaskMeta struct {
	RequiredRole             string `json:"required_role"`
	AssignmentMode           string `json:"assignment_mode"`
	AssignmentStrategy       string `json:"assignment_strategy"`
	ReviewerAgent            string `json:"reviewer_agent"`
	ReviewRules              string `json:"review_rules"`
	TimeoutSeconds           int    `json:"timeout_seconds"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
	EnableLeaseExtension     bool   `json:"enable_lease_extension"`
}

type RoleInfo struct {
	AgentID      string   `json:"agent_id"`
	AgentKey     string   `json:"agent_key"`
	DisplayName  string   `json:"display_name"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
}

type CompiledTeam struct {
	GraphBuildConfig
	TaskMeta       map[string]NodeTaskMeta `json:"task_meta"`
	RoleManifest   map[string]RoleInfo     `json:"role_manifest"`
	OriginalPolicy *TeamFailurePolicy      `json:"original_policy,omitempty"`
	CompiledAt     time.Time               `json:"compiled_at"`
}

type CompiledTeamRepo interface {
	Save(ctx context.Context, teamID, graphID, sessionID string, ct *CompiledTeam) error
	Load(ctx context.Context, teamID, graphID string) (*CompiledTeam, error)
	LoadForSession(ctx context.Context, teamID, graphID, sessionID string) (*CompiledTeam, error)
	Delete(ctx context.Context, teamID, graphID string) error
}

func NewCompiledTeam(cfg GraphBuildConfig, taskMeta map[string]NodeTaskMeta, roleManifest map[string]RoleInfo, originalPolicy *TeamFailurePolicy) *CompiledTeam {
	if taskMeta == nil {
		taskMeta = make(map[string]NodeTaskMeta)
	}
	if roleManifest == nil {
		roleManifest = make(map[string]RoleInfo)
	}
	return &CompiledTeam{
		GraphBuildConfig: cfg,
		TaskMeta:         taskMeta,
		RoleManifest:     roleManifest,
		OriginalPolicy:   originalPolicy,
		CompiledAt:       time.Now(),
	}
}

func (ct *CompiledTeam) TaskMetaForNode(nodeID string) (NodeTaskMeta, bool) {
	m, ok := ct.TaskMeta[nodeID]
	return m, ok
}

func (ct *CompiledTeam) RoleForNode(nodeID string) (RoleInfo, bool) {
	r, ok := ct.RoleManifest[nodeID]
	return r, ok
}
