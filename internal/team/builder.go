package team

import (
	"context"
	"fmt"
	"strings"

	bizagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/workflowagents/loopagent"
	"google.golang.org/adk/agent/workflowagents/parallelagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
)

// workflowPlan describes the built workflow for stream filtering and step persistence.
type workflowPlan struct {
	// persistMembers is members whose final assistant outputs are recorded as team_run_steps (ordered).
	persistMembers []MemberDef
	// persistRuntimeNames holds ADK agent.Name() for each persistMembers entry (same order), for author keymap.
	persistRuntimeNames []string
	// streamAuthor is the SubAgents leaf agent name (AgentKey) whose SSE deltas/done reflect the user-visible turn.
	streamAuthor string
	// workflowAuthorAliases are workflow wrapper agent names (sequential / loop / parallel parents)
	// that may appear as ev.Author on final responses; they map to streamAuthor's persist entry in the runner.
	workflowAuthorAliases []string
}

func firstOfThree(a, b, c string) string {
	for _, v := range []string{a, b, c} {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func buildLLMAgentForMember(ctx context.Context, ag biz.Agent, deps bizagent.BuilderDeps, mount tools.TurnMount, skillUserQuery string, provOpt, modOpt string, sess biz.Session, teamMode string, member MemberDef) (agent.Agent, error) {
	d := deps
	d.Provider = firstOfThree(provOpt, sess.Provider, ag.Provider)
	d.Model = firstOfThree(modOpt, sess.Model, ag.Model)
	d.TeamOrchestrationMode = strings.TrimSpace(teamMode)
	d.TeamMemberRole = strings.TrimSpace(member.Role)
	d.TeamMemberDisplayName = strings.TrimSpace(member.Name)
	if err := mount.Attach(ctx, ag, skillUserQuery, &d.Tools, &d.Toolsets); err != nil {
		return nil, err
	}
	return bizagent.BuildLLMAgent(ctx, ag, d)
}

func agentNamesFromSubs(subs []agent.Agent) []string {
	if len(subs) == 0 {
		return nil
	}
	out := make([]string, len(subs))
	for i, s := range subs {
		out[i] = s.Name()
	}
	return out
}

func buildLLMChain(
	ctx context.Context,
	members []MemberDef,
	deps bizagent.BuilderDeps,
	mount tools.TurnMount,
	skillUserQuery string,
	provOpt, modOpt string,
	sess biz.Session,
	teamMode string,
	resolve func(context.Context, string) (biz.Agent, error),
) ([]agent.Agent, error) {
	var out []agent.Agent
	for _, m := range members {
		ag, err := resolve(ctx, m.AgentID)
		if err != nil {
			return nil, err
		}
		a, err := buildLLMAgentForMember(ctx, ag, deps, mount, skillUserQuery, provOpt, modOpt, sess, teamMode, m)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func boundedLoopIterations(v int, maxCap uint) uint {
	if v <= 0 {
		return 0
	}
	u := uint(v)
	if u > maxCap {
		return maxCap
	}
	return u
}

// chunkParallelWorkers splits workers into batches of at most maxConcurrency (all in one batch if <=0).
func chunkParallelWorkers(workers []MemberDef, maxConcurrency int) [][]MemberDef {
	if len(workers) == 0 {
		return nil
	}
	if maxConcurrency <= 0 || maxConcurrency >= len(workers) {
		return [][]MemberDef{workers}
	}
	var out [][]MemberDef
	for i := 0; i < len(workers); i += maxConcurrency {
		j := i + maxConcurrency
		if j > len(workers) {
			j = len(workers)
		}
		out = append(out, workers[i:j])
	}
	return out
}

// loopMaxIterations bounds LoopAgent replays.
// critic_loop: critic_loop.max_iterations → loop_max_iterations → default 8.
// coordinator/adaptive: loop_max_iterations → default 3.
func loopMaxIterations(mode string, d Definition) uint {
	const maxCap = uint(32)
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "critic_loop":
		if d.CriticLoop != nil && d.CriticLoop.MaxIterations > 0 {
			return boundedLoopIterations(d.CriticLoop.MaxIterations, maxCap)
		}
		if d.LoopMaxIterations > 0 {
			return boundedLoopIterations(d.LoopMaxIterations, maxCap)
		}
		return 8
	case "coordinator", "adaptive":
		if d.LoopMaxIterations > 0 {
			return boundedLoopIterations(d.LoopMaxIterations, maxCap)
		}
		// Default 3 outer passes unless overridden (each increment runs the full member chain again).
		return 3
	default:
		if d.LoopMaxIterations > 0 {
			return boundedLoopIterations(d.LoopMaxIterations, maxCap)
		}
		return 8
	}
}

// BuildWorkflowRoot builds a workflow agent tree for native team execution via ADK runner.Run.
// skillUserQuery narrows skill toolsets per member (each member still uses its own SkillRuntimeJSON).
func BuildWorkflowRoot(
	ctx context.Context,
	mode string,
	def Definition,
	deps bizagent.BuilderDeps,
	mount tools.TurnMount,
	skillUserQuery string,
	sess biz.Session,
	provOpt, modOpt string,
	resolve func(context.Context, string) (biz.Agent, error),
) (root agent.Agent, plan workflowPlan, err error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "sequential":
		members := EnabledMembers(def)
		if len(members) == 0 {
			return nil, plan, fmt.Errorf("team: no members")
		}
		subs, err := buildLLMChain(ctx, members, deps, mount, skillUserQuery, provOpt, modOpt, sess, mode, resolve)
		if err != nil {
			return nil, plan, err
		}
		rootAgent, err := sequentialagent.New(sequentialagent.Config{
			AgentConfig: agent.Config{
				Name:        "team_sequential",
				Description: "Sequential team workflow",
				SubAgents:   subs,
			},
		})
		if err != nil {
			return nil, plan, err
		}
		plan.persistMembers = members
		plan.persistRuntimeNames = agentNamesFromSubs(subs)
		plan.streamAuthor = subs[len(subs)-1].Name()
		plan.workflowAuthorAliases = []string{"team_sequential"}
		return rootAgent, plan, nil

	case "parallel":
		workers := ParallelWorkers(def)
		if len(workers) == 0 {
			return nil, plan, fmt.Errorf("team: no workers")
		}
		synthID := strings.TrimSpace(SynthesizerAgentID(def))
		if len(workers) == 1 && synthID == "" {
			ag, err := resolve(ctx, workers[0].AgentID)
			if err != nil {
				return nil, plan, err
			}
			sub, err := buildLLMAgentForMember(ctx, ag, deps, mount, skillUserQuery, provOpt, modOpt, sess, mode, workers[0])
			if err != nil {
				return nil, plan, err
			}
			plan.persistMembers = workers
			plan.persistRuntimeNames = []string{sub.Name()}
			plan.streamAuthor = sub.Name()
			plan.workflowAuthorAliases = nil
			return sub, plan, nil
		}
		if len(workers) > 1 && synthID == "" {
			return nil, plan, fmt.Errorf("team: parallel team requires synthesizer_agent_id or a synthesizer member")
		}
		chunks := chunkParallelWorkers(workers, def.MaxConcurrency)
		var workerStages []agent.Agent
		var workerRuntimeNames []string
		anyParallelBatch := false
		for _, chunk := range chunks {
			subs, err := buildLLMChain(ctx, chunk, deps, mount, skillUserQuery, provOpt, modOpt, sess, mode, resolve)
			if err != nil {
				return nil, plan, err
			}
			if len(subs) == 1 {
				workerStages = append(workerStages, subs[0])
				workerRuntimeNames = append(workerRuntimeNames, subs[0].Name())
				continue
			}
			anyParallelBatch = true
			par, err := parallelagent.New(parallelagent.Config{
				AgentConfig: agent.Config{
					Name:        "team_parallel_workers",
					Description: "Parallel worker batch",
					SubAgents:   subs,
				},
			})
			if err != nil {
				return nil, plan, err
			}
			workerStages = append(workerStages, par)
			for _, s := range subs {
				workerRuntimeNames = append(workerRuntimeNames, s.Name())
			}
		}
		var workersPhase agent.Agent
		switch len(workerStages) {
		case 0:
			return nil, plan, fmt.Errorf("team: parallel workers phase empty")
		case 1:
			workersPhase = workerStages[0]
		default:
			wp, err := sequentialagent.New(sequentialagent.Config{
				AgentConfig: agent.Config{
					Name:        "team_parallel_stages",
					Description: "Sequential batches of parallel workers (max_concurrency)",
					SubAgents:   workerStages,
				},
			})
			if err != nil {
				return nil, plan, err
			}
			workersPhase = wp
		}
		synthAg, err := resolve(ctx, synthID)
		if err != nil {
			return nil, plan, err
		}
		synthMember := MemberDef{
			AgentID: synthAg.ID,
			Role:    "synthesizer",
			Name:    firstOfThree(synthAg.DisplayName, synthAg.AgentKey, ""),
		}
		synthLLM, err := buildLLMAgentForMember(ctx, synthAg, deps, mount, skillUserQuery, provOpt, modOpt, sess, mode, synthMember)
		if err != nil {
			return nil, plan, err
		}
		rootAgent, err := sequentialagent.New(sequentialagent.Config{
			AgentConfig: agent.Config{
				Name:        "team_parallel_root",
				Description: "Parallel workers then synthesizer",
				SubAgents:   []agent.Agent{workersPhase, synthLLM},
			},
		})
		if err != nil {
			return nil, plan, err
		}
		plan.persistMembers = append(append([]MemberDef{}, workers...), MemberDef{AgentID: synthAg.ID, Role: "synthesizer"})
		plan.persistRuntimeNames = append(workerRuntimeNames, synthLLM.Name())
		plan.streamAuthor = synthLLM.Name()
		aliases := []string{"team_parallel_root"}
		if len(workerStages) > 1 {
			aliases = append(aliases, "team_parallel_stages")
		}
		if anyParallelBatch {
			aliases = append(aliases, "team_parallel_workers")
		}
		plan.workflowAuthorAliases = aliases
		return rootAgent, plan, nil

	case "coordinator", "critic_loop", "adaptive":
		members := EnabledMembers(def)
		if len(members) == 0 {
			return nil, plan, fmt.Errorf("team: no members")
		}
		subs, err := buildLLMChain(ctx, members, deps, mount, skillUserQuery, provOpt, modOpt, sess, mode, resolve)
		if err != nil {
			return nil, plan, err
		}
		seqInner, err := sequentialagent.New(sequentialagent.Config{
			AgentConfig: agent.Config{
				Name:        "team_loop_body",
				Description: "Ordered members",
				SubAgents:   subs,
			},
		})
		if err != nil {
			return nil, plan, err
		}
		rootAgent, err := loopagent.New(loopagent.Config{
			MaxIterations: loopMaxIterations(mode, def),
			AgentConfig: agent.Config{
				Name:        "team_loop_" + mode,
				Description: "Loop workflow over team members",
				SubAgents:   []agent.Agent{seqInner},
			},
		})
		if err != nil {
			return nil, plan, err
		}
		plan.persistMembers = members
		plan.persistRuntimeNames = agentNamesFromSubs(subs)
		plan.streamAuthor = subs[len(subs)-1].Name()
		plan.workflowAuthorAliases = []string{"team_loop_" + mode, "team_loop_body"}
		return rootAgent, plan, nil

	default:
		return nil, plan, fmt.Errorf("team: unsupported mode %q", mode)
	}
}
