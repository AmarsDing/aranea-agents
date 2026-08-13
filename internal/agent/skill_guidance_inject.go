package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/skill/manifest"
	"aranea-agents/internal/skill/render"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
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
	// Progressive mode takes precedence over Full Profile: it works in both
	// "task" and "complete" prompt modes. Without this, "task" + progressive
	// would never write routed slugs, breaking health metrics observability.
	if biz.IsProgressiveSkillLoad(ag.Settings.GetSkillLoadMode()) {
		return newProgressiveSkillGuidanceHook(ag, deps)
	}
	// Full Profile mode (non-progressive): inject full skill guidance.
	// Only active in "complete" prompt mode.
	if !SkillsUseFullProfile(ag.SystemPromptMode) {
		return nil
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
			if totalChars+utf8.RuneCountInString(entry) > maxSkillGuidanceChars {
				remaining := len(entries) - written
				if remaining > 0 {
					b.WriteString(fmt.Sprintf("... and %d more skills (truncated)\n", remaining))
				}
				break
			}
			b.WriteString(entry)
			totalChars += utf8.RuneCountInString(entry)
			written++
		}
		if written == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if written < len(entries) {
			b.WriteString("\n> Other available skills can be loaded on demand using the skill_load tool.\n")
		}
		cue := truncateAtMarkdownBoundary(b.String(), maxSkillGuidanceChars)
		// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
		recordContextBudgetOnce(ctx, ContextBudgetCategorySkillGuidance, utf8.RuneCountInString(cue))
		// Prefix stabilization: append after the existing system block.
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = insertAfterLastSystem(args.Request.Messages, sys)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// newProgressiveSkillGuidanceHook returns a BeforeModel hook that:
//  1. Resolves routed skill slugs and writes them to invocation state (for
//     health metrics persistence by the invocation recorder).
//  2. Injects a compact system message listing routed slugs so the LLM knows
//     which skills were routed for this turn.
//
// The framework's SkillsRequestProcessor.injectOverview lists ALL skills with
// descriptions but does not distinguish routed ones. This message complements
// the overview by highlighting routed slugs, guiding the LLM's skill_load
// decisions. This is the progressive mode's [routed] marker equivalent.
func newProgressiveSkillGuidanceHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	return callbacks.NewBeforeModelHook(5, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		result := resolveAndWriteSkillState(ctx, ag.Settings, deps, true)
		if result == nil || len(result.Slugs) == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// Inject routed slugs as a compact system message. The framework's
		// injectOverview will append the full skill list (names+descriptions)
		// to the system message; this prefix highlights routed skills so the
		// LLM can prioritize loading them via skill_load.
		var b strings.Builder
		b.WriteString("## Routed Skills\n\n")
		b.WriteString("The following skills are routed for this turn. ")
		b.WriteString("Prefer loading these with the skill_load tool before invoking skill_run.\n\n")
		for _, slug := range result.Slugs {
			b.WriteString(fmt.Sprintf("- %s\n", slug))
		}
		// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
		recordContextBudgetOnce(ctx, ContextBudgetCategorySkillGuidance, utf8.RuneCountInString(b.String()))
		sys := trpcmodel.NewSystemMessage(b.String())
		args.Request.Messages = insertAfterLastSystem(args.Request.Messages, sys)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// resolveAndWriteSkillState resolves routed skill slugs and writes them to
// invocation state. When progressive is true, routed slugs are stored under
// skillRoutedSlugsStateKey so the invocation recorder can persist them for
// health metrics. Returns nil when no skills are resolved or on error.
func resolveAndWriteSkillState(ctx context.Context, runtime *biz.AgentRuntimeSettings, deps TRPCBuilderDeps, progressive bool) *skillruntime.ResolveResult {
	opts := &skillruntime.SkillToolsetOptions{UserQuery: skillruntime.TurnQueryFromContext(ctx)}
	// Avoid typed-nil interface: only set Runtime if runtime is non-nil,
	// otherwise the nil *biz.AgentRuntimeSettings creates a non-nil interface
	// wrapping a nil pointer, causing panic in ResolveSkillSlugsDetailed.
	if runtime != nil {
		opts.Runtime = runtime
	}
	result, err := skillruntime.ResolveSkillSlugsDetailed(ctx, deps.SkillUC, opts, deps.Logger())
	if err != nil {
		deps.Logger().Warn("resolveAndWriteSkillState: ResolveSkillSlugsDetailed failed",
			loggateway.StepID("agent.skill_guidance"),
			loggateway.Err(err))
		return nil
	}
	if len(result.Slugs) == 0 {
		return nil
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok {
		return result
	}
	if progressive {
		// skillRoutedSlugsStateKey is read by the invocation recorder
		// to persist routed_slugs for health metrics.
		inv.SetState(skillRoutedSlugsStateKey, result.Slugs)
	}
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
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(args, &obj); err != nil {
		return ""
	}
	for _, key := range []string{"slug", "skill_slug", "skill"} {
		if raw, ok := obj[key]; ok {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				return s
			}
		}
	}
	return ""
}

// truncateAtMarkdownBoundary truncates content to at most limit runes, preferring
// to cut at a Markdown boundary (heading, horizontal rule, or blank line) rather
// than in the middle of a paragraph.
func truncateAtMarkdownBoundary(content string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	boundaries := []string{"\n### ", "\n---", "\n\n"}
	bestPos := -1
	for _, sep := range boundaries {
		searchStart := limit - limit/4
		if searchStart < 0 {
			searchStart = 0
		}
		sepRunes := []rune(sep)
		// Search backwards from limit for the separator
		for i := limit - len(sepRunes); i >= searchStart; i-- {
			if i < 0 {
				break
			}
			match := true
			for j, sr := range sepRunes {
				if runes[i+j] != sr {
					match = false
					break
				}
			}
			if match && i > bestPos {
				bestPos = i
				break
			}
		}
	}
	if bestPos > 0 {
		return string(runes[:bestPos]) + "\n..."
	}
	return string(runes[:limit]) + "\n..."
}
