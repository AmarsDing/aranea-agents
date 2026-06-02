package biz

type NodeTaskMeta struct {
	RequiredRole             string
	AssignmentMode           string
	AssignmentStrategy       string
	ReviewerAgent            string
	ReviewRules              string
	TimeoutSeconds           int
	HeartbeatIntervalSeconds int
	EnableLeaseExtension     bool
}

type RoleInfo struct {
	AgentID      string
	AgentKey     string
	DisplayName  string
	Role         string
	Capabilities []string
}

type CompiledTeam struct {
	GraphBuildConfig
	RoleManifest   map[string]RoleInfo
	OriginalPolicy *TeamFailurePolicy
}

func NewCompiledTeam(cfg GraphBuildConfig, roleManifest map[string]RoleInfo, originalPolicy *TeamFailurePolicy) *CompiledTeam {
	if cfg.TaskMeta == nil {
		cfg.TaskMeta = make(map[string]NodeTaskMeta)
	}
	if roleManifest == nil {
		roleManifest = make(map[string]RoleInfo)
	}
	return &CompiledTeam{
		GraphBuildConfig: cfg,
		RoleManifest:     roleManifest,
		OriginalPolicy:   originalPolicy,
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
