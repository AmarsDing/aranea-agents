package evaluation

import (
	"fmt"

	"aranea-agents/internal/biz"

	trpcevalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
)

func attachExpectedTools(inv *trpcevalset.Invocation, meta CaseMetadata) {
	tools := expectedToolsFromMeta(meta)
	if len(tools) == 0 {
		return
	}
	inv.Tools = tools
}

func expectedToolsFromMeta(meta CaseMetadata) []*trpcevalset.Tool {
	entries := meta.expectedToolEntries()
	if len(entries) == 0 {
		return nil
	}
	out := make([]*trpcevalset.Tool, 0, len(entries))
	for i, e := range entries {
		out = append(out, &trpcevalset.Tool{
			ID:        fmt.Sprintf("exp-tool-%d", i),
			Name:      e.Name,
			Arguments: e.Arguments,
			Result:    e.Result,
		})
	}
	return out
}

func enrichEvalCase(c biz.EvalCase, ec *trpcevalset.EvalCase) {
	meta := ParseCaseMetadata(c.MetadataJSON)
	if ec.ConversationScenario == nil && meta.HasLLMSimulation() {
		enrichConversationScenario(&c, ec)
	}
	if len(ec.Conversation) > 0 {
		attachExpectedTools(ec.Conversation[len(ec.Conversation)-1], meta)
	}
}
