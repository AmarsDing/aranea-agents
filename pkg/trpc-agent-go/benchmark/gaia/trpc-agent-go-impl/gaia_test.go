//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package gaiaeval

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion"
	criterionllm "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/llm"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

var verifierPromptLeakageTerms = []string{
	"GAIA",
	"gaia",
	"17000",
	"thousand hours",
	"Kipchoge",
	"Moon",
	"e1fc63a2-da7a-432f-be78-7c4a95598703",
	"ground truth",
	"reference answer",
}

func assertNoVerifierPromptLeakage(t *testing.T, text string) {
	t.Helper()
	for _, term := range verifierPromptLeakageTerms {
		assert.NotContains(t, text, term)
	}
}

func TestGAIAFinalAnswerVerifier(t *testing.T) {
	t.Parallel()

	v := gaiaFinalAnswerVerifier{}

	res, err := v.Verify(
		context.Background(),
		&agent.Invocation{},
		nil,
	)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatalf("Passed = true, want false")
	}

	okEvt := &event.Event{
		Response: &model.Response{
			Done: true,
			Choices: []model.Choice{{
				Index: 0,
				Message: model.Message{
					Role:    model.RoleAssistant,
					Content: "FINAL ANSWER: 42",
				},
			}},
		},
	}
	res, err = v.Verify(context.Background(), &agent.Invocation{}, okEvt)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Passed {
		t.Fatalf("Passed = false, want true")
	}

	partText := "FINAL ANSWER: 42"
	okEvtParts := &event.Event{
		Response: &model.Response{
			Done: true,
			Choices: []model.Choice{{
				Index: 0,
				Message: model.Message{
					Role: model.RoleAssistant,
					ContentParts: []model.ContentPart{{
						Type: model.ContentTypeText,
						Text: &partText,
					}},
				},
			}},
		},
	}
	res, err = v.Verify(context.Background(), &agent.Invocation{}, okEvtParts)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Passed {
		t.Fatalf("Passed = false, want true (ContentParts)")
	}

	badEvt := &event.Event{
		Response: &model.Response{
			Done: true,
			Choices: []model.Choice{{
				Index: 0,
				Message: model.Message{
					Role:    model.RoleAssistant,
					Content: "Answer: 42",
				},
			}},
		},
	}
	res, err = v.Verify(context.Background(), &agent.Invocation{}, badEvt)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatalf("Passed = true, want false")
	}
	if !strings.Contains(res.Feedback, gaiaFinalAnswerPrefix) {
		t.Fatalf(
			"feedback missing %q: %q",
			gaiaFinalAnswerPrefix,
			res.Feedback,
		)
	}
}

func TestGAIAFinalAnswerVerifier_ReturnTypes(t *testing.T) {
	t.Parallel()

	var v gaiaFinalAnswerVerifier
	var _ runner.Verifier = v
}

func TestExtractFinalAnswer_BlockFinalAnswer(t *testing.T) {
	t.Parallel()

	content := `/*REASONING*/
Simulation and analysis show position scores: pos1 = 1/3, pos2 = 5/9, pos3 = 17/27.

/*FINAL_ANSWER*/
3`

	assert.Equal(t, "3", extractFinalAnswer(content))
}

func TestExtractFinalAnswer_BlockFinalAnswerWithPrefix(t *testing.T) {
	t.Parallel()

	content := `/*REASONING*/
The candidate should return a concise final answer.

/*FINAL ANSWER*/
FINAL ANSWER: 100`

	assert.Equal(t, "100", extractFinalAnswer(content))
}

func TestVerifyAnswer_NumericDoesNotUseContains(t *testing.T) {
	t.Parallel()

	assert.False(t, verifyAnswer("17000", "17"))
	assert.True(t, verifyAnswer("17", "17"))
}

func TestFormatAnswer_PreservesNumericCommaList(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		"132, 133, 134, 197, 245",
		formatAnswer("132,133,134,197,245"),
	)
	assert.Equal(t, "89706", formatAnswer("89,706"))
}

