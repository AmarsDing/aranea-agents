package graph

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	nl2graphLLMTimeout = 60 * time.Second
	agentInvokeFuncRef = "agent.invoke"

	templateDAG = "dag"
)

var fenceRegexp = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")

// NL2GraphConverter converts natural language task descriptions into graph
// build configurations using LLM analysis.
type NL2GraphConverter interface {
	Convert(ctx context.Context, taskDesc string, availableAgents []biz.AgentCapability) (*biz.GraphBuildConfig, error)
}

// NL2GraphConverterImpl implements NL2GraphConverter using LLM analysis.
// It analyzes the task, matches a template, fills nodes with available agents,
// validates the DAG, and falls back to a sequential pipeline on validation failure.
type NL2GraphConverterImpl struct {
	llm trpcmodel.Model
	lg  loggateway.Logger
}

var _ NL2GraphConverter = (*NL2GraphConverterImpl)(nil)

// NewNL2GraphConverter creates a new NL2GraphConverter implementation.
// The llm parameter may be nil when no planner model is configured; in that
// case Convert returns an Internal error so callers can fall back.
func NewNL2GraphConverter(llm trpcmodel.Model, lg loggateway.Logger) *NL2GraphConverterImpl {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &NL2GraphConverterImpl{
		llm: llm,
		lg:  lg.With(loggateway.Domain("nl2graph")),
	}
}

// Convert transforms a natural language task description into a GraphBuildConfig.
// It calls the LLM to analyze the task, builds a graph config from the analysis,
// validates the DAG, and falls back to a sequential pipeline if validation fails.
func (c *NL2GraphConverterImpl) Convert(ctx context.Context, taskDesc string, availableAgents []biz.AgentCapability) (*biz.GraphBuildConfig, error) {
	if strings.TrimSpace(taskDesc) == "" {
		return nil, apierror.BadRequest(apierror.DomainGraph, "task description is required")
	}

	if c.llm == nil {
		return nil, apierror.Internal(apierror.DomainGraph, "NL2Graph LLM not configured")
	}

	analysis, err := c.analyzeTask(ctx, taskDesc, availableAgents)
	if err != nil {
		return nil, err
	}

	// If LLM returned no parseable subtasks, fall back to a single-node
	// sequential pipeline representing the whole task.
	if len(analysis.Subtasks) == 0 {
		c.lg.Warn("NL2Graph no subtasks, falling back to single-node sequential pipeline",
			loggateway.StepID("nl2graph.empty_fallback"),
		)
		return c.fallbackSingle(taskDesc, availableAgents), nil
	}

	config := c.buildConfig(analysis, availableAgents)

	if !validateDAG(config) {
		c.lg.Warn("NL2Graph DAG validation failed, falling back to sequential pipeline",
			loggateway.StepID("nl2graph.fallback"),
		)
		config = c.fallbackSequential(analysis, availableAgents)
	}

	return config, nil
}

// analyzeTask calls the LLM to analyze the task and identify subtasks with
// dependencies. Returns an error only on LLM call failure; malformed JSON
// yields an empty analysis so the caller can fall back.
func (c *NL2GraphConverterImpl) analyzeTask(ctx context.Context, taskDesc string, availableAgents []biz.AgentCapability) (taskAnalysis, error) {
	prompt := buildNL2GraphPrompt(taskDesc, availableAgents)

	callCtx, cancel := context.WithTimeout(ctx, nl2graphLLMTimeout)
	defer cancel()

	req := trpcmodel.NewRequest([]trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: nl2graphSystemPrompt()},
		{Role: trpcmodel.RoleUser, Content: prompt},
	})

	respChan, err := c.llm.GenerateContent(callCtx, req)
	if err != nil {
		return taskAnalysis{}, apierror.Internal(apierror.DomainGraph, "LLM generate content").WithCause(err)
	}

	text, err := consumeModelResponse(respChan)
	if err != nil {
		return taskAnalysis{}, err
	}

	return c.parseTaskAnalysis(text)
}

// parseTaskAnalysis parses the LLM response as JSON. On failure, logs a warning
// and returns an empty analysis (caller falls back to sequential).
func (c *NL2GraphConverterImpl) parseTaskAnalysis(text string) (taskAnalysis, error) {
	text = stripFences(text)
	var analysis taskAnalysis
	if err := json.Unmarshal([]byte(text), &analysis); err != nil {
		c.lg.Warn("NL2Graph LLM returned malformed JSON, using fallback strategy",
			loggateway.StepID("nl2graph.parse_fallback"),
			loggateway.Err(err),
		)
		return taskAnalysis{}, nil
	}
	return analysis, nil
}

