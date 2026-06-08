package agent

import (
	"context"
	"os"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const promptSnapshotStateKey = "aranea:prompt_snapshot_calls"

func promptSnapshotEnabled() bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_PROMPT_SNAPSHOT"))
	return v == "" || v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "on")
}

func newPromptSnapshotBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if !promptSnapshotEnabled() && !l0SnapshotHookActive(ag) {
		return nil
	}
	return callbacks.NewBeforeModelHook(10, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		callIndex := promptSnapshotCallIndex(ctx)
		report := analyzePromptRequest(args.Request.Messages)
		if promptSnapshotEnabled() {
			deps.Logger().Info("Prompt 组成快照", loggateway.StepID("agent.prompt.compose"), loggateway.Phase("done"),
				loggateway.Int("model_call_index", callIndex),
				loggateway.Int("est_tokens", report.EstTokens),
				loggateway.Int("system_chars", report.SystemChars),
				loggateway.Int("user_chars", report.UserChars),
				loggateway.Int("assistant_chars", report.AssistantChars),
				loggateway.Int("tool_chars", report.ToolChars),
				loggateway.Int("system_msgs", report.SystemMsgs),
				loggateway.Int("user_msgs", report.UserMsgs),
				loggateway.Int("assistant_msgs", report.AssistantMsgs),
				loggateway.Int("tool_msgs", report.ToolMsgs),
				loggateway.Int("section_identity", report.Sections["identity"]),
				loggateway.Int("section_instruction", report.Sections["instruction"]),
				loggateway.Int("section_runtime_cue", report.Sections["runtime_cue"]),
				loggateway.Int("section_skills", report.Sections["skills"]),
				loggateway.Int("section_l1", report.Sections["l1_memory"]),
				loggateway.Int("section_l2", report.Sections["l2_memory"]),
				loggateway.Int("section_l2_l3", report.Sections["l2_l3_memory"]),
				loggateway.Int("section_l3", report.Sections["l3_memory"]),
				loggateway.Int("section_l4", report.Sections["l4_memory"]),
				loggateway.Int("section_intent", report.Sections["intent"]),
				loggateway.Int("section_session_summary", report.Sections["session_summary"]),
				loggateway.Int("section_user_memories", report.Sections["user_memories"]),
			)
		}
		persistL0AssemblySnapshot(ctx, deps, ag, args.Request.Messages, callIndex)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func promptSnapshotCallIndex(ctx context.Context) int {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return 1
	}
	n := 1
	if v, ok := inv.GetState(promptSnapshotStateKey); ok {
		if i, ok := v.(int); ok {
			n = i + 1
		}
	}
	inv.SetState(promptSnapshotStateKey, n)
	return n
}

type promptComposeReport struct {
	SystemChars     int
	UserChars       int
	AssistantChars  int
	ToolChars       int
	SystemMsgs      int
	UserMsgs        int
	AssistantMsgs   int
	ToolMsgs        int
	EstTokens       int
	Sections        map[string]int
}

func analyzePromptRequest(messages []trpcmodel.Message) promptComposeReport {
	report := promptComposeReport{Sections: make(map[string]int)}
	for _, m := range messages {
		ch := messageCharLen(m)
		switch m.Role {
		case trpcmodel.RoleSystem:
			report.SystemMsgs++
			report.SystemChars += ch
			for k, v := range classifySystemSections(m.Content) {
				report.Sections[k] += v
			}
		case trpcmodel.RoleUser:
			report.UserMsgs++
			report.UserChars += ch
		case trpcmodel.RoleAssistant:
			report.AssistantMsgs++
			report.AssistantChars += ch
		case trpcmodel.RoleTool:
			report.ToolMsgs++
			report.ToolChars += ch
		}
	}
	total := report.SystemChars + report.UserChars + report.AssistantChars + report.ToolChars
	report.EstTokens = estTokensFromChars(total)
	return report
}

func messageCharLen(m trpcmodel.Message) int {
	n := utf8.RuneCountInString(strings.TrimSpace(m.Content))
	for _, p := range m.ContentParts {
		if p.Text != nil {
			n += utf8.RuneCountInString(strings.TrimSpace(*p.Text))
		}
	}
	return n
}

func estTokensFromChars(chars int) int {
	if chars <= 0 {
		return 0
	}
	t := chars / 4
	if t == 0 {
		return 1
	}
	return t
}

func classifySystemSections(text string) map[string]int {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	out := make(map[string]int)
	if strings.HasPrefix(text, "You are ") {
		out["identity"] = estTokensFromChars(utf8.RuneCountInString(textPrefixUntilBlankLine(text)))
	}
	if strings.Contains(text, "<internal_config") {
		out["instruction"] += sectionTokens(text, "<internal_config", "## Runtime capability policy")
	}
	if strings.Contains(text, "## Runtime capability policy") {
		out["runtime_cue"] += sectionTokens(text, "## Runtime capability policy", "## L1 working memory")
	}
	if strings.Contains(text, "## L1 working memory") {
		out["l1_memory"] += sectionTokens(text, "## L1 working memory", "## L2 episodic")
	}
	if strings.Contains(text, "## L2+L3 memory") {
		out["l2_l3_memory"] += sectionTokens(text, "## L2+L3 memory", "## L4 knowledge")
	}
	if strings.Contains(text, "## L2 episodic memory") {
		out["l2_memory"] += sectionTokens(text, "## L2 episodic memory", "## L3 semantic")
	}
	if strings.Contains(text, "## L3 semantic memory") {
		out["l3_memory"] += sectionTokens(text, "## L3 semantic memory", "## L4 knowledge")
	}
	if strings.Contains(text, "## L4 knowledge graph") {
		out["l4_memory"] += sectionTokens(text, "## L4 knowledge graph", "")
	}
	if strings.Contains(text, "Available skills:") {
		out["skills"] += sectionTokens(text, "Available skills:", "## ")
	}
	if strings.Contains(text, "Derived intent (align your plan and tools to this JSON):") {
		out["intent"] += sectionTokens(text, "Derived intent (align your plan and tools to this JSON):", "")
	}
	if strings.Contains(text, "## User Memories") {
		out["user_memories"] += sectionTokens(text, "## User Memories", "")
	}
	if strings.Contains(text, "Session summary") || strings.Contains(text, "session summary") {
		out["session_summary"] += estTokensFromChars(utf8.RuneCountInString(text))
	}
	if out["instruction"] == 0 && out["runtime_cue"] == 0 && out["identity"] == 0 {
		out["instruction"] = estTokensFromChars(utf8.RuneCountInString(text))
	}
	return out
}

func textPrefixUntilBlankLine(text string) string {
	if i := strings.Index(text, "\n\n"); i >= 0 {
		return text[:i]
	}
	return text
}

func sectionTokens(text, startMarker, endMarker string) int {
	i := strings.Index(text, startMarker)
	if i < 0 {
		return 0
	}
	seg := text[i:]
	if endMarker != "" {
		if j := strings.Index(seg[len(startMarker):], endMarker); j >= 0 {
			seg = seg[:len(startMarker)+j]
		}
	}
	return estTokensFromChars(utf8.RuneCountInString(seg))
}
