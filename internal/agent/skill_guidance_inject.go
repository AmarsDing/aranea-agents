package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/skill/manifest"
	"aranea-agents/internal/skill/render"
	"aranea-agents/internal/tools/skillruntime"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const maxSkillGuidanceChars = 4000

// skillSelectionReasonStateKey is the invocation state key for skill selection reasons.
const skillSelectionReasonStateKey = "aranea.skill_selection_reasons"

// skillRoutedSlugsStateKey is the invocation state key for routed skill slugs.
const skillRoutedSlugsStateKey = "aranea.skill_routed_slugs"

// skillLoadedSlugStateKey is the invocation state key for the slug loaded by
// skill_load or skill_run. Set by the after-tool hook so the invocation
// recorder can persist it.
const skillLoadedSlugStateKey = "aranea.skill_loaded_slug"

// skillTokenUsageStateKey is the invocation state key for accumulated token usage.
const skillTokenUsageStateKey = "aranea.skill_token_usage"

// tokenUsageSnapshot stores accumulated prompt/completion/total token counts.
type tokenUsageSnapshot struct {
	PromptTokens     int `json:"prompt"`
	CompletionTokens int `json:"completion"`
	TotalTokens      int `json:"total"`
}

func newSkillGuidanceBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if ag.Settings == nil || deps.SkillUC == nil {
		return nil
	}
	if !SkillsUseFullProfile(ag.SystemPromptMode) {
		return nil
	}
	if biz.IsProgressiveSkillLoad(ag.Settings.GetSkillLoadMode()) {
		return newProgressiveSkillGuidanceHook(ag, deps)
	}
	return callbacks.NewBeforeModelHook(5, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		result := resolveAndWriteSkillState(ctx, ag.Settings, deps, false)
		if result == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		entries, err := deps.SkillUC.BatchGetSkillGuidance(ctx, result.Slugs)
		if err != nil || len(entries) == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		var b strings.Builder
		b.WriteString("## Available Skills\n\nThe following skills are routed for this turn. Use the skill_run tool to invoke them, or skill_load to load additional skills.\n\n")
		totalChars := 0
		written := 0
		for _, e := range entries {
			m := manifest.Parse(e.Guidance)
			guidance := render.SkillGuidance(m, render.RenderOptions{Mode: render.ModeAIOptimized})
			entry := fmt.Sprintf("### %s\n%s\n\n", e.Slug, guidance)
			if totalChars+len(entry) > maxSkillGuidanceChars {
				remaining := len(entries) - written
				if remaining > 0 {
					b.WriteString(fmt.Sprintf("... and %d more skills (truncated)\n", remaining))
				}
				break
			}
			b.WriteString(entry)
			totalChars += len(entry)
			written++
		}
		if written == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if written < len(entries) {
			b.WriteString("\n> Other available skills can be loaded on demand using the skill_load tool.\n")
		}
		cue := truncateAtMarkdownBoundary(b.String(), maxSkillGuidanceChars)
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func newProgressiveSkillGuidanceHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	return callbacks.NewBeforeModelHook(5, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// resolveAndWriteSkillState writes routed slugs and selection
		// reasons to invocation state. No system message is injected;
		// injectOverview handles all Skill display with [routed] markers.
		resolveAndWriteSkillState(ctx, ag.Settings, deps, true)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// resolveAndWriteSkillState resolves routed skill slugs and writes them to
// invocation state. When progressive is true, routed slugs are stored under
// RoutedSkillsStateKey so the SkillsRequestProcessor can mark them as [routed].
// Returns nil when no skills are resolved or on error.
func resolveAndWriteSkillState(ctx context.Context, runtime *biz.AgentRuntimeSettings, deps TRPCBuilderDeps, progressive bool) *skillruntime.ResolveResult {
	opts := &skillruntime.SkillToolsetOptions{Runtime: runtime, UserQuery: skillruntime.TurnQueryFromContext(ctx)}
	result, err := skillruntime.ResolveSkillSlugsDetailed(ctx, deps.SkillUC, opts, deps.Logger())
	if err != nil || len(result.Slugs) == 0 {
		return nil
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok {
		return result
	}
	if progressive {
		// RoutedSkillsStateKey is read by trpc-agent-go's SkillsRequestProcessor
		// to mark skills as [routed] in the overview. skillRoutedSlugsStateKey is
		// read by the invocation recorder to persist routed_slugs for health metrics.
		// Both store the same data but serve different consumers.
		inv.SetState(trpcllmagent.RoutedSkillsStateKey, result.Slugs)
	}
	inv.SetState(skillRoutedSlugsStateKey, result.Slugs)
	inv.SetState(skillSelectionReasonStateKey, result.Reasons)
	return result
}

// newTokenUsageAccumulatorAfterHook returns an AfterModel hook that accumulates
// token usage from each model call into invocation state. The skill invocation
// recorder reads this state to populate the token_usage field.
func newTokenUsageAccumulatorAfterHook() callbacks.Callback {
	return callbacks.NewAfterModelHook(0, func(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
		if args == nil || args.Response == nil || args.Response.Usage == nil {
			return &trpcmodel.AfterModelResult{Context: ctx}, nil
		}
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil {
			return &trpcmodel.AfterModelResult{Context: ctx}, nil
		}
		u := args.Response.Usage
		prev, _ := inv.GetState(skillTokenUsageStateKey)
		snap := tokenUsageSnapshot{}
		if p, ok := prev.(tokenUsageSnapshot); ok {
			snap = p
		}
		snap.PromptTokens += u.PromptTokens
		snap.CompletionTokens += u.CompletionTokens
		snap.TotalTokens += u.TotalTokens
		inv.SetState(skillTokenUsageStateKey, snap)
		return &trpcmodel.AfterModelResult{Context: ctx}, nil
	})
}

// newSkillLoadCaptureAfterHook returns an AfterTool hook that captures the slug
// from skill_load/skill_run tool calls and writes it to invocation state.
// The invocation recorder reads this state to populate the loaded_slug field.
func newSkillLoadCaptureAfterHook() callbacks.AfterToolHook {
	return callbacks.NewAfterToolHook(0, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil {
			return &trpctool.AfterToolResult{}, nil
		}
		toolName := strings.ToLower(strings.TrimSpace(args.ToolName))
		if toolName != "skill_load" && toolName != "skill_run" {
			return &trpctool.AfterToolResult{}, nil
		}
		// Extract slug from tool arguments (JSON: {"slug": "xxx"} or {"skill_slug": "xxx"}).
		slug := extractSlugFromArgs(args.Arguments)
		if slug == "" {
			return &trpctool.AfterToolResult{}, nil
		}
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil {
			return &trpctool.AfterToolResult{}, nil
		}
		inv.SetState(skillLoadedSlugStateKey, slug)
		return &trpctool.AfterToolResult{}, nil
	})
}

// extractSlugFromArgs parses tool arguments to extract the skill slug.
func extractSlugFromArgs(args []byte) string {
	if len(args) == 0 {
		return ""
	}
	// Simple JSON key extraction without full unmarshal.
	s := string(args)
	// Try "slug" key first, then "skill_slug".
	for _, key := range []string{`"slug"`, `"skill_slug"`} {
		idx := strings.Index(s, key)
		if idx < 0 {
			continue
		}
		// Find the value after the key.
		rest := s[idx+len(key):]
		// Skip whitespace and colon.
		rest = strings.TrimLeft(rest, " \t\n\r:")
		// Skip opening quote.
		rest = strings.TrimLeft(rest, " \t\n\r")
		if len(rest) == 0 || rest[0] != '"' {
			continue
		}
		rest = rest[1:]
		end := strings.IndexByte(rest, '"')
		if end < 0 {
			continue
		}
		return rest[:end]
	}
	return ""
}

// truncateAtMarkdownBoundary truncates content to at most limit bytes, preferring
// to cut at a Markdown boundary (heading, horizontal rule, or blank line) rather
// than in the middle of a paragraph.
func truncateAtMarkdownBoundary(content string, limit int) string {
	if len(content) <= limit {
		return content
	}
	boundaries := []string{"\n### ", "\n---", "\n\n"}
	bestPos := -1
	for _, sep := range boundaries {
		searchStart := limit - limit/4
		if searchStart < 0 {
			searchStart = 0
		}
		idx := strings.LastIndex(content[searchStart:limit], sep)
		if idx >= 0 {
			absIdx := searchStart + idx
			if absIdx > bestPos {
				bestPos = absIdx
			}
		}
	}
	if bestPos > 0 {
		return content[:bestPos]
	}
	return content[:limit]
}