// buildConfig constructs a GraphBuildConfig from the task analysis and
// available agents. Each subtask becomes an agent node; depends_on entries
// become edges. DAG template uses EngineDAG; others use EngineBSP.
func (c *NL2GraphConverterImpl) buildConfig(analysis taskAnalysis, agents []biz.AgentCapability) *biz.GraphBuildConfig {
	nodes := make([]biz.NodeDef, 0, len(analysis.Subtasks))
	for _, st := range analysis.Subtasks {
		nodes = append(nodes, biz.NodeDef{
			ID:          st.ID,
			Type:        biz.NodeTypeAgent,
			Description: st.Description,
			AgentName:   matchAgent(st, agents),
			FuncRef:     agentInvokeFuncRef,
		})
	}

	edges := make([]biz.EdgeDef, 0)
	for _, st := range analysis.Subtasks {
		for _, dep := range st.DependsOn {
			edges = append(edges, biz.EdgeDef{
				From: dep,
				To:   st.ID,
				Kind: biz.EdgeKindFlow,
			})
		}
	}

	engine := biz.EngineBSP
	if analysis.Template == templateDAG {
		engine = biz.EngineDAG
	}

	return &biz.GraphBuildConfig{
		Nodes:           nodes,
		Edges:           edges,
		EntryPoint:      analysis.EntryPoint,
		FinishPoint:     analysis.FinishPoint,
		ExecutionEngine: engine,
	}
}

// fallbackSequential builds a linear sequential pipeline from the subtasks
// in their declared order, ignoring original dependencies.
func (c *NL2GraphConverterImpl) fallbackSequential(analysis taskAnalysis, agents []biz.AgentCapability) *biz.GraphBuildConfig {
	nodes := make([]biz.NodeDef, 0, len(analysis.Subtasks))
	edges := make([]biz.EdgeDef, 0)

	for i, st := range analysis.Subtasks {
		nodes = append(nodes, biz.NodeDef{
			ID:          st.ID,
			Type:        biz.NodeTypeAgent,
			Description: st.Description,
			AgentName:   matchAgent(st, agents),
			FuncRef:     agentInvokeFuncRef,
		})
		if i > 0 {
			edges = append(edges, biz.EdgeDef{
				From: analysis.Subtasks[i-1].ID,
				To:   st.ID,
				Kind: biz.EdgeKindFlow,
			})
		}
	}

	return &biz.GraphBuildConfig{
		Nodes:           nodes,
		Edges:           edges,
		EntryPoint:      analysis.Subtasks[0].ID,
		FinishPoint:     analysis.Subtasks[len(analysis.Subtasks)-1].ID,
		ExecutionEngine: biz.EngineBSP,
	}
}

// fallbackSingle builds a single-node sequential pipeline representing the
// entire task. Used when LLM output is unparseable.
func (c *NL2GraphConverterImpl) fallbackSingle(taskDesc string, agents []biz.AgentCapability) *biz.GraphBuildConfig {
	agentName := ""
	if len(agents) > 0 {
		agentName = agents[0].AgentKey
	}
	return &biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{
				ID:          "step1",
				Type:        biz.NodeTypeAgent,
				Description: taskDesc,
				AgentName:   agentName,
				FuncRef:     agentInvokeFuncRef,
			},
		},
		EntryPoint:      "step1",
		FinishPoint:     "step1",
		ExecutionEngine: biz.EngineBSP,
	}
}

// matchAgent selects the best agent for a subtask. Tries exact role match
// first, then domain match, then falls back to the first available agent.
func matchAgent(st subtaskDef, agents []biz.AgentCapability) string {
	if len(agents) == 0 {
		return ""
	}

	if st.RequiredRole != "" {
		for _, a := range agents {
			for _, role := range a.Roles {
				if strings.EqualFold(role, st.RequiredRole) {
					return a.AgentKey
				}
			}
		}
	}

	if st.RequiredDomain != "" {
		for _, a := range agents {
			for _, domain := range a.Domains {
				if strings.EqualFold(domain, st.RequiredDomain) {
					return a.AgentKey
				}
			}
		}
	}

	return agents[0].AgentKey
}