func TestVerifyAnswer_NumericCommaList(t *testing.T) {
	t.Parallel()

	assert.True(
		t,
		verifyAnswer("132,133,134,197,245", "132, 133, 134, 197, 245"),
	)
	assert.True(t, verifyAnswer("89,706", "89706"))
}

func TestFilterTasksByID_PrefersExactIDBeforeIndex(t *testing.T) {
	t.Parallel()
	tasks := []GAIATask{
		{TaskID: "11111111-1111-1111-1111-111111111111"},
		{TaskID: "5d0080cb-90d7-4712-bc33-848150e917d3"},
		{TaskID: "33333333-3333-3333-3333-333333333333"},
		{TaskID: "44444444-4444-4444-4444-444444444444"},
		{TaskID: "55555555-5555-5555-5555-555555555555"},
	}
	selected := filterTasksByID(tasks, "5d0080cb-90d7-4712-bc33-848150e917d3")
	require.Len(t, selected, 1)
	assert.Equal(t, "5d0080cb-90d7-4712-bc33-848150e917d3", selected[0].TaskID)
	selected = filterTasksByID(tasks, "5")
	require.Len(t, selected, 1)
	assert.Equal(t, "55555555-5555-5555-5555-555555555555", selected[0].TaskID)
}

func TestGAIAPairwiseMessagesConstructor_IncludesTraceWithoutGroundTruth(t *testing.T) {
	t.Parallel()

	constructor := newGAIAPairwiseMessagesConstructor()
	userContent := model.NewUserMessage("How many thousand hours are in 17000 hours?")
	actualFinal := model.NewAssistantMessage("FINAL ANSWER: 17000")
	expectedFinal := model.NewAssistantMessage("FINAL ANSWER: 17")
	actual := &evalset.Invocation{
		UserContent:   &userContent,
		FinalResponse: &actualFinal,
		Tools: []*evalset.Tool{{
			Name:      "execute_python",
			Arguments: map[string]any{"code": "print(17000 / 1000)"},
			Result:    "17",
		}},
	}
	expected := &evalset.Invocation{
		UserContent:   &userContent,
		FinalResponse: &expectedFinal,
	}
	messages, err := constructor.ConstructMessages(
		context.Background(),
		[]*evalset.Invocation{actual},
		[]*evalset.Invocation{expected},
		&metric.EvalMetric{
			Criterion: criterion.New(
				criterion.WithLLMJudge(
					criterionllm.New("", "",
						criterionllm.WithRubrics([]*criterionllm.Rubric{{
							ID: "unit-scale",
							Content: &criterionllm.RubricContent{
								Text: "Respect the requested unit scale.",
							},
						}}),
					),
				),
			),
		},
	)

	assert.NoError(t, err)
	assert.Len(t, messages, 1)
	prompt := messages[0].Content
	assert.Contains(t, prompt, "How many thousand hours are in 17000 hours?")
	assert.Contains(t, prompt, "FINAL ANSWER: 17000")
	assert.Contains(t, prompt, "FINAL ANSWER: 17")
	assert.Contains(t, prompt, "execute_python")
	assert.Contains(t, prompt, "print(17000 / 1000)")
	assert.Contains(t, prompt, "Never use any information outside the visible request and candidate traces")
	assert.Contains(t, prompt, "Do not assume access to hidden answers, labels, scores, or task metadata.")
	assert.Contains(t, prompt, "Respect the requested unit scale.")
	assert.Contains(t, prompt, "<score_A>LETTER_A_TO_T</score_A>")
	assert.NotContains(t, prompt, "GAIA")
	assert.NotContains(t, prompt, "ground truth")
	assert.NotContains(t, prompt, "reference answer")
}

