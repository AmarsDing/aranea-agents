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
	"os"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion"
	criterionllm "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/llm"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/runner/bestofn"
)

const (
	defaultLLMVerifierAttempts       = 3
	defaultLLMVerifierJudgeModelName = "deepseek-v4-flash"
	defaultLLMVerifierJudgeSamples   = 1
	defaultLLMVerifierJudgeMaxTokens = 32768
)

// LLMVerifierConfig contains settings for the GAIA best-of-N verifier runner.
type LLMVerifierConfig struct {
	Attempts       int
	JudgeModelName string
	JudgeSamples   int
	JudgeMaxTokens int
}

// DefaultLLMVerifierConfig returns the default best-of-N verifier settings.
func DefaultLLMVerifierConfig() LLMVerifierConfig {
	return LLMVerifierConfig{
		Attempts:       defaultLLMVerifierAttempts,
		JudgeModelName: defaultLLMVerifierJudgeModelName,
		JudgeSamples:   defaultLLMVerifierJudgeSamples,
		JudgeMaxTokens: defaultLLMVerifierJudgeMaxTokens,
	}
}

func (cfg LLMVerifierConfig) withDefaults() LLMVerifierConfig {
	defaults := DefaultLLMVerifierConfig()
	if cfg.Attempts == 0 {
		cfg.Attempts = defaults.Attempts
	}
	if cfg.JudgeModelName == "" {
		cfg.JudgeModelName = defaults.JudgeModelName
	}
	if cfg.JudgeSamples == 0 {
		cfg.JudgeSamples = defaults.JudgeSamples
	}
	if cfg.JudgeMaxTokens == 0 {
		cfg.JudgeMaxTokens = defaults.JudgeMaxTokens
	}
	return cfg
}

// LLMVerifierRunnerFactory returns a best-of-N runner factory for GAIA.
func LLMVerifierRunnerFactory(_ Config, verifierCfg LLMVerifierConfig) RunnerFactory {
	verifierCfg = verifierCfg.withDefaults()
	return func(ag agent.Agent) (runner.Runner, error) {
		judgeRunner := runner.NewRunner(
			"gaia-llm-verifier-judge",
			newGAIAJudgeAgent(verifierCfg.JudgeModelName, verifierCfg.JudgeMaxTokens),
		)
		bestOfNOpt, err := bestofn.NewRunnerOption(
			bestofn.WithAttempts(verifierCfg.Attempts),
			bestofn.WithAttemptParallelEnabled(true),
			bestofn.WithAttemptParallelism(verifierCfg.Attempts),
			bestofn.WithSelectionMode(bestofn.SelectionModePairwise),
			bestofn.WithEvalMetrics(gaiaLLMVerifierMetric()),
			bestofn.WithRegistry(newGAIAVerifierRegistry()),
			bestofn.WithJudgeRunner(judgeRunner),
			bestofn.WithJudgeRunnerNumSamples(verifierCfg.JudgeSamples),
		)
		if err != nil {
			_ = judgeRunner.Close()
			return nil, err
		}
		return &runnerWithJudge{
			Runner: runner.NewRunner("gaia-runner", ag, bestOfNOpt),
			judge:  judgeRunner,
		}, nil
	}
}

type runnerWithJudge struct {
	runner.Runner
	judge runner.Runner
}

func (r *runnerWithJudge) Close() error {
	err := r.Runner.Close()
	if judgeErr := r.judge.Close(); err == nil {
		err = judgeErr
	}
	return err
}

func newGAIAJudgeAgent(modelName string, maxTokens int) agent.Agent {
	return llmagent.New("gaia-llm-verifier-judge-agent",
		llmagent.WithModel(openai.New(modelName, gaiaJudgeModelOptions()...)),
		llmagent.WithGenerationConfig(gaiaJudgeGenerationConfig(maxTokens)),
	)
}

func gaiaJudgeGenerationConfig(maxTokens int) model.GenerationConfig {
	logprobs := true
	topLogprobs := 20
	temperature := 0.0
	return model.GenerationConfig{
		MaxTokens:   intPtr(maxTokens),
		Temperature: &temperature,
		Logprobs:    &logprobs,
		TopLogprobs: &topLogprobs,
		Stream:      false,
	}
}

func gaiaJudgeModelOptions() []openai.Option {
	opts := make([]openai.Option, 0, 2)
	if apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); apiKey != "" {
		opts = append(opts, openai.WithAPIKey(apiKey))
	}
	if baseURL := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")); baseURL != "" {
		opts = append(opts, openai.WithBaseURL(baseURL))
	}
	return opts
}

