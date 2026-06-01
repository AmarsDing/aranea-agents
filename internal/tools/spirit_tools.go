package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type AssembleTeamInput struct {
	AgentKeys   []string `json:"agent_keys"  jsonschema:"description=参与团队的 Agent key 列表"`
	Mode        string   `json:"mode"         jsonschema:"description=团队编排模式,enum=coordinator,enum=sequential,enum=parallel"`
	TaskPrompt  string   `json:"task_prompt"  jsonschema:"description=任务描述，用于生成团队名称和成员指令"`
	TaskDAGJSON string   `json:"task_dag_json,omitempty" jsonschema:"description=任务 DAG 的 JSON 描述（可选），包含节点和依赖关系。格式为 [{id,task_name,description,depends_on,mode,agent_keys}]"`
	AutoStart   *bool    `json:"auto_start,omitempty"  jsonschema:"description=是否自动启动团队执行（默认 true）"`
}

type AssembleTeamOutput struct {
	TeamID         string `json:"team_id"`
	SessionID      string `json:"session_id"`
	TeamName       string `json:"team_name"`
	TopologyReason string `json:"topology_reason,omitempty"`
	DAGDiagram     string `json:"dag_diagram,omitempty"`
}

type TeamProgressView struct {
	TeamID      string  `json:"team_id"`
	TeamName    string  `json:"team_name"`
	Status      string  `json:"status"`
	ProgressPct float64 `json:"progress_pct"`
	CurrentStep string  `json:"current_step"`
	DurationMs  int64   `json:"duration_ms"`
}

type SpiritTeamAssemblerPort interface {
	AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, error)
	SuggestTopology(ctx context.Context, taskDescription string) (string, bool)
}

type SpiritTeamQueryPort interface {
	ListActiveTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
	ListAllTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
	GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int
}

type SpiritTeamControllerPort interface {
	CancelTeam(ctx context.Context, teamID string) error
	CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]biz.TeamProgress, error)
}

type SpiritSynthesisPort interface {
	SynthesizeResults(ctx context.Context, spiritSessionID string, strategy string) (*biz.SynthesisOutput, error)
}

func NewAssembleTeamTool(assembler SpiritTeamAssemblerPort, query SpiritTeamQueryPort, lg loggateway.Logger) *trpcfunction.FunctionTool[AssembleTeamInput, AssembleTeamOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input AssembleTeamInput) (AssembleTeamOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return AssembleTeamOutput{}, kerrors.BadRequest("SPIRIT", "spirit session id not found in context")
			}

			activeTeams, err := query.ListActiveTeams(ctx, spiritSessionID)
			if err != nil {
				return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "query active teams: "+err.Error())
			}
			maxParallel := query.GetMaxParallelTeams(ctx, spiritSessionID)
			if len(activeTeams) >= maxParallel {
				return AssembleTeamOutput{}, kerrors.BadRequest(
					"SPIRIT",
					fmt.Sprintf("max parallel teams (%d) reached, wait for existing teams to complete", maxParallel),
				)
			}

			mode := strings.TrimSpace(input.Mode)
			var topologyReason string

			dag, dagErr := biz.ParseTaskDAG(strings.TrimSpace(input.TaskDAGJSON), lg)
			if dagErr != nil {
				return AssembleTeamOutput{}, kerrors.BadRequest("SPIRIT", "invalid task dag: "+dagErr.Error())
			}

			if dag != nil && len(dag.Nodes) > 0 {
				routed := dag.RouteTopology()
				if mode == "" {
					mode = string(routed)
				}
				topologyReason = biz.FormatTopologyReason(routed, false, dag)
			}

			if cached, found := assembler.SuggestTopology(ctx, strings.TrimSpace(input.TaskPrompt)); found && cached != "" {
				if mode == "" {
					mode = cached
				}
				if topologyReason == "" {
					topologyReason = fmt.Sprintf("基于历史编排缓存推荐拓扑: %s", cached)
				}
			}
			if mode == "" {
				mode = "coordinator"
			}
			autoStart := true
			if input.AutoStart != nil {
				autoStart = *input.AutoStart
			}
			params := biz.SpiritTeamParams{
				SpiritSessionID:    spiritSessionID,
				TaskDescription:    strings.TrimSpace(input.TaskPrompt),
				AgentKeys:          input.AgentKeys,
				Mode:               mode,
				ParallelConfigJSON: buildParallelConfigJSON(maxParallel),
				TopologyReason:     topologyReason,
				AutoStart:          autoStart,
			}

			if dag != nil && len(dag.Nodes) > 1 {
				outputs, err := assembleDAGTeams(ctx, assembler, dag, spiritSessionID, mode, input.AgentKeys, maxParallel, autoStart)
				if err != nil {
					return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "assemble dag teams: "+err.Error())
				}
				if len(outputs) > 0 {
					outputs[0].DAGDiagram = dag.ToTextDiagram()
					outputs[0].TopologyReason = topologyReason
					return outputs[0], nil
				}
				return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "no teams created from dag")
			}

			if dag != nil && len(dag.Nodes) == 1 {
				for _, node := range dag.OrderedNodes() {
					params.DagNodeID = string(node.ID)
					dependsOn := make([]string, len(node.DependsOn))
					for i, d := range node.DependsOn {
						dependsOn[i] = string(d)
					}
					params.DependsOn = dependsOn
					break
				}
			}

			team, session, err := assembler.AssembleTeam(ctx, params)
			if err != nil {
				return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "assemble team: "+err.Error())
			}
			return AssembleTeamOutput{
				TeamID:         team.ID,
				SessionID:      session.ID,
				TeamName:       team.DisplayName,
				TopologyReason: topologyReason,
			}, nil
		},
		trpcfunction.WithName("assemble_team"),
		trpcfunction.WithDescription("组建任务团队。当用户需求复杂、需要多 Agent 协作时调用此工具。支持同一精灵会话下并行组建多个团队。系统会根据历史编排缓存自动推荐最优拓扑。简单问答请直接回复，无需调用此工具。"),
	)
}

