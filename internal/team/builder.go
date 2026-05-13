package team

import "strings"

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
