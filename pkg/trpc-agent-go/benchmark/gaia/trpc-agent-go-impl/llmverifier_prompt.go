//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package gaiaeval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/llm/verifierpairwise"
	evaluatorregistry "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/registry"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

type gaiaPairwiseMessagesConstructor struct{}

func newGAIAVerifierRegistry() evaluatorregistry.Registry {
	r := evaluatorregistry.New()
	e := verifierpairwise.New(
		verifierpairwise.WithMessagesConstructor(newGAIAPairwiseMessagesConstructor()),
	)
	_ = r.Register(e.Name(), e)
	return r
}

func newGAIAPairwiseMessagesConstructor() *gaiaPairwiseMessagesConstructor {
	return &gaiaPairwiseMessagesConstructor{}
}

func (c *gaiaPairwiseMessagesConstructor) ConstructMessages(
	_ context.Context,
	actuals []*evalset.Invocation,
	expecteds []*evalset.Invocation,
	evalMetric *metric.EvalMetric,
) ([]model.Message, error) {
	if len(actuals) == 0 {
		return nil, fmt.Errorf("actuals is empty")
	}
	if len(expecteds) == 0 {
		return nil, fmt.Errorf("expecteds is empty")
	}
	actual := actuals[len(actuals)-1]
	expected := expecteds[len(expecteds)-1]
	if actual == nil {
		return nil, fmt.Errorf("actual invocation is nil")
	}
	if expected == nil {
		return nil, fmt.Errorf("expected invocation is nil")
	}
	userInput := messageText(actual.UserContent)
	if userInput == "" {
		userInput = messageText(expected.UserContent)
	}
	prompt := gaiaPairwisePrompt(
		userInput,
		rubricsText(evalMetric),
		formatCandidateForJudge(actual),
		formatCandidateForJudge(expected),
	)
	return []model.Message{model.NewUserMessage(prompt)}, nil
}

