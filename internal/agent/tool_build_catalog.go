package agent

import (
	"context"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/loggateway"
)

// toolBuildPlan bundles the per-build tool inputs loaded once by
// BuildTRPCLLMAgent and shared between tool assembly and the callback chain.
type toolBuildPlan struct {
	eff     map[string]bool
	catalog *toolBuildCatalog
	gate    *toolConfirmGate
}

// toolBuildCatalog is the per-build snapshot of tool catalog rows and agent
// overrides, batch-loaded once per agent build. It replaces the previous
// 3×N+1 pattern where applyRuntimeToolConfigs and buildCatalogConfirmTools
// (built twice: toolset confirmation policy + callback-chain hook) each ran
// one aggregation-heavy GetTool query per enabled tool (~210 queries, ~10s
// per cold build with 70 tools at ~49ms each).
type toolBuildCatalog struct {
	entries   map[string]biztool.ToolCatalogEntry
	overrides map[string]biz.ToolAgentOverride
	// overridesFailed marks the overrides query failure: the confirmation
	// gate must fail closed (every enabled tool requires confirmation),
	// matching the pre-snapshot behavior.
	overridesFailed bool
}

// loadToolBuildCatalog batch-loads the snapshot with exactly two queries
// (one IN batch + one overrides list). Returns nil when there is nothing to
// load (no ToolUC, empty agentID, or no enabled tools).
func loadToolBuildCatalog(ctx context.Context, agentID string, eff map[string]bool, deps TRPCBuilderDeps) *toolBuildCatalog {
	if deps.ToolUC == nil || strings.TrimSpace(agentID) == "" || len(eff) == 0 {
		return nil
	}
	keys := make([]string, 0, len(eff))
	for key, enabled := range eff {
		if enabled {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys) // deterministic SQL + logs
	lg := deps.Logger()

	c := &toolBuildCatalog{}
	entries, err := deps.ToolUC.ListToolCatalogEntries(ctx, keys)
	if err != nil {
		// Fail-soft for runtime configs (all skipped) and fail-closed for the
		// confirmation gate (missing entry ⇒ requires confirmation) — the
		// same degradation the per-key GetTool errors produced.
		lg.Warn("tool build catalog: batch load failed, configs skipped and confirm gate fails closed",
			loggateway.StepID("agent.tool_build"),
			loggateway.Str("agent_id", agentID),
			loggateway.Err(err))
		entries = nil
	}
	c.entries = make(map[string]biztool.ToolCatalogEntry, len(entries))
	for _, e := range entries {
		k := strings.TrimSpace(e.Key)
		if k == "" {
			continue
		}
		if _, dup := c.entries[k]; !dup {
			c.entries[k] = e // first row wins, matching GetTool's LIMIT 1
		}
	}

	overrides, oerr := deps.ToolUC.ListToolAgentOverridesByAgent(ctx, agentID)
	if oerr != nil {
		c.overridesFailed = true
		lg.Warn("tool build catalog: overrides load failed, confirm gate fails closed",
			loggateway.StepID("agent.tool_build"),
			loggateway.Str("agent_id", agentID),
			loggateway.Err(oerr))
		overrides = nil
	}
	c.overrides = make(map[string]biz.ToolAgentOverride, len(overrides))
	for _, o := range overrides {
		c.overrides[strings.TrimSpace(o.ToolKey)] = o
	}
	return c
}

// mergedConfigMaps computes per-tool runtime config maps: base = config_json
// (falling back to default_config_json), overlaid with the agent's config
// override. Tools missing from the snapshot are skipped (fail-soft), matching
// the previous per-key GetTool error handling.
func (c *toolBuildCatalog) mergedConfigMaps(eff map[string]bool) map[string]map[string]any {
	if c == nil {
		return nil
	}
	merged := make(map[string]map[string]any)
	for key, enabled := range eff {
		if !enabled {
			continue
		}
		e, ok := c.entries[key]
		if !ok {
			continue
		}
		base := strings.TrimSpace(e.ConfigJSON)
		if base == "" {
			base = strings.TrimSpace(e.DefaultConfigJSON)
		}
		merged[key] = biz.MergeToolConfigJSON(base, c.overrides[key].ConfigOverrideJSON)
	}
	return merged
}

// confirmCatalog builds the confirmation-gate catalog from the snapshot.
// Fail-closed rules preserved from the pre-snapshot implementation:
//   - overrides query failed  ⇒ every enabled tool requires confirmation
//   - tool row missing (deleted mid-build / load failure) ⇒ requires confirmation
func (c *toolBuildCatalog) confirmCatalog(eff map[string]bool) map[string]confirmCatalogEntry {
	if c == nil {
		return nil
	}
	out := make(map[string]confirmCatalogEntry)
	for key, enabled := range eff {
		if !enabled {
			continue
		}
		if c.overridesFailed {
			out[key] = confirmCatalogEntry{requiresConfirm: true}
			continue
		}
		e, ok := c.entries[key]
		if !ok {
			out[key] = confirmCatalogEntry{requiresConfirm: true}
			continue
		}
		ov, hasOV := c.overrides[key]
		// ToolRequiresConfirmation only reads Tool.RequiresConfirmation; the
		// partial Tool mirrors the catalog entry without a second struct copy.
		if biz.ToolRequiresConfirmation(biz.Tool{RequiresConfirmation: e.RequiresConfirmation}, ov, hasOV) {
			out[key] = confirmCatalogEntry{requiresConfirm: true}
		}
	}
	return out
}