func TestGAIAPairwisePrompt_IsDatasetAgnostic(t *testing.T) {
	t.Parallel()

	prompt := gaiaPairwisePrompt("", "", "", "")

	assertNoVerifierPromptLeakage(t, prompt)
	assert.Contains(t, prompt, "Mandatory Evaluation Steps")
	assert.Contains(t, prompt, "required final answer form")
	assert.Contains(t, prompt, "explicit constraints")
	assert.Contains(t, prompt, "visible evidence trace")
	assert.Contains(t, prompt, "Do not replace the visible evidence trace with your own background knowledge")
	assert.Contains(t, prompt, "unit or scale")
	assert.Contains(t, prompt, "form requested by the user")
	assert.Contains(t, prompt, "requested value without adding unrequested labels")
	assert.Contains(t, prompt, "requested string granularity")
	assert.Contains(t, prompt, "source-visible punctuation, hyphenation, apostrophes")
	assert.Contains(t, prompt, "title-page text, catalog metadata, publisher or official metadata")
	assert.Contains(t, prompt, "amendment note, changelog, version diff")
	assert.Contains(t, prompt, "Plain text mirrors and snippets can lose layout")
	assert.Contains(t, prompt, "visibly preserves the requested layout")
	assert.Contains(t, prompt, "final-only candidate with no evidence trace")
	assert.Contains(t, prompt, "answer-key pages, benchmark datasets, previous agent traces")
	assert.Contains(t, prompt, "same user request, another agent's transcript, run logs")
	assert.Contains(t, prompt, "prior final-answer markers")
	assert.Contains(t, prompt, "Quoted words and phrases")
	assert.Contains(t, prompt, "singular versus plural forms")
	assert.Contains(t, prompt, "count only comparable items")
	assert.Contains(t, prompt, "Do not count parent section names")
	assert.Contains(t, prompt, "member titles inside each candidate container")
	assert.Contains(t, prompt, "same core value")
	assert.Contains(t, prompt, "easiest to extract exactly")
	assert.Contains(t, prompt, "unit named in the question as context")
	assert.Contains(t, prompt, "more familiar modern or nearby entity")
	assert.Contains(t, prompt, "multi-hop requests")
	assert.Contains(t, prompt, "collection choice, sorting, counting, exact quoted-term matching")
	assert.Contains(t, prompt, "Classify the support behind each candidate")
	assert.Contains(t, prompt, "contaminated secondary evidence")
	assert.Contains(t, prompt, "unsupported plausibility is not evidence")
	assert.Contains(t, prompt, "exact term form and exact text scope")
	assert.Contains(t, prompt, "punctuation, hyphenation, apostrophes, capitalization")
	assert.Contains(t, prompt, "most authoritative visible source")
	assert.Contains(t, prompt, "source statement or diff that directly names the changed term")
	assert.Contains(t, prompt, "relevant member titles")
	assert.Contains(t, prompt, "assign meaningfully different score letters")
	assert.Contains(t, prompt, "do not give them the same score")
	assert.Contains(t, prompt, "hidden answers, labels, scores, or task metadata")
}

