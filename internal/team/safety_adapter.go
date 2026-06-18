package team

// TECH-DEBT(P2-19/20): SwarmSafetyOptions/SessionIsolationOptions/MemberToolOptions
// 适配器已实现但未接入生产路径。项目 Team 编排当前使用自建安全策略与
// 会话隔离机制，未切换到框架 team.Option 方案。适配器保留作为未来
// 切换到框架 Team 安全机制的桥接点（alignment-plan.md §四 协同包 C）。

import (
	"time"

	"trpc.group/trpc-go/trpc-agent-go/team"
)

// SwarmSafetyOptions converts the project's SwarmConfigDef into framework
// team.Option functions that enable the framework's Swarm safety mechanisms.
//
// TECH-DEBT(P2-19): 未接入生产路径，见文件头说明。
//
// P2-19: This adapter enables the framework's built-in Swarm safety:
//   - MaxHandoffs: limits total transfers in a single run
//   - NodeTimeout: limits how long a single member may run after transfer
//   - RepetitiveHandoffWindow + RepetitiveHandoffMinUnique: detects loops
//
// When the SwarmConfigDef is nil (non-swarm team), no options are returned.
func SwarmSafetyOptions(swarm *SwarmConfigDef) []team.Option {
	if swarm == nil {
		return nil
	}
	cfg := team.SwarmConfig{
		MaxHandoffs:                swarm.MaxHandoffs,
		RepetitiveHandoffWindow:    swarm.RepetitiveHandoffWindow,
		RepetitiveHandoffMinUnique: swarm.RepetitiveHandoffMinUnique,
	}
	if swarm.NodeTimeoutSeconds > 0 {
		cfg.NodeTimeout = time.Duration(swarm.NodeTimeoutSeconds) * time.Second
	}
	return []team.Option{team.WithSwarmConfig(cfg)}
}

// SessionIsolationOptions converts the project's SwarmConfigDef into framework
// team.Option functions that enable session isolation for Team members.
//
// TECH-DEBT(P2-20): 未接入生产路径，见文件头说明。
//
// P2-20: This adapter enables the framework's WithSwarmIndependentAgents,
// which gives each Swarm member a private session (history isolation).
// When CrossRequestTransfer is also enabled, the last transfer target
// receives future user turns instead of the entry member.
//
// When the SwarmConfigDef is nil (non-swarm team), no options are returned.
func SessionIsolationOptions(swarm *SwarmConfigDef) []team.Option {
	if swarm == nil {
		return nil
	}
	var opts []team.Option
	// Enable per-agent session isolation for swarm members.
	opts = append(opts, team.WithSwarmIndependentAgents())
	// Enable cross-request transfer if configured.
	if swarm.CrossRequestTransfer {
		opts = append(opts, team.WithCrossRequestTransfer(true))
	}
	return opts
}

// MemberToolOptions converts the project's MemberToolDef into framework
// team.Option functions for coordinator-mode member tool configuration.
//
// TECH-DEBT(P2-19/20): 未接入生产路径，见文件头说明。
func MemberToolOptions(mt *MemberToolDef) []team.Option {
	if mt == nil {
		return nil
	}
	cfg := team.MemberToolConfig{
		StreamInner:       mt.StreamInner,
		SkipSummarization: mt.SkipSummarization,
	}
	switch mt.InnerTextMode {
	case "include":
		cfg.InnerTextMode = team.InnerTextModeInclude
	case "exclude":
		cfg.InnerTextMode = team.InnerTextModeExclude
	default:
		cfg.InnerTextMode = team.InnerTextModeDefault
	}
	switch mt.HistoryScope {
	case "isolated":
		cfg.HistoryScope = team.HistoryScopeIsolated
	case "parent_branch":
		cfg.HistoryScope = team.HistoryScopeParentBranch
	default:
		cfg.HistoryScope = team.HistoryScopeDefault
	}
	opts := []team.Option{team.WithMemberToolConfig(cfg)}
	if mt.ToolSetName != "" {
		opts = append(opts, team.WithMemberToolSetName(mt.ToolSetName))
	}
	return opts
}