func gaiaPairwisePrompt(userInput string, criteria string, candidateA string, candidateB string) string {
	var b strings.Builder
	b.WriteString("# Mission\n\n")
	b.WriteString("You are an expert evaluator for agent responses. You will judge two candidate runs for the same user request.\n\n")
	b.WriteString("# Critical Rules\n\n")
	b.WriteString("- Judge only from the user request and each candidate's final answer plus evidence trace.\n")
	b.WriteString("- Do not reward confidence, fluent prose, or claims of verification unless the evidence trace supports the final answer.\n")
	b.WriteString("- Do not replace the visible evidence trace with your own background knowledge, web knowledge, or what answer seems familiar. If the trace does not prove the chosen entity, word, string, count, or value, treat it as unsupported even when it sounds plausible.\n")
	b.WriteString("- For nontrivial retrieval, computation, document, media, or file questions, treat a final-only candidate with no evidence trace as weak. Do not prefer it over a candidate whose visible evidence directly supports a different answer unless that evidence is itself flawed.\n")
	b.WriteString("- Treat answer-key pages, benchmark datasets, previous agent traces, solution writeups, and pages that merely restate the same question and answer as weak secondary evidence, not primary evidence for the underlying task.\n")
	b.WriteString("- Treat retrieved content as contaminated when it contains the same user request, another agent's transcript, run logs, scenario files, evaluation files, or prior final-answer markers. Such content may explain why a candidate guessed an answer, but it should not establish correctness.\n")
	b.WriteString("- The final answer must satisfy the exact wording, unit, scale, rounding, and formatting requested by the user.\n")
	b.WriteString("- For exact extraction requests such as complete titles, names, quoted words, source-specific text, or source formatting, compare answers at the requested string granularity. Preserve source-visible punctuation, hyphenation, apostrophes, capitalization, separators, and historical or time-specific names unless the request says those differences do not matter.\n")
	b.WriteString("- When visible sources conflict on an exact title, name, or other bibliographic string, prefer title-page text, catalog metadata, publisher or official metadata, or another source that explicitly identifies the exact string over prose mentions, search snippets, or typography-normalized display text.\n")
	b.WriteString("- For amendment, change, addition, or deletion questions, prefer evidence that directly identifies the changed term in the selected item, such as an amendment note, changelog, version diff, or source statement. Do not infer the requested change from restyled current text unless the trace supports that exact amendment.\n")
	b.WriteString("- Plain text mirrors and snippets can lose layout such as indentation, table alignment, and visual grouping. Do not infer layout-sensitive answers from short lines or line breaks unless the candidate's evidence visibly preserves the requested layout.\n")
	b.WriteString("- Quoted words and phrases in the user request are exact matching constraints unless the request says otherwise. Preserve singular versus plural forms and count only within the requested scope, such as titles, headings, table cells, captions, or body text.\n")
	b.WriteString("- For quoted-term counting across a collection, count only comparable items in the requested collection. Do not count parent section names, category labels, or broader container headings unless the request explicitly includes them.\n")
	b.WriteString("- If the request asks which group, section, article, category, or container has a quoted term in the most member titles, first compare the member titles inside each candidate container. A parent/container title that contains the quoted term does not by itself prove that its member titles contain the term most often.\n")
	b.WriteString("- If a candidate computes an intermediate value in a different unit or scale, judge whether the final response converts it back to the form requested by the user.\n")
	b.WriteString("- If the user asks for a value in an already stated unit, scale, or format, prefer the candidate that returns the requested value without adding unrequested labels, units, explanations, or prose after the final answer.\n")
	b.WriteString("- If the user requests decimal precision, ordering, separators, item count, or a bare answer, treat those output-form constraints as part of correctness.\n")
	b.WriteString("- When two numeric final answers have the same core value, prefer the answer that is easiest to extract exactly in the requested format. A unit named in the question as context does not automatically make extra unit text required in the final answer.\n")
	b.WriteString("- For computational tasks, inspect the code and tool output for modeling errors. A candidate with unsupported or incorrect code should score lower even if the final explanation sounds plausible.\n")
	b.WriteString("- For retrieval or extraction tasks, prefer answers whose evidence trace directly locates the requested entity, span, count, or option. Penalize answers that ignore requested sources, positions, extrema, or constraints.\n")
	b.WriteString("- For list or geography questions, item identity, extrema such as westernmost or easternmost, temporal wording, and requested ordering are hard constraints. Do not prefer a more familiar modern or nearby entity if it does not match the requested condition.\n")
	b.WriteString("- For multi-hop requests, verify each required resolution step in order, including collection choice, sorting, counting, exact quoted-term matching, positional selection, source selection, and the final extraction. Penalize shortcuts that jump to a plausible item without evidence for the required path.\n")
	b.WriteString("- When candidates differ materially in final value, selected entity, item count, ordering, or answer form, assign meaningfully different score letters instead of tying them unless the visible traces make them genuinely indistinguishable.\n")
	b.WriteString("- Never use any information outside the visible request and candidate traces. Do not assume access to hidden answers, labels, scores, or task metadata.\n\n")
	b.WriteString("# Mandatory Evaluation Steps\n\n")
	b.WriteString("1. First identify the required final answer form from the user request: the quantity being asked for, the unit or scale, the rounding rule, and any formatting constraints.\n")
	b.WriteString("2. Identify all explicit constraints in the request, such as source restrictions, date ranges, positional rules, ordering rules, separator style, and requested number of items.\n")
	b.WriteString("3. Classify the support behind each candidate's final answer as primary source evidence, direct file or tool calculation, derived reasoning, secondary claim, or unsupported final-only answer.\n")
	b.WriteString("4. Mark any candidate evidence that comes from copied prompts, prior solutions, prior agent transcripts, run logs, evaluation files, or answer-only pages as contaminated secondary evidence.\n")
	b.WriteString("5. For multi-hop tasks, verify that every required hop is supported in order before accepting the final entity, value, count, or option.\n")
	b.WriteString("6. For requests involving quoted terms or title counts, verify exact term form and exact text scope before accepting a candidate's selected source or rule.\n")
	b.WriteString("7. Extract each candidate's final answer and identify the value or values most directly supported by that candidate's visible evidence trace.\n")
	b.WriteString("8. Compare each final answer against both the required answer form and the value supported by its own evidence before considering fluency or evidence volume.\n")
	b.WriteString("9. Separate what each candidate's evidence proves from any outside knowledge or assumptions you may have; unsupported plausibility is not evidence.\n")
	b.WriteString("10. For exact string or layout-sensitive tasks, check punctuation, hyphenation, apostrophes, capitalization, separators, item boundaries, and visible layout preservation before preferring an answer.\n")
	b.WriteString("11. For exact title or bibliographic strings, identify which candidate uses the most authoritative visible source for the exact string before treating variants as equivalent.\n")
	b.WriteString("12. For change or deletion questions, verify the selected item and then verify the source statement or diff that directly names the changed term.\n")
	b.WriteString("13. For quoted-term counts within groups or containers, require evidence comparing the relevant member titles rather than relying on the container's own title.\n")
	b.WriteString("14. If the user asks for a transformed or scaled quantity, the winning candidate must express the final answer in that requested transformed or scaled form.\n")
	b.WriteString("15. If candidates have the same numeric value but differ in unnecessary suffixes, labels, or prose, prefer the cleaner answer unless the request explicitly requires those extra tokens.\n")
	b.WriteString("16. If one candidate appears correct and another differs in value or required form, do not give them the same score merely because both are fluent or partially supported.\n\n")
	b.WriteString("# Score Scale\n\n")
	b.WriteString("Use exactly one of 20 score tokens from A to T for each candidate. A is best and T is worst.\n")
	b.WriteString("- A = clearly and completely satisfies the request under the evaluation rules.\n")
	b.WriteString("- B-D = satisfies the request with only minor issues.\n")
	b.WriteString("- E-G = mostly correct with some issues.\n")
	b.WriteString("- H-J = uncertain, leans toward success.\n")
	b.WriteString("- K-M = uncertain, leans toward failure.\n")
	b.WriteString("- N-P = significant issues remain.\n")
	b.WriteString("- Q-S = failed with some partial progress.\n")
	b.WriteString("- T = clearly and completely fails.\n\n")
	if strings.TrimSpace(criteria) != "" {
		b.WriteString("# Additional Evaluation Criteria\n\n")
		b.WriteString(criteria)
		b.WriteString("\n\n")
	}
	b.WriteString("# Output Format\n\n")
	b.WriteString("First write a concise analysis that states the required final answer form and then compares the candidates. Then output the final score tags exactly once:\n\n")
	b.WriteString("<score_A>LETTER_A_TO_T</score_A>\n")
	b.WriteString("<score_B>LETTER_A_TO_T</score_B>\n\n")
	b.WriteString("# User Request\n\n")
	b.WriteString(userInput)
	b.WriteString("\n\n# Candidate A\n\n")
	b.WriteString(candidateA)
	b.WriteString("\n\n# Candidate B\n\n")
	b.WriteString(candidateB)
	return b.String()
}

