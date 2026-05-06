package team

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/adkadapter"
	"aranea-agents/internal/biz"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/workflowagents/loopagent"
	"google.golang.org/adk/agent/workflowagents/parallelagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
)

// workflowPlan describes the built workflow for stream filtering and step persistence.
type workflowPlan struct {
	// persistMembers is members whose final assistant outputs are recorded as team_run_steps (ordered).
	persistMembers []MemberDef
	// streamAuthor is the SubAgents leaf agent name (AgentKey) whose SSE deltas/done reflect the user-visible turn.
	streamAuthor string
}

func firstOfThree(a, b, c string) string {
	for _, v := range []string{a, b, c} {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func buildLLMChain(
	ctx context.Context,
	members []MemberDef,
	deps adkadapter.BuilderDeps,
	provOpt, modOpt string,
	sess biz.Session,
	resolve func(context.Context, string) (biz.Agent, error),
) ([]agent.Agent, error) {
	var out []agent.Agent
	for _, m := range members {
		ag, err := resolve(ctx, m.AgentID)
		if err != nil {
			return nil, err
		}
		d := deps
		d.Provider = firstOfThree(provOpt, sess.Provider, ag.Provider)
		d.Model = firstOfThree(modOpt, sess.Model, ag.Model)
		a, err := adkadapter.BuildLLMAgent(ctx, ag, d)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func loopMaxIterations(d Definition) uint {
	if d.TimeoutSeconds > 0 && d.TimeoutSeconds <= 64 {
		return uint(d.TimeoutSeconds)
	}
	return 8
}

// BuildWorkflowRoot builds a workflow agent tree for native team execution via ADK runner.Run.
func BuildWorkflowRoot(
	ctx context.Context,
	mode string,
	def Definition,
	deps adkadapter.BuilderDeps,
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
		subs, err := buildLLMChain(ctx, members, deps, provOpt, modOpt, sess, resolve)
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
		plan.streamAuthor = subs[len(subs)-1].Name()
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
			d := deps
			d.Provider = firstOfThree(provOpt, sess.Provider, ag.Provider)
			d.Model = firstOfThree(modOpt, sess.Model, ag.Model)
			sub, err := adkadapter.BuildLLMAgent(ctx, ag, d)
			if err != nil {
				return nil, plan, err
			}
			plan.persistMembers = workers
			plan.streamAuthor = sub.Name()
			return sub, plan, nil
		}
		if len(workers) > 1 && synthID == "" {
			return nil, plan, fmt.Errorf("team: parallel team requires synthesizer_agent_id or a synthesizer member")
		}
		wAgents, err := buildLLMChain(ctx, workers, deps, provOpt, modOpt, sess, resolve)
		if err != nil {
			return nil, plan, err
		}
		par, err := parallelagent.New(parallelagent.Config{
			AgentConfig: agent.Config{
				Name:        "team_parallel_workers",
				Description: "Parallel worker agents",
				SubAgents:   wAgents,
			},
		})
		if err != nil {
			return nil, plan, err
		}
		synthAg, err := resolve(ctx, synthID)
		if err != nil {
			return nil, plan, err
		}
		d := deps
		d.Provider = firstOfThree(provOpt, sess.Provider, synthAg.Provider)
		d.Model = firstOfThree(modOpt, sess.Model, synthAg.Model)
		synthLLM, err := adkadapter.BuildLLMAgent(ctx, synthAg, d)
		if err != nil {
			return nil, plan, err
		}
		rootAgent, err := sequentialagent.New(sequentialagent.Config{
			AgentConfig: agent.Config{
				Name:        "team_parallel_root",
				Description: "Parallel workers then synthesizer",
				SubAgents:   []agent.Agent{par, synthLLM},
			},
		})
		if err != nil {
			return nil, plan, err
		}
		plan.persistMembers = append(append([]MemberDef{}, workers...), MemberDef{AgentID: synthAg.ID, Role: "synthesizer"})
		plan.streamAuthor = synthLLM.Name()
		return rootAgent, plan, nil

	case "coordinator", "critic_loop", "adaptive":
		members := EnabledMembers(def)
		if len(members) == 0 {
			return nil, plan, fmt.Errorf("team: no members")
		}
		subs, err := buildLLMChain(ctx, members, deps, provOpt, modOpt, sess, resolve)
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
			MaxIterations: loopMaxIterations(def),
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
		plan.streamAuthor = subs[len(subs)-1].Name()
		return rootAgent, plan, nil

	default:
		return nil, plan, fmt.Errorf("team: unsupported mode %q", mode)
	}
}
