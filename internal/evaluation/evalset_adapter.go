package evaluation

import (
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcevalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	AppName = "aranea"

	MetricExactMatch       = "exact_match"
	MetricContainsMatch    = "contains_match"
	MetricLLMAsJudge       = "llm_as_judge"
	MetricToolCallAccuracy = "tool_call_accuracy"
)

func FrameworkMetricNames() []string {
	return []string{MetricExactMatch, MetricContainsMatch, MetricLLMAsJudge, MetricToolCallAccuracy}
}

func BizCasesToEvalSet(dataset biz.EvalDataset, cases []biz.EvalCase, lg loggateway.Logger) *trpcevalset.EvalSet {
	es := &trpcevalset.EvalSet{
		EvalSetID: dataset.ID,
		Name:      dataset.Name,
	}
	for _, c := range cases {
		es.EvalCases = append(es.EvalCases, bizCaseToEvalCase(c, lg))
	}
	return es
}

func bizCaseToEvalCase(c biz.EvalCase, lg loggateway.Logger) *trpcevalset.EvalCase {
	evalID := c.ID
	if evalID == "" {
		evalID = fmt.Sprintf("case-%s", c.DatasetID)
	}
	meta := ParseCaseMetadata(c.MetadataJSON, lg)
	ec := &trpcevalset.EvalCase{
		EvalID:       evalID,
		Conversation: invocationsFromCase(c, meta),
		// ISSUE-005: the framework hard-requires SessionInput on every case
		// (local inference fails with "session input is nil" otherwise).
		SessionInput: sessionInputFromMeta(meta),
	}
	if scenario := buildConversationScenario(meta, c.Input); scenario != nil {
		ec.ConversationScenario = scenario
	}
	// P3-2: case-level rubric → framework EvalCaseRubric on the llm_as_judge
	// metric instance; the judge merges it into the scoring criterion.
	// The llm_rubric_response evaluator hard-requires at least one rubric per
	// case ("llm judge rubrics are required"), and its prompt never shows the
	// expected answer — so cases without a custom rubric get a synthesized
	// default that embeds the reference answer, or a generic quality rubric
	// when there is none. A custom rubric is authoritative and replaces the
	// default.
	rubricText := strings.TrimSpace(meta.Rubric)
	if rubricText == "" {
		rubricText = defaultRubricText(c.ExpectedOutput)
	}
	ec.Rubrics = append(ec.Rubrics, &trpcevalset.EvalCaseRubric{
		MetricName: MetricLLMAsJudge,
		ID:         evalID + "-rubric",
		Content:    &trpcevalset.EvalCaseRubricContent{Text: rubricText},
	})
	enrichEvalCase(c, ec, lg)
	return ec
}

// defaultRubricText synthesizes the fallback scoring standard for cases
// without a custom rubric. With an expected output the judge scores semantic
// consistency against the embedded reference answer; otherwise it scores
// generic response quality.
func defaultRubricText(expectedOutput string) string {
	if expected := strings.TrimSpace(expectedOutput); expected != "" {
		return fmt.Sprintf("判断最终回答与参考答案是否语义一致、结论正确（允许等价表述与数值等价）。参考答案：%s", expected)
	}
	return "评估最终回答的整体质量：准确、相关、完整、表达清晰，且直接回应了用户请求。"
}

// stripCaseRubricsWhenNoJudge removes case-level rubrics when llm_as_judge is
// not among the run's metrics: the framework fails any case whose rubric
// references a metric instance that was not registered for the run.
func stripCaseRubricsWhenNoJudge(es *trpcevalset.EvalSet, want map[string]bool) {
	if es == nil || want[MetricLLMAsJudge] {
		return
	}
	for _, ec := range es.EvalCases {
		if ec != nil {
			ec.Rubrics = nil
		}
	}
}

// sessionInputFromMeta builds the mandatory SessionInput, applying optional
// metadata overrides (session_user_id / session_state) on top of defaults.
func sessionInputFromMeta(meta CaseMetadata) *trpcevalset.SessionInput {
	si := &trpcevalset.SessionInput{
		AppName: AppName,
		UserID:  "eval",
		State:   map[string]any{},
	}
	if uid := strings.TrimSpace(meta.SessionUserID); uid != "" {
		si.UserID = uid
	}
	for k, v := range meta.SessionState {
		si.State[k] = v
	}
	return si
}

func invocationsFromCase(c biz.EvalCase, meta CaseMetadata) []*trpcevalset.Invocation {
	if meta.HasMultiTurn() {
		return invocationsFromTurns(c.ID, meta.Turns, c.ExpectedOutput)
	}
	userMsg := trpcmodel.NewUserMessage(c.Input)
	expectedMsg := trpcmodel.NewAssistantMessage(c.ExpectedOutput)
	return []*trpcevalset.Invocation{{
		InvocationID:  c.ID + "-inv-0",
		UserContent:   &userMsg,
		FinalResponse: &expectedMsg,
	}}
}

func invocationsFromTurns(caseID string, turns []EvalTurn, fallbackExpected string) []*trpcevalset.Invocation {
	var invs []*trpcevalset.Invocation
	var pendingUser string
	invIdx := 0
	flush := func(user, expected string) {
		if strings.TrimSpace(user) == "" {
			return
		}
		if expected == "" {
			expected = fallbackExpected
		}
		u := trpcmodel.NewUserMessage(user)
		a := trpcmodel.NewAssistantMessage(expected)
		invs = append(invs, &trpcevalset.Invocation{
			InvocationID:  fmt.Sprintf("%s-inv-%d", caseID, invIdx),
			UserContent:   &u,
			FinalResponse: &a,
		})
		invIdx++
	}
	for _, t := range turns {
		role := strings.ToLower(strings.TrimSpace(t.Role))
		content := t.Content
		switch role {
		case "user":
			if pendingUser != "" {
				flush(pendingUser, "")
			}
			pendingUser = content
		case "assistant":
			if pendingUser != "" {
				flush(pendingUser, content)
				pendingUser = ""
			}
		}
	}
	if pendingUser != "" {
		flush(pendingUser, fallbackExpected)
	}
	if len(invs) == 0 && strings.TrimSpace(fallbackExpected) != "" {
		u := trpcmodel.NewUserMessage("")
		a := trpcmodel.NewAssistantMessage(fallbackExpected)
		invs = append(invs, &trpcevalset.Invocation{
			InvocationID:  caseID + "-inv-0",
			UserContent:   &u,
			FinalResponse: &a,
		})
	}
	return invs
}