// validateDAG checks that the graph has no cycles and has a valid entry point.
// Returns true if the graph is a valid DAG (or has no edges), false otherwise.
func validateDAG(cfg *biz.GraphBuildConfig) bool {
	if cfg == nil || len(cfg.Nodes) == 0 {
		return false
	}

	nodeSet := make(map[string]bool, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		nodeSet[n.ID] = true
	}

	if cfg.EntryPoint == "" || !nodeSet[cfg.EntryPoint] {
		return false
	}

	// Build adjacency list from edges (only between known nodes).
	adj := make(map[string][]string, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		adj[n.ID] = nil
	}
	for _, e := range cfg.Edges {
		if nodeSet[e.From] && nodeSet[e.To] {
			adj[e.From] = append(adj[e.From], e.To)
		}
	}

	// DFS cycle detection: white=unvisited, gray=in-progress, black=done.
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(cfg.Nodes))

	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		color[node] = gray
		for _, next := range adj[node] {
			if color[next] == gray {
				return true
			}
			if color[next] == white && hasCycle(next) {
				return true
			}
		}
		color[node] = black
		return false
	}

	for _, n := range cfg.Nodes {
		if color[n.ID] == white {
			if hasCycle(n.ID) {
				return false
			}
		}
	}

	return true
}

// consumeModelResponse drains the response channel and concatenates content.
func consumeModelResponse(respChan <-chan *trpcmodel.Response) (string, error) {
	var sb strings.Builder
	for resp := range respChan {
		if resp == nil {
			continue
		}
		if resp.Error != nil {
			return "", apierror.Internal(apierror.DomainGraph, "LLM response error").WithCause(resp.Error)
		}
		for _, choice := range resp.Choices {
			if choice.Delta.Content != "" {
				sb.WriteString(choice.Delta.Content)
			}
			if choice.Message.Content != "" {
				sb.WriteString(choice.Message.Content)
			}
		}
	}
	return sb.String(), nil
}

// stripFences removes optional markdown code fences from model output.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if m := fenceRegexp.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return s
}

// buildNL2GraphPrompt builds the user prompt for LLM task analysis.
func buildNL2GraphPrompt(taskDesc string, availableAgents []biz.AgentCapability) string {
	var sb strings.Builder
	sb.WriteString("Analyze the following task and break it into subtasks:\n\n")
	sb.WriteString("Task: " + taskDesc + "\n\n")
	if len(availableAgents) > 0 {
		sb.WriteString("Available agents:\n")
		for _, a := range availableAgents {
			sb.WriteString("  - AgentKey: " + a.AgentKey)
			if len(a.Roles) > 0 {
				sb.WriteString(", Roles: " + strings.Join(a.Roles, ", "))
			}
			if len(a.Domains) > 0 {
				sb.WriteString(", Domains: " + strings.Join(a.Domains, ", "))
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\nOutput ONLY a JSON object with the schema specified in the system prompt.\n")
	return sb.String()
}

// nl2graphSystemPrompt returns the system prompt for LLM task analysis.
func nl2graphSystemPrompt() string {
	return `You are a graph orchestration designer. Analyze the task and break it into subtasks.
Return JSON with this schema:
{
  "subtasks": [
    {"id": "step1", "description": "...", "depends_on": [], "required_role": "researcher", "required_domain": "search"}
  ],
  "template": "sequential|parallel|coordinator|dag",
  "entry_point": "step1",
  "finish_point": "stepN"
}

Rules:
- Output ONLY a JSON object, no markdown fences, no explanation
- subtasks: list of subtask definitions with unique IDs
- depends_on: list of subtask IDs that must complete before this subtask
- template: "sequential" for linear deps, "parallel" for no deps, "coordinator" for hub-spoke, "dag" for complex deps
- entry_point: ID of the first subtask (no dependencies)
- finish_point: ID of the last subtask (nothing depends on it)
- required_role/required_domain: used to match an agent for the subtask`
}

// taskAnalysis is the LLM-parsed task analysis result.
type taskAnalysis struct {
	Subtasks    []subtaskDef `json:"subtasks"`
	Template    string       `json:"template"`
	EntryPoint  string       `json:"entry_point"`
	FinishPoint string       `json:"finish_point"`
}

// subtaskDef describes a single subtask identified by the LLM.
type subtaskDef struct {
	ID             string   `json:"id"`
	Description    string   `json:"description"`
	DependsOn      []string `json:"depends_on"`
	RequiredRole   string   `json:"required_role"`
	RequiredDomain string   `json:"required_domain"`
}
