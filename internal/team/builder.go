package team

import (
	"context"
	"strings"

	bizagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

func firstOfThree(a, b, c string) string {
	for _, v := range []string{a, b, c} {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func buildTRPCLLMAgentForMember(ctx context.Context, ag biz.Agent, deps bizagent.TRPCBuilderDeps, provOpt, modOpt string, sess biz.Session, teamMode string, member MemberDef) (trpcagent.Agent, error) {
	d := deps
	d.Provider = firstOfThree(provOpt, sess.Provider, ag.Provider)
	d.Model = firstOfThree(modOpt, sess.Model, ag.Model)
	d.DialogMode = strings.TrimSpace(teamMode)
	return bizagent.BuildTRPCLLMAgent(ctx, ag, d)
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
	default:
		if d.LoopMaxIterations > 0 {
			return boundedLoopIterations(d.LoopMaxIterations, maxCap)
		}
		return 3
	}
}

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