func gaiaLLMVerifierMetric() *metric.EvalMetric {
	return &metric.EvalMetric{
		MetricName: "llm_verifier_pairwise",
		Threshold:  0.5,
		Criterion: &criterion.Criterion{
			LLMJudge: &criterionllm.LLMCriterion{
				Rubrics: []*criterionllm.Rubric{
					{
						ID: "answer_form_alignment",
						Content: &criterionllm.RubricContent{
							Text: "Prefer the candidate whose final answer matches the form requested by the user, including the requested unit, scale, rounding, ordering, separator style, item count, and whether the answer should be a bare value or include labels.",
						},
					},
					{
						ID: "evidence_grounding",
						Content: &criterionllm.RubricContent{
							Text: "Prefer the candidate whose final answer is directly supported by the visible tool outputs, retrieved source text, calculations, or file contents, especially when the user asks about a specific source, document, image, audio file, rule, or official record.",
						},
					},
					{
						ID: "evidence_over_prior_knowledge",
						Content: &criterionllm.RubricContent{
							Text: "Do not let the judge's own background knowledge or plausibility assumptions override the candidate trace. If a candidate's trace does not prove the selected entity, word, string, count, or value, treat that answer as unsupported even when it sounds familiar.",
						},
					},
					{
						ID: "source_quality",
						Content: &criterionllm.RubricContent{
							Text: "Prefer primary sources, direct file contents, direct tool observations, and candidate-owned calculations over answer-key pages, benchmark datasets, previous agent traces, solution writeups, or pages that merely restate the same question and answer.",
						},
					},
					{
						ID: "contaminated_evidence_penalty",
						Content: &criterionllm.RubricContent{
							Text: "Penalize candidates whose support comes from retrieved copies of the same user request, prior agent transcripts, run logs, scenario files, evaluation files, or prior final-answer markers instead of evidence from the underlying source requested by the user.",
						},
					},
					{
						ID: "trace_sufficiency",
						Content: &criterionllm.RubricContent{
							Text: "For nontrivial retrieval, computation, document, media, or file tasks, penalize final-only candidates with no evidence trace when another candidate has visible evidence that directly supports a competing final answer.",
						},
					},
					{
						ID: "final_answer_fidelity",
						Content: &criterionllm.RubricContent{
							Text: "Prefer the candidate whose final answer preserves the exact value supported by its own evidence trace. When a trace clearly establishes a value, prefer the candidate that carries that value into the final answer without substituting a nearby, more common, or more fluent-looking answer.",
						},
					},
					{
						ID: "constraint_completion",
						Content: &criterionllm.RubricContent{
							Text: "Prefer the candidate that satisfies all explicit constraints in the request, including source restrictions, date ranges, alphabetic or positional selection rules, formatting constraints, and requested number of items.",
						},
					},
					{
						ID: "quoted_term_scope",
						Content: &criterionllm.RubricContent{
							Text: "When the request quotes a word or phrase or asks for counts within titles, headings, table cells, captions, or body text, prefer candidates that preserve the exact term form and count only within the requested text scope.",
						},
					},
					{
						ID: "exact_string_fidelity",
						Content: &criterionllm.RubricContent{
							Text: "For exact extraction requests such as complete titles, names, quoted words, source-specific text, or required answer formats, compare final answers at the requested string granularity, including source-visible punctuation, hyphenation, apostrophes, capitalization, separators, and historical or time-specific names.",
						},
					},
					{
						ID: "exact_source_priority",
						Content: &criterionllm.RubricContent{
							Text: "When visible sources conflict on an exact title, name, or bibliographic string, prefer title-page text, catalog metadata, publisher or official metadata, or another source that explicitly identifies the exact string over prose mentions, search snippets, or typography-normalized display text.",
						},
					},
					{
						ID: "change_evidence_directness",
						Content: &criterionllm.RubricContent{
							Text: "For amendment, change, addition, or deletion questions, prefer candidates whose trace directly identifies the changed term in the selected item through an amendment note, changelog, version diff, or source statement. Penalize candidates that infer the requested change from restyled current text without evidence for that exact amendment.",
						},
					},
					{
						ID: "quoted_collection_scope",
						Content: &criterionllm.RubricContent{
							Text: "For quoted-term counting across a collection, count only comparable items in the requested collection. If the request asks which group, section, article, category, or container has a quoted term in the most member titles, require evidence comparing those member titles and do not rely on the parent or container title itself.",
						},
					},
					{
						ID: "layout_evidence_visibility",
						Content: &criterionllm.RubricContent{
							Text: "For layout-sensitive requests such as indentation, table position, visual grouping, or source formatting, require evidence that visibly preserves the requested layout. Do not infer layout from plain text mirrors, snippets, short lines, or line breaks that may have lost formatting.",
						},
					},
					{
						ID: "multi_hop_resolution",
						Content: &criterionllm.RubricContent{
							Text: "For nested or multi-hop requests, prefer the candidate that shows evidence for each required resolution step, including collection choice, sorting, counting, exact quoted-term matching, positional selection, source selection, and final extraction.",
						},
					},
					{
						ID: "concise_extractable_answer",
						Content: &criterionllm.RubricContent{
							Text: "Prefer a concise final answer that is easy to extract when it preserves the requested answer form and the evidence-supported value.",
						},
					},
					{
						ID: "output_form_penalties",
						Content: &criterionllm.RubricContent{
							Text: "Penalize candidates that add unrequested units, labels, narrative text, or alternative values to the final answer when the request asks for a bare value, a stated unit's value, a fixed precision, or a specific list format.",
						},
					},
					{
						ID: "numeric_extractability_tiebreak",
						Content: &criterionllm.RubricContent{
							Text: "When candidates have the same core numeric value, prefer the candidate whose final answer is easiest to extract exactly in the requested format. A unit or scale named in the question as context does not automatically make extra unit text required in the final answer.",
						},
					},
					{
						ID: "entity_list_identity",
						Content: &criterionllm.RubricContent{
							Text: "For list and geography questions, prefer the candidate whose listed entities satisfy the requested identity, extrema, temporal wording, item count, and ordering. Do not prefer a familiar modern or nearby entity over the entity that matches the requested condition.",
						},
					},
					{
						ID: "meaningful_pairwise_separation",
						Content: &criterionllm.RubricContent{
							Text: "When the candidates differ in final value, selected entity, count, ordering, location, or formatting constraint satisfaction, give the better-supported candidate a clearly better score instead of treating the pair as a tie.",
						},
					},
				},
			},
		},
	}
}

var _ runner.Runner = (*runnerWithJudge)(nil)

func (r *runnerWithJudge) Run(
	ctx context.Context,
	userID string,
	sessionID string,
	message model.Message,
	runOpts ...agent.RunOption,
) (<-chan *event.Event, error) {
	return r.Runner.Run(ctx, userID, sessionID, message, runOpts...)
}
