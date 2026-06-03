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
)

const maxSkillGuidanceChars = 4000

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
	return callbacks.NewBeforeModelHook(5, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		runtime := ag.Settings
		opts := &skillruntime.SkillToolsetOptions{Runtime: runtime, UserQuery: skillruntime.TurnQueryFromContext(ctx)}
		result, err := skillruntime.ResolveSkillSlugsDetailed(ctx, deps.SkillUC, opts, deps.LG)
		if err != nil || len(result.Slugs) == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		entries, err := deps.SkillUC.BatchGetSkillGuidance(ctx, result.Slugs)
		if err != nil || len(entries) == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		var b strings.Builder
		b.WriteString("## Available Skills\n\nThe following skills are available for this turn. Use the skill_run tool to invoke them.\n\n")
		totalChars := 0
		written := 0
		for _, e := range entries {
			m := manifest.Parse(e.Guidance)
			guidance := render.SkillGuidance(m, render.RenderOptions{})
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
		cue := b.String()
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func newProgressiveSkillGuidanceHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	return callbacks.NewBeforeModelHook(5, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		runtime := ag.Settings
		opts := &skillruntime.SkillToolsetOptions{Runtime: runtime, UserQuery: skillruntime.TurnQueryFromContext(ctx)}
		result, err := skillruntime.ResolveSkillSlugsDetailed(ctx, deps.SkillUC, opts, deps.LG)
		if err != nil || len(result.Slugs) == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// Store routed skill names in invocation state so the
		// SkillsRequestProcessor can mark them as [routed] in the
		// overview on subsequent turns.
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok {
			inv.SetState(trpcllmagent.RoutedSkillsStateKey, result.Slugs)
		}
		entries, err := deps.SkillUC.BatchGetSkillGuidance(ctx, result.Slugs)
		if err != nil || len(entries) == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		var b strings.Builder
		b.WriteString("## Available Skills\n\nThe following skills are available. Use the skill_load tool to load specific skill details.\n\n")
		for _, e := range entries {
			m := manifest.Parse(e.Guidance)
			name := strings.TrimSpace(m.Name)
			if name == "" {
				name = e.Slug
			}
			desc := strings.TrimSpace(m.Description)
			if desc != "" {
				fmt.Fprintf(&b, "- **%s**: %s\n", name, desc)
			} else {
				fmt.Fprintf(&b, "- **%s**\n", name)
			}
		}
		cue := b.String()
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