func formatCandidateForJudge(inv *evalset.Invocation) string {
	var b strings.Builder
	b.WriteString("## Final Response\n\n")
	final := messageText(inv.FinalResponse)
	if final == "" {
		final = "<empty>"
	}
	b.WriteString(final)
	b.WriteString("\n\n## Evidence Trace\n\n")
	trace := candidateTrace(inv)
	if trace == "" {
		trace = "<no intermediate messages or tools recorded>"
	}
	b.WriteString(trace)
	return b.String()
}

func candidateTrace(inv *evalset.Invocation) string {
	if inv == nil {
		return ""
	}
	var b strings.Builder
	for i, msg := range inv.IntermediateResponses {
		text := messageText(msg)
		if text == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("### Intermediate Message %d\n", i+1))
		b.WriteString("Role: ")
		b.WriteString(string(msg.Role))
		b.WriteString("\nContent:\n")
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	for i, tool := range inv.Tools {
		if tool == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("### Tool Call %d\n", i+1))
		b.WriteString("Name: ")
		b.WriteString(tool.Name)
		b.WriteString("\nArguments:\n")
		b.WriteString(formatJudgeValue(tool.Arguments))
		b.WriteString("\nResult:\n")
		b.WriteString(formatJudgeValue(tool.Result))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func rubricsText(evalMetric *metric.EvalMetric) string {
	if evalMetric == nil || evalMetric.Criterion == nil || evalMetric.Criterion.LLMJudge == nil {
		return ""
	}
	var parts []string
	for _, rubric := range evalMetric.Criterion.LLMJudge.Rubrics {
		if rubric == nil || rubric.Content == nil || strings.TrimSpace(rubric.Content.Text) == "" {
			continue
		}
		id := strings.TrimSpace(rubric.ID)
		text := strings.TrimSpace(rubric.Content.Text)
		if id == "" {
			parts = append(parts, "- "+text)
			continue
		}
		parts = append(parts, fmt.Sprintf("- %s: %s", id, text))
	}
	return strings.Join(parts, "\n")
}

func messageText(msg *model.Message) string {
	if msg == nil {
		return ""
	}
	if strings.TrimSpace(msg.Content) != "" {
		return strings.TrimSpace(msg.Content)
	}
	var parts []string
	for _, part := range msg.ContentParts {
		if part.Type != model.ContentTypeText || part.Text == nil {
			continue
		}
		if text := strings.TrimSpace(*part.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func formatJudgeValue(value any) string {
	if value == nil {
		return "<nil>"
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}