type CheckTeamProgressInput struct{}

type CheckTeamProgressOutput struct {
	Teams []TeamProgressView `json:"teams"`
}

func NewCheckTeamProgressTool(controller SpiritTeamControllerPort) *trpcfunction.FunctionTool[CheckTeamProgressInput, CheckTeamProgressOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, _ CheckTeamProgressInput) (CheckTeamProgressOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return CheckTeamProgressOutput{}, kerrors.BadRequest("SPIRIT", "spirit session id not found in context")
			}
			progress, err := controller.CheckTeamProgress(ctx, spiritSessionID)
			if err != nil {
				return CheckTeamProgressOutput{}, kerrors.InternalServer("SPIRIT", "check team progress: "+err.Error())
			}
			views := make([]TeamProgressView, 0, len(progress))
			for _, p := range progress {
				views = append(views, TeamProgressView{
					TeamID:      p.TeamID,
					TeamName:    p.TeamName,
					Status:      p.Status,
					ProgressPct: p.ProgressPct,
					CurrentStep: p.CurrentStep,
					DurationMs:  p.DurationMs,
				})
			}
			return CheckTeamProgressOutput{Teams: views}, nil
		},
		trpcfunction.WithName("check_team_progress"),
		trpcfunction.WithDescription("查询当前精灵会话下所有团队的执行进度。用于监控并行任务的推进情况。"),
	)
}

type CancelTeamInput struct {
	TeamID string `json:"team_id" jsonschema:"description=要取消的团队 ID"`
}

type CancelTeamOutput struct {
	TeamID string `json:"team_id"`
	Status string `json:"status"`
}

func NewCancelTeamTool(controller SpiritTeamControllerPort) *trpcfunction.FunctionTool[CancelTeamInput, CancelTeamOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input CancelTeamInput) (CancelTeamOutput, error) {
			teamID := strings.TrimSpace(input.TeamID)
			if teamID == "" {
				return CancelTeamOutput{}, kerrors.BadRequest("SPIRIT", "team_id is required")
			}
			err := controller.CancelTeam(ctx, teamID)
			if err != nil {
				return CancelTeamOutput{}, err
			}
			return CancelTeamOutput{TeamID: teamID, Status: "cancelled"}, nil
		},
		trpcfunction.WithName("cancel_team"),
		trpcfunction.WithDescription("取消正在运行的团队。取消后释放并行团队配额。已完成的团队不可取消。"),
	)
}

type SynthesizeResultsInput struct {
	Strategy string `json:"strategy,omitempty" jsonschema:"description=合成策略,enum=template,enum=prompt,enum=hybrid"`
}

type SynthesizeResultsOutput struct {
	Content     string                    `json:"content"`
	Strategy    string                    `json:"strategy"`
	TeamResults []biz.TeamSynthesisResult `json:"team_results"`
}

