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
	return callbacks.NewBeforeModelHook(5, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		runtime := ag.Settings
		opts := &skillruntime.SkillToolsetOptions{Runtime: runtime, UserQuery: skillruntime.TurnQueryFromContext(ctx)}
		result, err := skillruntime.ResolveSkillSlugsDetailed(ctx, deps.SkillUC, opts)
		if err != nil || len(result.Slugs) == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		var b strings.Builder
		b.WriteString("## Available Skills\n\nThe following skills are available for this turn. Use the skill_run tool to invoke them.\n\n")
		totalChars := 0
		written := 0
		for _, slug := range result.Slugs {
			skill, err := deps.SkillUC.GetBySkillKey(ctx, slug)
			if err != nil {
				continue
			}
			markdown, err := deps.SkillUC.GetLatestMarkdown(ctx, skill.ID)
			if err != nil {
				markdown = ""
			}
			m := manifest.Parse(markdown)
			guidance := render.SkillGuidance(m, render.RenderOptions{})
			entry := fmt.Sprintf("### %s\n%s\n\n", slug, guidance)
			if totalChars+len(entry) > maxSkillGuidanceChars {
				remaining := len(result.Slugs) - written
				if remaining > 0 {
					b.WriteString(fmt.Sprintf("... and %d more skills (truncated)\n", remaining))
				}
				break
			}
			b.WriteString(entry)
			totalChars += len(entry)
			written++
		}
		cue := b.String()
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
