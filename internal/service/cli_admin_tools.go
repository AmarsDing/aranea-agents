package service

import (
	"context"
	"os"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/cli_admin"
	deliverabletools "aranea-agents/internal/tools/deliverable"
	"aranea-agents/internal/tools/memory_butler"
	orchtools "aranea-agents/internal/tools/orchestrator"
	"aranea-agents/internal/tools/skills_butler"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// sessionModelLookup resolves the effective provider/model for a Spirit session.
// It implements tools.SessionModelLookup for plan_and_execute "inherit" mode.
type sessionModelLookup struct {
	sessions biz.SessionTurnManager
}

func (s sessionModelLookup) GetSessionModel(ctx context.Context, sessionID string) (provider, model string) {
	if s.sessions == nil || sessionID == "" {
		return "", ""
	}
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return "", ""
	}
	if sess.LastProvider != "" && sess.LastModel != "" {
		return sess.LastProvider, sess.LastModel
	}
	return sess.DefaultProvider, sess.DefaultModel
}

type cliAdminSkillRepo struct {
	uc biz.CLIAdminSkillLister
}

func (r cliAdminSkillRepo) ListSkills(ctx context.Context, keyword string, limit, offset int32) ([]cli_admin.SkillItem, int32, error) {
	if r.uc == nil {
		return nil, 0, nil
	}
	page, err := r.uc.List(ctx, biz.SkillListQuery{
		Search: strings.TrimSpace(keyword),
		Limit:  int(limit),
		Offset: int(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]cli_admin.SkillItem, 0, len(page.Items))
	for _, s := range page.Items {
		items = append(items, skillItemFromBiz(s))
	}
	return items, int32(page.Total), nil
}

func (r cliAdminSkillRepo) GetSkill(ctx context.Context, id string) (*cli_admin.SkillItem, error) {
	if r.uc == nil {
		return nil, nil
	}
	s, err := r.uc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item := skillItemFromBiz(s)
	return &item, nil
}

type cliAdminAgentRepo struct {
	uc biz.CLIAdminAgentLister
}

func (r cliAdminAgentRepo) ListAgents(ctx context.Context, keyword string, limit, offset int32) ([]cli_admin.AgentItem, int32, error) {
	if r.uc == nil {
		return nil, 0, nil
	}
	page, err := r.uc.List(ctx, biz.AgentListQuery{
		Keyword: strings.TrimSpace(keyword),
		Limit:   int(limit),
		Offset:  int(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]cli_admin.AgentItem, 0, len(page.Items))
	for _, a := range page.Items {
		items = append(items, agentItemFromBiz(a))
	}
	return items, int32(page.Total), nil
}

func (r cliAdminAgentRepo) GetAgent(ctx context.Context, id string) (*cli_admin.AgentItem, error) {
	if r.uc == nil {
		return nil, nil
	}
	a, err := r.uc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item := agentItemFromBiz(a)
	return &item, nil
}

func (r cliAdminAgentRepo) GetAgentByAgentKey(ctx context.Context, agentKey string) (*cli_admin.AgentItem, error) {
	if r.uc == nil {
		return nil, nil
	}
	a, err := r.uc.GetByAgentKey(ctx, agentKey)
	if err != nil {
		return nil, err
	}
	item := agentItemFromBiz(a)
	return &item, nil
}

func (o *ChatOrchestrator) cliAdminTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
	if o == nil || !cli_admin.IsCLIAdminAllowed(strings.TrimSpace(ag.AgentKey)) {
		return nil
	}
	return cli_admin.RegisterAll(cli_admin.Deps{
		SkillRepo:           cliAdminSkillRepo{uc: o.td().ReadDeps.CLIAdminSkillUC},
		AgentRepo:           cliAdminAgentRepo{uc: o.td().ReadDeps.CLIAdminAgentUC},
		APIBaseURL:          cliAdminAPIBaseURL(ctx, o.td().ReadDeps.Settings),
		APIToken:            cliAdminAPIToken(),
		AllowedPrivateHosts: cliAdminAllowedPrivateHosts(),
	})
}

// cliAdminAllowedPrivateHosts reads ARANEA_ALLOWED_PRIVATE_HOSTS (comma-separated
// exact host names) exempting intranet Git servers from the SSRF public-host guard.
func cliAdminAllowedPrivateHosts() []string {
	v := strings.TrimSpace(os.Getenv("ARANEA_ALLOWED_PRIVATE_HOSTS"))
	if v == "" {
		return nil
	}
	var out []string
	for _, h := range strings.Split(v, ",") {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}

func (o *ChatOrchestrator) spiritCustomTools(ag biz.Agent) []trpctool.Tool {
	if o == nil || o.spiritAssembler() == nil {
		return nil
	}
	isSpirit := strings.TrimSpace(ag.AgentKey) == biz.SpiritAgentKey
	// Also allow agents with build_orchestration_graph in their ToolsAllowJSON
	var hasGraphToolAllowed bool
	if ag.Settings != nil {
		hasGraphToolAllowed = biz.ToolKeyInAllowJSON(ag.Settings.ToolsAllowJSON, "build_orchestration_graph")
	}
	if !isSpirit && !hasGraphToolAllowed {
		return nil
	}
	var out []trpctool.Tool

	// Spirit mode selection is handled by TaskPlanner.Plan() inside plan_and_execute.
	// The planner uses ComplexityRuleEngine (rule-based) + LLM (task decomposition) hybrid approach,
	// outputting OrchestrationStrategy (direct/single_agent/parallel/dag/coordinator).

	// New three-phase orchestration tools (Spirit-only, requires planner+allocator+orchestrator).
	if isSpirit {
		planner := o.team().TaskPlanner
		allocator := o.team().AgentAllocator
		orchestrator := o.team().TaskOrchestrator
		if planner != nil && allocator != nil && orchestrator != nil {
			out = append(out, tools.NewPlanAndExecuteTool(planner, allocator, orchestrator, o.spiritAssembler(), sessionModelLookup{sessions: o.td().Sessions}, o.td().Pipeline.EventBus, o.lg()))
			// check_progress removed: the system-push pattern (checkAllTeamsCompleted
			// → SynthesizeResults → ExecuteTurn) replaces the LLM-polling pattern.
			// The Spirit LLM no longer needs to poll team status; the system
			// automatically injects a synthesis message when all teams complete.
			out = append(out, tools.NewCancelOrchestrationTool(orchestrator, o.planBoardOrchFallback(), o.lg()))
		}

		// Synthesize results tool (still actively used for post-orchestration result synthesis).
		if o.spiritSynthesis() != nil {
			out = append(out, tools.NewSynthesizeResultsTool(o.spiritSynthesis()))
		}
	}

	// Graph orchestration tool for complex multi-agent DAG execution.
	// Available to Spirit agents and agents with build_orchestration_graph in ToolsAllowJSON.
	var graphBuilder orchtools.GraphBuilderPort
	if o.graphExec() != nil {
		graphBuilder = graphBuilderAdapter{exec: o.graphExec()}
	}
	out = append(out, orchtools.NewBuildOrchestrationGraphTool(graphBuilder))

	return out
}

func (o *ChatOrchestrator) skillsButlerTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
	if o == nil {
		return nil
	}
	if strings.TrimSpace(ag.AgentKey) != biz.SkillsAgentKey {
		return nil
	}
	return skills_butler.RegisterAll(skills_butler.Deps{
		Skills:    skillsButlerSkillUsecaseAdapter{uc: o.skillEvo()},
		Evolution: skillsButlerEvolutionAdapter{uc: o.evolution()},
		Queries:   skillsButlerQueryAdapter{reader: o.skillStats()},
		Analytics: skillsButlerAnalyticsAdapter{uc: o.expAnalytics(), agentID: ag.ID},
		LG:        o.lg(),
	})
}

func (o *ChatOrchestrator) memoryButlerTools(_ context.Context, ag biz.Agent) []trpctool.Tool {
	if o == nil {
		return nil
	}
	if strings.TrimSpace(ag.AgentKey) != biz.MemoryAgentKey {
		return nil
	}
	return memory_butler.RegisterAll(memory_butler.Deps{
		Analytics:   o.expAnalytics(),
		MemoryAdmin: o.td().Persist.Memory.AdminUsecase,
		Agents:      o.td().ReadDeps.Agents,
		LG:          o.lg(),
	})
}

// deliverableReaderTools assembles the P2 read_upstream_deliverable tool for
// every agent (read-only, low risk). Downstream team members use it to fetch
// an upstream team's full deliverable text when the injected summary is
// truncated; the tool is a thin adapter over biz.SpiritTeamController.
func (o *ChatOrchestrator) deliverableReaderTools() []trpctool.Tool {
	if o == nil || o.team().SpiritUC == nil {
		return nil
	}
	return []trpctool.Tool{deliverabletools.NewReadUpstreamDeliverableTool(o.team().SpiritUC, o.lg())}
}

func skillItemFromBiz(s biz.Skill) cli_admin.SkillItem {
	version := ""
	if s.CurrentVersion != nil {
		version = s.CurrentVersion.Version
	}
	return cli_admin.SkillItem{
		ID:          s.ID,
		SkillKey:    s.Slug,
		DisplayName: s.Name,
		Status:      s.Status,
		Version:     version,
	}
}

func agentItemFromBiz(a biz.Agent) cli_admin.AgentItem {
	return cli_admin.AgentItem{
		ID:          a.ID,
		AgentKey:    a.AgentKey,
		DisplayName: a.DisplayName,
		Provider:    a.Provider,
		Model:       a.Model,
		Status:      a.Status,
	}
}

func cliAdminAPIBaseURL(ctx context.Context, sys biz.SystemSettingRepo) string {
	for _, key := range []string{"ARANEA_CLI_ADMIN_BASE_URL", "ARANEA_BASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	if sys != nil {
		if s, err := sys.Get(ctx); err == nil && strings.TrimSpace(s.A2APublicBaseURL) != "" {
			return strings.TrimRight(strings.TrimSpace(s.A2APublicBaseURL), "/")
		}
	}
	return "http://127.0.0.1:8080"
}

func cliAdminAPIToken() string {
	for _, key := range []string{"ARANEA_CLI_ADMIN_TOKEN", "ARANEA_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// graphBuilderAdapter adapts biz.GraphExecutor to orchestrator.GraphBuilderPort.
// This bridges the gap between the biz-level graph execution port and the
// orchestrator tool's builder interface, enabling build_orchestration_graph
// to both generate and execute Graph DAGs.
type graphBuilderAdapter struct {
	exec biz.GraphExecutor
}

var _ orchtools.GraphBuilderPort = graphBuilderAdapter{}

func (a graphBuilderAdapter) BuildAndExecute(ctx context.Context, config biz.GraphBuildConfig, sessionID string) (string, error) {
	// Generate a deterministic graph ID from the config and session for idempotency.
	// Include sessionID to ensure uniqueness across concurrent Spirit sessions
	// that may share the same entry point (S-04 fix).
	graphID := "spirit_graph_" + config.EntryPoint + "_" + sessionID
	execID, err := a.exec.ExecuteGraphBuildConfig(ctx, graphID, sessionID, config, nil)
	if err != nil {
		return "", err
	}
	return execID, nil
}