func NewSynthesizeResultsTool(synthesis SpiritSynthesisPort) *trpcfunction.FunctionTool[SynthesizeResultsInput, SynthesizeResultsOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input SynthesizeResultsInput) (SynthesizeResultsOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return SynthesizeResultsOutput{}, kerrors.BadRequest("SPIRIT", "spirit session id not found in context")
			}
			output, err := synthesis.SynthesizeResults(ctx, spiritSessionID, input.Strategy)
			if err != nil {
				return SynthesizeResultsOutput{}, kerrors.InternalServer("SPIRIT", "synthesize results: "+err.Error())
			}
			return SynthesizeResultsOutput{
				Content:     output.Content,
				Strategy:    string(output.Strategy),
				TeamResults: output.TeamResults,
			}, nil
		},
		trpcfunction.WithName("synthesize_results"),
		trpcfunction.WithDescription("合成所有已完成团队的执行结果。当所有并行团队完成后调用此工具，将各团队结果整合为综合报告。"),
	)
}

type ListButlersInput struct{}

type ListButlersOutput struct {
	Butlers []ButlerInfo `json:"butlers"`
}

type ButlerInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func NewListButlersTool() *trpcfunction.FunctionTool[ListButlersInput, ListButlersOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, _ ListButlersInput) (ListButlersOutput, error) {
			return ListButlersOutput{
				Butlers: []ButlerInfo{
					{Name: "orchestrator", DisplayName: "编排管家", Description: "负责分析任务、选择 Agent、组建团队", Status: "available"},
				},
			}, nil
		},
		trpcfunction.WithName("list_butlers"),
		trpcfunction.WithDescription("列出可用的管家列表。精灵可以委派任务给不同的管家。"),
	)
}

type QueryButlerStatusInput struct {
	ButlerName string `json:"butler_name" jsonschema:"description=管家名称"`
}

type QueryButlerStatusOutput struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	ActiveTasks int    `json:"active_tasks"`
}

func NewQueryButlerStatusTool() *trpcfunction.FunctionTool[QueryButlerStatusInput, QueryButlerStatusOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input QueryButlerStatusInput) (QueryButlerStatusOutput, error) {
			return QueryButlerStatusOutput{
				Name:        strings.TrimSpace(input.ButlerName),
				Status:      "available",
				ActiveTasks: 0,
			}, nil
		},
		trpcfunction.WithName("query_butler_status"),
		trpcfunction.WithDescription("查询指定管家的当前状态和活跃任务数。"),
	)
}

func spiritSessionIDFromCtx(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	if inv.Session != nil {
		return inv.Session.ID
	}
	return ""
}

func buildParallelConfigJSON(maxConcurrent int) string {
	if maxConcurrent <= 0 {
		return ""
	}
	cfg := biz.ParallelConfig{
		MaxConcurrentTeams: maxConcurrent,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return string(b)
}

func taskNodeIDsToStrings(ids []biz.TaskNodeID) []string {
	if len(ids) == 0 {
		return nil
	}
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = string(id)
	}
	return result
}

func assembleDAGTeams(ctx context.Context, assembler SpiritTeamAssemblerPort, dag *biz.TaskDAG, spiritSessionID, mode string, agentKeys []string, maxParallel int, autoStart bool) ([]AssembleTeamOutput, error) {
	var outputs []AssembleTeamOutput
	for _, node := range dag.OrderedNodes() {
		nodeAgentKeys := node.AgentKeys
		if len(nodeAgentKeys) == 0 {
			nodeAgentKeys = agentKeys
		}
		nodeMode := mode
		if node.Mode != "" {
			nodeMode = node.Mode
		}
		dependsOn := make([]string, len(node.DependsOn))
		for i, d := range node.DependsOn {
			dependsOn[i] = string(d)
		}
		params := biz.SpiritTeamParams{
			SpiritSessionID:    spiritSessionID,
			TaskDescription:    node.Description,
			AgentKeys:          nodeAgentKeys,
			Mode:               nodeMode,
			DagNodeID:          string(node.ID),
			DependsOn:          taskNodeIDsToStrings(node.DependsOn),
			ParallelConfigJSON: buildParallelConfigJSON(maxParallel),
			AutoStart:          autoStart,
		}
		team, session, err := assembler.AssembleTeam(ctx, params)
		if err != nil {
			return outputs, err
		}
		outputs = append(outputs, AssembleTeamOutput{
			TeamID:    team.ID,
			SessionID: session.ID,
			TeamName:  team.DisplayName,
		})
	}
	return outputs, nil
}