func TestGAIALLMVerifierMetric_RubricsAreGeneric(t *testing.T) {
	t.Parallel()

	evalMetric := gaiaLLMVerifierMetric()
	var rubricText strings.Builder
	for _, rubric := range evalMetric.Criterion.LLMJudge.Rubrics {
		rubricText.WriteString(rubric.ID)
		rubricText.WriteString("\n")
		rubricText.WriteString(rubric.Content.Text)
		rubricText.WriteString("\n")
	}
	text := rubricText.String()

	assertNoVerifierPromptLeakage(t, text)
	assert.NotContains(t, text, "Judge only from the user request and the two final responses")
	assert.Contains(t, text, "answer_form_alignment")
	assert.Contains(t, text, "evidence_grounding")
	assert.Contains(t, text, "evidence_over_prior_knowledge")
	assert.Contains(t, text, "source_quality")
	assert.Contains(t, text, "contaminated_evidence_penalty")
	assert.Contains(t, text, "trace_sufficiency")
	assert.Contains(t, text, "final_answer_fidelity")
	assert.Contains(t, text, "constraint_completion")
	assert.Contains(t, text, "quoted_term_scope")
	assert.Contains(t, text, "exact_string_fidelity")
	assert.Contains(t, text, "exact_source_priority")
	assert.Contains(t, text, "change_evidence_directness")
	assert.Contains(t, text, "quoted_collection_scope")
	assert.Contains(t, text, "layout_evidence_visibility")
	assert.Contains(t, text, "multi_hop_resolution")
	assert.Contains(t, text, "concise_extractable_answer")
	assert.Contains(t, text, "output_form_penalties")
	assert.Contains(t, text, "numeric_extractability_tiebreak")
	assert.Contains(t, text, "entity_list_identity")
	assert.Contains(t, text, "meaningful_pairwise_separation")
	assert.Contains(t, text, "bare value or include labels")
	assert.Contains(t, text, "visible tool outputs")
	assert.Contains(t, text, "background knowledge or plausibility assumptions")
	assert.Contains(t, text, "answer-key pages, benchmark datasets, previous agent traces")
	assert.Contains(t, text, "retrieved copies of the same user request")
	assert.Contains(t, text, "prior final-answer markers")
	assert.Contains(t, text, "final-only candidates with no evidence trace")
	assert.Contains(t, text, "preserves the exact value supported by its own evidence trace")
	assert.Contains(t, text, "positional selection rules")
	assert.Contains(t, text, "exact term form and count only within the requested text scope")
	assert.Contains(t, text, "requested string granularity")
	assert.Contains(t, text, "source-visible punctuation, hyphenation, apostrophes")
	assert.Contains(t, text, "title-page text, catalog metadata, publisher or official metadata")
	assert.Contains(t, text, "amendment note, changelog, version diff")
	assert.Contains(t, text, "count only comparable items")
	assert.Contains(t, text, "group, section, article, category, or container")
	assert.Contains(t, text, "member titles")
	assert.Contains(t, text, "visibly preserves the requested layout")
	assert.Contains(t, text, "plain text mirrors, snippets, short lines, or line breaks")
	assert.Contains(t, text, "collection choice, sorting, counting, exact quoted-term matching")
	assert.Contains(t, text, "add unrequested units, labels, narrative text, or alternative values")
	assert.Contains(t, text, "same core numeric value")
	assert.Contains(t, text, "unit or scale named in the question as context")
	assert.Contains(t, text, "requested identity, extrema, temporal wording, item count, and ordering")
	assert.Contains(t, text, "give the better-supported candidate a clearly better score")
}

func TestGAIAJudgeGenerationConfigUsesDeterministicSettings(t *testing.T) {
	t.Parallel()
	config := gaiaJudgeGenerationConfig(123)
	require.NotNil(t, config.MaxTokens)
	assert.Equal(t, 123, *config.MaxTokens)
	require.NotNil(t, config.Temperature)
	assert.Equal(t, 0.0, *config.Temperature)
	require.NotNil(t, config.Logprobs)
	assert.True(t, *config.Logprobs)
	require.NotNil(t, config.TopLogprobs)
	assert.Equal(t, 20, *config.TopLogprobs)
	assert.False(t, config.Stream)
}

func TestGAIAJudgeModelOptionsUseOpenAIEnv(t *testing.T) {
	var requestPath string
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "judge-env-test",
			"object": "chat.completion",
			"created": 1699200000,
			"model": "deepseek-v4-flash",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "ok"
					},
					"finish_reason": "stop"
				}
			]
		}`)
	}))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("OPENAI_BASE_URL", server.URL)
	judgeModel := openai.New("deepseek-v4-flash", gaiaJudgeModelOptions()...)
	responses, err := judgeModel.GenerateContent(context.Background(), &model.Request{
		Messages: []model.Message{
			model.NewUserMessage("hi"),
		},
		GenerationConfig: model.GenerationConfig{
			Stream: false,
		},
	})
	assert.NoError(t, err)
	if err == nil {
		for response := range responses {
			if response.Error != nil {
				assert.NoError(t, response.Error)
			}
			if response.Done {
				break
			}
		}
	}
	assert.True(t, strings.HasSuffix(requestPath, "/chat/completions"))
	assert.Equal(t, "Bearer test-openai-key", authorization)
}
