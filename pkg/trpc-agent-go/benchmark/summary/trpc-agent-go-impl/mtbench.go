//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-agent-go-benchmark/summary/trpc-agent-go-impl/evaluation/dataset"
	"trpc.group/trpc-go/trpc-agent-go-benchmark/summary/trpc-agent-go-impl/evaluation/evaluator"
	"trpc.group/trpc-go/trpc-agent-go-benchmark/summary/trpc-agent-go-impl/evaluation/evaluator/comparator"
	"trpc.group/trpc-go/trpc-agent-go-benchmark/summary/trpc-agent-go-impl/evaluation/evaluator/passhatk"
	"trpc.group/trpc-go/trpc-agent-go-benchmark/summary/trpc-agent-go-impl/evaluation/evaluator/retention"
	evalsummary "trpc.group/trpc-go/trpc-agent-go-benchmark/summary/trpc-agent-go-impl/evaluation/evaluator/summary"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	sessionsummary "trpc.group/trpc-go/trpc-agent-go/session/summary"
)

type mtBenchRunMode string

const (
	mtBenchModeBaseline mtBenchRunMode = "baseline"
	mtBenchModeSummary  mtBenchRunMode = "summary"
)

const (
	weightConsistency = 0.50
	weightTokens      = 0.30
	weightRetention   = 0.20
)

var mtBench101TaskCodeSet = map[string]struct{}{
	"AR": {},
	"CC": {},
	"CM": {},
	"CR": {},
	"FR": {},
	"GR": {},
	"IC": {},
	"MR": {},
	"PI": {},
	"SA": {},
	"SC": {},
	"SI": {},
	"TS": {},
}

type MTBenchCaseResult struct {
	CaseID string `json:"caseId"`

	TokenEfficiency *MTBenchTokenEfficiency `json:"tokenEfficiency"`
	Consistency     *MTBenchConsistency     `json:"consistency"`
	Retention       *MTBenchRetention       `json:"retention"`

	BaselineRuns []*MTBenchRunResult `json:"baselineRuns,omitempty"`
	SummaryRuns  []*MTBenchRunResult `json:"summaryRuns,omitempty"`
}

type MTBenchTokenEfficiency struct {
	BaselineTokens int `json:"baselineTokens"`
	SummaryTokens  int `json:"summaryTokens"`
	TokensSaved    int `json:"tokensSaved"`

	BaselinePromptTokens int `json:"baselinePromptTokens"`
	SummaryPromptTokens  int `json:"summaryPromptTokens"`
	PromptTokensSaved    int `json:"promptTokensSaved"`

	BaselineCompletionTokens int `json:"baselineCompletionTokens"`
	SummaryCompletionTokens  int `json:"summaryCompletionTokens"`

	BaselineLastPrompt int `json:"baselineLastPrompt"`
	SummaryLastPrompt  int `json:"summaryLastPrompt"`

	SavingsPercentage       float64 `json:"savingsPercentage"`
	PromptSavingsPercentage float64 `json:"promptSavingsPercentage"`
	CompressionRatio        float64 `json:"compressionRatio"`
}

type MTBenchConsistency struct {
	Score            float64        `json:"score"`
	PassHat1         float64        `json:"passHat1"`
	PassHat2         float64        `json:"passHat2,omitempty"`
	PassHat4         float64        `json:"passHat4,omitempty"`
	SuccessCount     int            `json:"successCount"`
	TotalRuns        int            `json:"totalRuns"`
	Variance         float64        `json:"variance"`
	ConsistencyLevel string         `json:"consistencyLevel"`
	Details          map[string]any `json:"details,omitempty"`
}

type MTBenchRetention struct {
	RetentionRate float64   `json:"retentionRate"`
	KeyInfoCount  int       `json:"keyInfoCount"`
	RetainedCount int       `json:"retainedCount"`
	MissingInfo   []string  `json:"missingInfo,omitempty"`
	PerTurn       []float64 `json:"perTurn,omitempty"`
	PerRun        []float64 `json:"perRun,omitempty"`
}

type MTBenchRunResult struct {
	Mode        mtBenchRunMode        `json:"mode"`
	Invocations []*evalset.Invocation `json:"invocations"`

	TokenUsagePerTurn []*TokenUsage `json:"tokenUsagePerTurn"`

	TotalTokens      int `json:"totalTokens"`
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`

	Duration time.Duration `json:"duration"`
}

type MTBenchResults struct {
	Timestamp   string               `json:"timestamp"`
	Model       string               `json:"model"`
	NumCases    int                  `json:"numCases"`
	NumRuns     int                  `json:"numRuns"`
	CaseResults []*MTBenchCaseResult `json:"caseResults"`

	AvgTokenSavings  float64 `json:"avgTokenSavings"`
	AvgPromptSavings float64 `json:"avgPromptSavings"`
	AvgConsistency   float64 `json:"avgConsistency"`
	AvgRetention     float64 `json:"avgRetention"`
	OverallScore     float64 `json:"overallScore"`
}

type MTBenchBenchmark struct {
	cfg       *appConfig
	evaluator *mtBenchEvaluator
}

type mtBenchEvaluator struct {
	cfg        *appConfig
	llm        model.Model
	passHatK   *passhatk.PassHatKEvaluator
	retention  *retention.RetentionEvaluator
	comparator comparator.ConversationComparator
}

func newMTBenchBenchmark(cfg *appConfig) *MTBenchBenchmark {
	return &MTBenchBenchmark{
		cfg:       cfg,
		evaluator: newMTBenchEvaluator(cfg),
	}
}

func newMTBenchEvaluator(cfg *appConfig) *mtBenchEvaluator {
	llm := openai.New(cfg.ModelName)

	var conv comparator.ConversationComparator
	if cfg.UseLLMEval {
		conv = evalsummary.NewSummaryComparator(llm, cfg.MTBench.ConsistencyThreshold)
	} else {
		conv = evalsummary.NewSummaryComparator(nil, cfg.MTBench.ConsistencyThreshold)
	}

	passHatK := passhatk.New(conv, passhatk.WithKValues(cfg.MTBench.KValues))

	var retEval *retention.RetentionEvaluator
	if cfg.UseLLMEval {
		retEval = retention.New(llm, retention.WithThreshold(cfg.MTBench.RetentionThreshold))
	} else {
		retEval = retention.New(nil, retention.WithThreshold(cfg.MTBench.RetentionThreshold))
	}

	return &mtBenchEvaluator{
		cfg:        cfg,
		llm:        llm,
		passHatK:   passHatK,
		retention:  retEval,
		comparator: conv,
	}
}

func (b *MTBenchBenchmark) Run(ctx context.Context) error {
	log.Printf("=== Summary Evaluation (τ-bench inspired) ===")
	log.Printf("Model: %s", b.cfg.ModelName)
	log.Printf("Dataset: %s", b.cfg.DatasetPath)
	log.Printf("Output: %s", b.cfg.OutputDir)
	log.Printf("Event Threshold: %d", b.cfg.Events)
	log.Printf("Runs per mode: %d", b.cfg.MTBench.NumRuns)
	log.Printf("LLM Evaluation: %v", b.cfg.UseLLMEval)
	log.Printf("Resume: %v", b.cfg.Resume)
	log.Printf("Consistency Threshold: %.2f", b.cfg.MTBench.ConsistencyThreshold)
	log.Printf("Retention Threshold: %.2f", b.cfg.MTBench.RetentionThreshold)
	log.Printf("K Values: %v", b.cfg.MTBench.KValues)
	log.Printf("Weights: Consistency %.0f%%, Tokens %.0f%%, Retention %.0f%%",
		weightConsistency*100, weightTokens*100, weightRetention*100)

	if b.cfg.MTBench.TaskFilterRaw != "" {
		log.Printf("Filtering MT-Bench-101 by task: %s", b.cfg.MTBench.TaskFilterRaw)
	}

	cases, err := loadMTBenchEvalCases(
		b.cfg.DatasetPath,
		b.cfg.NumCases,
		b.cfg.MTBench.TaskFilters,
	)
	if err != nil {
		return fmt.Errorf("load MT-Bench-101 cases: %w", err)
	}
	log.Printf("Loaded %d test cases", len(cases))

	results := &MTBenchResults{
		Timestamp:   time.Now().Format(time.RFC3339),
		Model:       b.cfg.ModelName,
		NumCases:    len(cases),
		NumRuns:     b.cfg.MTBench.NumRuns,
		CaseResults: make([]*MTBenchCaseResult, 0, len(cases)),
	}

	completedCases := make(map[string]bool)
	if b.cfg.Resume {
		if checkpoint := loadMTBenchCheckpoint(b.cfg.OutputDir); checkpoint != nil {
			results.CaseResults = checkpoint.CaseResults
			for _, cr := range checkpoint.CaseResults {
				completedCases[cr.CaseID] = true
			}
			log.Printf("Resumed from checkpoint: %d cases completed", len(completedCases))
		}
	}

	startTime := time.Now()
	for i, tc := range cases {
		if completedCases[tc.EvalID] {
			log.Printf("[%d/%d] Case: %s - SKIPPED (already completed)", i+1, len(cases), tc.EvalID)
			continue
		}

		caseStart := time.Now()
		log.Printf("")
		log.Printf("[%d/%d] Case: %s (%d turns)", i+1, len(cases), tc.EvalID, len(tc.Conversation))

		caseResult, err := b.evaluator.evaluateCase(ctx, tc)
		if err != nil {
			log.Printf("  Error: %v", err)
			continue
		}

		results.CaseResults = append(results.CaseResults, caseResult)
		saveMTBenchCaseLog(b.cfg.OutputDir, b.cfg.ModelName, caseResult)
		saveMTBenchCheckpoint(b.cfg.OutputDir, results)

		log.Printf("  Duration: %v", time.Since(caseStart).Round(time.Millisecond))
		log.Printf("  Tokens (total): %d -> %d (%.1f%% saved)",
			caseResult.TokenEfficiency.BaselineTokens,
			caseResult.TokenEfficiency.SummaryTokens,
			caseResult.TokenEfficiency.SavingsPercentage)
		log.Printf("  Tokens (prompt): %d -> %d (%.1f%% saved)",
			caseResult.TokenEfficiency.BaselinePromptTokens,
			caseResult.TokenEfficiency.SummaryPromptTokens,
			caseResult.TokenEfficiency.PromptSavingsPercentage)
		log.Printf("  Last turn prompt: %d -> %d",
			caseResult.TokenEfficiency.BaselineLastPrompt,
			caseResult.TokenEfficiency.SummaryLastPrompt)
		log.Printf("  Consistency: %.2f (Pass^1=%.2f, %d/%d passed)",
			caseResult.Consistency.Score,
			caseResult.Consistency.PassHat1,
			caseResult.Consistency.SuccessCount,
			caseResult.Consistency.TotalRuns)
		log.Printf("  Retention: %.2f (%d/%d key info)",
			caseResult.Retention.RetentionRate,
			caseResult.Retention.RetainedCount,
			caseResult.Retention.KeyInfoCount)

		elapsed := time.Since(startTime)
		avgPerCase := elapsed / time.Duration(i+1)
		remaining := avgPerCase * time.Duration(len(cases)-i-1)
		log.Printf("  Progress: %d/%d | Elapsed: %v | ETA: %v",
			i+1, len(cases), elapsed.Round(time.Second), remaining.Round(time.Second))
	}

	aggregateMTBenchResults(results)
	printMTBenchResults(results)
	saveMTBenchResults(b.cfg.OutputDir, results)
	return nil
}

func (e *mtBenchEvaluator) evaluateCase(
	ctx context.Context,
	evalCase *evalset.EvalCase,
) (*MTBenchCaseResult, error) {
	log.Printf("  Running baseline mode...")
	baselineRuns, err := e.runMultiple(ctx, evalCase, mtBenchModeBaseline)
	if err != nil {
		return nil, fmt.Errorf("baseline runs failed: %w", err)
	}

	log.Printf("  Running summary mode...")
	summaryRuns, err := e.runMultiple(ctx, evalCase, mtBenchModeSummary)
	if err != nil {
		return nil, fmt.Errorf("summary runs failed: %w", err)
	}

	log.Printf("  Evaluating...")
	tokenEff := e.evaluateTokenEfficiency(baselineRuns, summaryRuns)
	consistency, err := e.evaluateConsistency(ctx, baselineRuns, summaryRuns)
	if err != nil {
		return nil, fmt.Errorf("consistency evaluation failed: %w", err)
	}
	retentionResult, err := e.evaluateRetention(ctx, baselineRuns, summaryRuns)
	if err != nil {
		return nil, fmt.Errorf("retention evaluation failed: %w", err)
	}

	return &MTBenchCaseResult{
		CaseID:          evalCase.EvalID,
		TokenEfficiency: tokenEff,
		Consistency:     consistency,
		Retention:       retentionResult,
		BaselineRuns:    baselineRuns,
		SummaryRuns:     summaryRuns,
	}, nil
}

func (e *mtBenchEvaluator) runMultiple(
	ctx context.Context,
	evalCase *evalset.EvalCase,
	mode mtBenchRunMode,
) ([]*MTBenchRunResult, error) {
	results := make([]*MTBenchRunResult, 0, e.cfg.MTBench.NumRuns)
	for i := 0; i < e.cfg.MTBench.NumRuns; i++ {
		if e.cfg.MTBench.NumRuns > 1 {
			log.Printf("    [%s] run %d/%d...", mode, i+1, e.cfg.MTBench.NumRuns)
		}
		result, err := e.runOnce(ctx, evalCase, mode, i)
		if err != nil {
			return nil, fmt.Errorf("run %d failed: %w", i, err)
		}
		if e.cfg.MTBench.NumRuns > 1 {
			log.Printf("    [%s] run %d/%d: %d tokens, %v",
				mode, i+1, e.cfg.MTBench.NumRuns, result.TotalTokens, result.Duration.Round(time.Millisecond))
		}
		results = append(results, result)
	}
	return results, nil
}

func (e *mtBenchEvaluator) runOnce(
	ctx context.Context,
	evalCase *evalset.EvalCase,
	mode mtBenchRunMode,
	runIndex int,
) (*MTBenchRunResult, error) {
	start := time.Now()

	var sessService *inmemory.SessionService
	withSummary := mode == mtBenchModeSummary
	if withSummary {
		sum := sessionsummary.NewSummarizer(e.llm, summaryOptions(e.cfg)...)
		sessService = inmemory.NewSessionService(
			inmemory.WithSummarizer(sum),
			inmemory.WithAsyncSummaryNum(1),
			inmemory.WithSummaryQueueSize(10),
			inmemory.WithSummaryJobTimeout(30*time.Second),
		)
	} else {
		sessService = inmemory.NewSessionService()
	}

	agOpts := []llmagent.Option{
		llmagent.WithModel(e.llm),
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream:    false,
			MaxTokens: intPtr(2000),
		}),
		llmagent.WithAddSessionSummary(withSummary),
	}
	if withSummary {
		agOpts = append(agOpts,
			llmagent.WithMessageBranchFilterMode(llmagent.BranchFilterModeAll),
		)
	}
	ag := llmagent.New(
		"eval-agent",
		agOpts...,
	)

	r := runner.NewRunner("eval-app", ag, runner.WithSessionService(sessService))
	defer r.Close()

	userID := "eval-user"
	sessionID := fmt.Sprintf(
		"session-%s-%s-%d-%d",
		evalCase.EvalID,
		mode,
		runIndex,
		time.Now().UnixNano(),
	)

	invocations := make([]*evalset.Invocation, 0, len(evalCase.Conversation))
	tokenUsagePerTurn := make([]*TokenUsage, 0, len(evalCase.Conversation))
	var totalTokens, promptTokens, completionTokens int

	for i, origInv := range evalCase.Conversation {
		if origInv.UserContent == nil {
			continue
		}

		userMsg := origInv.UserContent.Content
		if e.cfg.Verbose {
			log.Printf("      Turn %d [User]: %s", i+1, truncateStr(userMsg, 200))
		} else {
			log.Printf("      Turn %d: sending message (%d chars)...", i+1, len(userMsg))
		}

		evtCh, err := r.Run(ctx, userID, sessionID, model.NewUserMessage(userMsg))
		if err != nil {
			return nil, fmt.Errorf("turn %d failed: %w", i+1, err)
		}

		response, usage := consumeEvents(evtCh)
		tokenUsagePerTurn = append(tokenUsagePerTurn, usage)
		totalTokens += usage.TotalTokens
		promptTokens += usage.PromptTokens
		completionTokens += usage.CompletionTokens

		if e.cfg.Verbose {
			log.Printf("      Turn %d [Assistant]: %s (p=%d, c=%d, t=%d)",
				i+1, truncateStr(response, 200),
				usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
		} else {
			log.Printf("      Turn %d: received response (%d chars, p=%d c=%d t=%d)",
				i+1, len(response), usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
		}

		invocations = append(invocations, &evalset.Invocation{
			InvocationID: fmt.Sprintf("%d", i+1),
			UserContent:  origInv.UserContent,
			FinalResponse: &model.Message{
				Role:    model.RoleAssistant,
				Content: response,
			},
		})
	}

	return &MTBenchRunResult{
		Mode:              mode,
		Invocations:       invocations,
		TokenUsagePerTurn: tokenUsagePerTurn,
		TotalTokens:       totalTokens,
		PromptTokens:      promptTokens,
		CompletionTokens:  completionTokens,
		Duration:          time.Since(start),
	}, nil
}

func (e *mtBenchEvaluator) evaluateTokenEfficiency(
	baselineRuns, summaryRuns []*MTBenchRunResult,
) *MTBenchTokenEfficiency {
	var baselineTotal, summaryTotal int
	var baselinePrompt, summaryPrompt int
	var baselineCompletion, summaryCompletion int
	var baselineLastPrompt, summaryLastPrompt int

	for _, r := range baselineRuns {
		baselineTotal += r.TotalTokens
		baselinePrompt += r.PromptTokens
		baselineCompletion += r.CompletionTokens
		if len(r.TokenUsagePerTurn) > 0 {
			baselineLastPrompt += r.TokenUsagePerTurn[len(r.TokenUsagePerTurn)-1].PromptTokens
		}
	}
	for _, r := range summaryRuns {
		summaryTotal += r.TotalTokens
		summaryPrompt += r.PromptTokens
		summaryCompletion += r.CompletionTokens
		if len(r.TokenUsagePerTurn) > 0 {
			summaryLastPrompt += r.TokenUsagePerTurn[len(r.TokenUsagePerTurn)-1].PromptTokens
		}
	}

	n := len(baselineRuns)
	baselineAvg := baselineTotal / n
	summaryAvg := summaryTotal / n
	baselinePromptAvg := baselinePrompt / n
	summaryPromptAvg := summaryPrompt / n
	baselineCompletionAvg := baselineCompletion / n
	summaryCompletionAvg := summaryCompletion / n
	baselineLastPromptAvg := baselineLastPrompt / n
	summaryLastPromptAvg := summaryLastPrompt / n

	tokensSaved := baselineAvg - summaryAvg
	promptTokensSaved := baselinePromptAvg - summaryPromptAvg

	var savingsPercentage, promptSavingsPercentage, compressionRatio float64
	if baselineAvg > 0 {
		savingsPercentage = float64(tokensSaved) / float64(baselineAvg) * 100
		if summaryAvg > 0 {
			compressionRatio = float64(baselineAvg) / float64(summaryAvg)
		}
	}
	if baselinePromptAvg > 0 {
		promptSavingsPercentage = float64(promptTokensSaved) / float64(baselinePromptAvg) * 100
	}

	return &MTBenchTokenEfficiency{
		BaselineTokens:           baselineAvg,
		SummaryTokens:            summaryAvg,
		TokensSaved:              tokensSaved,
		BaselinePromptTokens:     baselinePromptAvg,
		SummaryPromptTokens:      summaryPromptAvg,
		PromptTokensSaved:        promptTokensSaved,
		BaselineCompletionTokens: baselineCompletionAvg,
		SummaryCompletionTokens:  summaryCompletionAvg,
		BaselineLastPrompt:       baselineLastPromptAvg,
		SummaryLastPrompt:        summaryLastPromptAvg,
		SavingsPercentage:        savingsPercentage,
		PromptSavingsPercentage:  promptSavingsPercentage,
		CompressionRatio:         compressionRatio,
	}
}

func (e *mtBenchEvaluator) evaluateConsistency(
	ctx context.Context,
	baselineRuns, summaryRuns []*MTBenchRunResult,
) (*MTBenchConsistency, error) {
	baselineInvs := make([][]*evalset.Invocation, len(baselineRuns))
	for i, r := range baselineRuns {
		baselineInvs[i] = r.Invocations
	}
	summaryInvs := make([][]*evalset.Invocation, len(summaryRuns))
	for i, r := range summaryRuns {
		summaryInvs[i] = r.Invocations
	}

	result, err := e.passHatK.EvaluateMultiRun(ctx, baselineInvs, summaryInvs, nil)
	if err != nil {
		return nil, err
	}

	var passHat1, passHat2, passHat4, variance float64
	var successCount, totalRuns int
	if details := result.Details; details != nil {
		passHat1 = asFloat64(details["pass_hat_1"])
		passHat2 = asFloat64(details["pass_hat_2"])
		passHat4 = asFloat64(details["pass_hat_4"])
		variance = asFloat64(details["variance"])
		successCount = asInt(details["success_count"])
		totalRuns = asInt(details["total_runs"])
	}

	level := "low"
	if result.OverallScore >= 0.9 {
		level = "high"
	} else if result.OverallScore >= 0.7 {
		level = "medium"
	}

	return &MTBenchConsistency{
		Score:            result.OverallScore,
		PassHat1:         passHat1,
		PassHat2:         passHat2,
		PassHat4:         passHat4,
		SuccessCount:     successCount,
		TotalRuns:        totalRuns,
		Variance:         variance,
		ConsistencyLevel: level,
		Details:          result.Details,
	}, nil
}

func (e *mtBenchEvaluator) evaluateRetention(
	ctx context.Context,
	baselineRuns, summaryRuns []*MTBenchRunResult,
) (*MTBenchRetention, error) {
	baselineInvs := make([][]*evalset.Invocation, len(baselineRuns))
	for i, r := range baselineRuns {
		baselineInvs[i] = r.Invocations
	}
	summaryInvs := make([][]*evalset.Invocation, len(summaryRuns))
	for i, r := range summaryRuns {
		summaryInvs[i] = r.Invocations
	}

	if len(baselineInvs) == 1 && len(summaryInvs) == 1 {
		result, err := e.retention.Evaluate(ctx, summaryInvs[0], baselineInvs[0], nil)
		if err != nil {
			return nil, err
		}
		return parseMTBenchRetention(result), nil
	}

	result, err := e.retention.EvaluateMultiRun(ctx, baselineInvs, summaryInvs, nil)
	if err != nil {
		return nil, err
	}
	return parseMTBenchRetention(result), nil
}

func loadMTBenchEvalCases(
	datasetPath string,
	numCases int,
	taskFilters []string,
) ([]*evalset.EvalCase, error) {
	datasetPath = filepath.Clean(datasetPath)

	info, err := os.Stat(datasetPath)
	if err != nil {
		return nil, fmt.Errorf("dataset path does not exist: %w", err)
	}

	var (
		loader   *dataset.DatasetLoader
		filename string
	)
	if info.Mode().IsRegular() {
		loader = dataset.NewDatasetLoader(filepath.Dir(datasetPath))
		filename = filepath.Base(datasetPath)
	} else {
		mtBenchPath := filepath.Join(datasetPath, "subjective", "mtbench101.jsonl")
		if _, err := os.Stat(mtBenchPath); err != nil {
			return nil, fmt.Errorf("MT-Bench-101 file not found: %s", mtBenchPath)
		}
		loader = dataset.NewDatasetLoader(filepath.Dir(datasetPath))
		filename = filepath.Base(datasetPath) + "/subjective/mtbench101.jsonl"
	}

	log.Printf("Loading MT-Bench-101 dataset...")
	entries, err := loader.LoadMTBench101(filename, taskFilters...)
	if err != nil {
		return nil, fmt.Errorf("load MT-Bench-101: %w", err)
	}
	if len(entries) == 0 {
		if len(taskFilters) > 0 {
			return nil, fmt.Errorf(
				"no MT-Bench-101 entries matched task filter: %s, valid values: %s",
				strings.Join(taskFilters, ","),
				strings.Join(validMTBench101TaskCodes(), ","),
			)
		}
		return nil, fmt.Errorf("no MT-Bench-101 entries found")
	}

	cases := dataset.ConvertMTBench101ToEvalCases(entries)
	if numCases > 0 && numCases < len(cases) {
		cases = cases[:numCases]
	}
	log.Printf("Loaded %d cases from MT-Bench-101 (total: %d)", len(cases), len(entries))
	return cases, nil
}

func aggregateMTBenchResults(results *MTBenchResults) {
	if len(results.CaseResults) == 0 {
		return
	}

	var totalSavings, promptSavings, totalConsistency, totalRetention float64
	for _, cr := range results.CaseResults {
		totalSavings += cr.TokenEfficiency.SavingsPercentage
		promptSavings += cr.TokenEfficiency.PromptSavingsPercentage
		totalConsistency += cr.Consistency.Score
		totalRetention += cr.Retention.RetentionRate
	}

	n := float64(len(results.CaseResults))
	results.AvgTokenSavings = totalSavings / n
	results.AvgPromptSavings = promptSavings / n
	results.AvgConsistency = totalConsistency / n
	results.AvgRetention = totalRetention / n

	tokenScore := results.AvgTokenSavings / 100
	if tokenScore > 1 {
		tokenScore = 1
	}
	if tokenScore < 0 {
		tokenScore = 0
	}

	results.OverallScore = weightConsistency*results.AvgConsistency +
		weightTokens*tokenScore +
		weightRetention*results.AvgRetention
}

func printMTBenchResults(results *MTBenchResults) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Summary Evaluation Results")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("\nModel: %s\n", results.Model)
	fmt.Printf("Cases: %d, Runs per mode: %d\n", results.NumCases, results.NumRuns)

	fmt.Println("\n--- Token Efficiency (30%) ---")
	fmt.Printf("Average Total Savings: %.1f%%\n", results.AvgTokenSavings)
	fmt.Printf("Average Prompt Savings: %.1f%%\n", results.AvgPromptSavings)

	fmt.Println("\n--- Response Consistency (50%) - Pass^k ---")
	fmt.Printf("Average Score: %.3f\n", results.AvgConsistency)

	fmt.Println("\n--- Information Retention (20%) ---")
	fmt.Printf("Average Retention: %.3f\n", results.AvgRetention)

	fmt.Println("\n--- Overall Score ---")
	fmt.Printf("%.3f (Consistency: %.0f%%, Tokens: %.0f%%, Retention: %.0f%%)\n",
		results.OverallScore,
		weightConsistency*100, weightTokens*100, weightRetention*100)

	fmt.Println("\n--- Per-Case Summary ---")
	for _, cr := range results.CaseResults {
		fmt.Printf("  %s: total=%.0f%%, prompt=%.0f%%, consistency=%.2f, retention=%.2f\n",
			cr.CaseID,
			cr.TokenEfficiency.SavingsPercentage,
			cr.TokenEfficiency.PromptSavingsPercentage,
			cr.Consistency.Score,
			cr.Retention.RetentionRate)
	}
	fmt.Println(strings.Repeat("=", 60))
}

func saveMTBenchCaseLog(outputDir, modelName string, cr *MTBenchCaseResult) {
	logPath := filepath.Join(outputDir, cr.CaseID+".log")
	f, err := os.Create(logPath)
	if err != nil {
		log.Printf("Failed to create log: %v", err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "=== Evaluation Log: %s ===\n", cr.CaseID)
	fmt.Fprintf(f, "Timestamp: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "Model: %s\n\n", modelName)

	fmt.Fprintf(f, "--- TOKEN EFFICIENCY ---\n")
	fmt.Fprintf(f, "Total Tokens:\n")
	fmt.Fprintf(f, "  Baseline: %d tokens\n", cr.TokenEfficiency.BaselineTokens)
	fmt.Fprintf(f, "  Summary:  %d tokens\n", cr.TokenEfficiency.SummaryTokens)
	fmt.Fprintf(f, "  Saved:    %d tokens (%.1f%%)\n", cr.TokenEfficiency.TokensSaved, cr.TokenEfficiency.SavingsPercentage)
	fmt.Fprintf(f, "Prompt Tokens:\n")
	fmt.Fprintf(f, "  Baseline: %d tokens\n", cr.TokenEfficiency.BaselinePromptTokens)
	fmt.Fprintf(f, "  Summary:  %d tokens\n", cr.TokenEfficiency.SummaryPromptTokens)
	fmt.Fprintf(f, "  Saved:    %d tokens (%.1f%%)\n", cr.TokenEfficiency.PromptTokensSaved, cr.TokenEfficiency.PromptSavingsPercentage)
	fmt.Fprintf(f, "Completion Tokens:\n")
	fmt.Fprintf(f, "  Baseline: %d tokens\n", cr.TokenEfficiency.BaselineCompletionTokens)
	fmt.Fprintf(f, "  Summary:  %d tokens\n", cr.TokenEfficiency.SummaryCompletionTokens)
	fmt.Fprintf(f, "Last Turn Prompt (most relevant for summary):\n")
	fmt.Fprintf(f, "  Baseline: %d tokens\n", cr.TokenEfficiency.BaselineLastPrompt)
	fmt.Fprintf(f, "  Summary:  %d tokens\n", cr.TokenEfficiency.SummaryLastPrompt)
	fmt.Fprintf(f, "Compression: %.2fx\n\n", cr.TokenEfficiency.CompressionRatio)

	fmt.Fprintf(f, "--- CONSISTENCY (Pass^k) ---\n")
	fmt.Fprintf(f, "Score: %.3f (%s)\n", cr.Consistency.Score, cr.Consistency.ConsistencyLevel)
	fmt.Fprintf(f, "Pass^1: %.3f\n", cr.Consistency.PassHat1)
	if cr.Consistency.PassHat2 > 0 {
		fmt.Fprintf(f, "Pass^2: %.3f\n", cr.Consistency.PassHat2)
	}
	if cr.Consistency.PassHat4 > 0 {
		fmt.Fprintf(f, "Pass^4: %.3f\n", cr.Consistency.PassHat4)
	}
	fmt.Fprintf(f, "Success: %d/%d runs\n", cr.Consistency.SuccessCount, cr.Consistency.TotalRuns)
	fmt.Fprintf(f, "Variance: %.4f\n\n", cr.Consistency.Variance)

	fmt.Fprintf(f, "--- INFORMATION RETENTION ---\n")
	fmt.Fprintf(f, "Retention Rate: %.3f\n", cr.Retention.RetentionRate)
	fmt.Fprintf(f, "Key Info: %d found, %d retained\n", cr.Retention.KeyInfoCount, cr.Retention.RetainedCount)
	if len(cr.Retention.MissingInfo) > 0 {
		fmt.Fprintf(f, "Missing Info:\n")
		for _, info := range cr.Retention.MissingInfo {
			fmt.Fprintf(f, "  - %s\n", info)
		}
	}
	fmt.Fprintf(f, "\n")

	if len(cr.BaselineRuns) > 0 {
		run := cr.BaselineRuns[0]
		fmt.Fprintf(f, "--- BASELINE MODE (total=%d, prompt=%d, completion=%d, %v) ---\n",
			run.TotalTokens, run.PromptTokens, run.CompletionTokens, run.Duration.Round(time.Millisecond))
		for i, inv := range run.Invocations {
			fmt.Fprintf(f, "[Turn %s]", inv.InvocationID)
			if i < len(run.TokenUsagePerTurn) {
				u := run.TokenUsagePerTurn[i]
				fmt.Fprintf(f, " (p=%d, c=%d, t=%d)", u.PromptTokens, u.CompletionTokens, u.TotalTokens)
			}
			fmt.Fprintf(f, "\n")
			if inv.UserContent != nil {
				fmt.Fprintf(f, "User: %s\n", inv.UserContent.Content)
			}
			if inv.FinalResponse != nil {
				fmt.Fprintf(f, "Assistant: %s\n", inv.FinalResponse.Content)
			}
			fmt.Fprintf(f, "\n")
		}
	}

	if len(cr.SummaryRuns) > 0 {
		run := cr.SummaryRuns[0]
		fmt.Fprintf(f, "--- SUMMARY MODE (total=%d, prompt=%d, completion=%d, %v) ---\n",
			run.TotalTokens, run.PromptTokens, run.CompletionTokens, run.Duration.Round(time.Millisecond))
		for i, inv := range run.Invocations {
			fmt.Fprintf(f, "[Turn %s]", inv.InvocationID)
			if i < len(run.TokenUsagePerTurn) {
				u := run.TokenUsagePerTurn[i]
				fmt.Fprintf(f, " (p=%d, c=%d, t=%d)", u.PromptTokens, u.CompletionTokens, u.TotalTokens)
			}
			fmt.Fprintf(f, "\n")
			if inv.UserContent != nil {
				fmt.Fprintf(f, "User: %s\n", inv.UserContent.Content)
			}
			if inv.FinalResponse != nil {
				fmt.Fprintf(f, "Assistant: %s\n", inv.FinalResponse.Content)
			}
			fmt.Fprintf(f, "\n")
		}
	}
}

func saveMTBenchResults(outputDir string, results *MTBenchResults) {
	jsonPath := filepath.Join(outputDir, "results.json")
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal results: %v", err)
		return
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		log.Printf("Failed to write results: %v", err)
		return
	}
	log.Printf("Results saved to: %s", jsonPath)
}

func saveMTBenchCheckpoint(outputDir string, results *MTBenchResults) {
	checkpointPath := filepath.Join(outputDir, "checkpoint.json")
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal checkpoint: %v", err)
		return
	}
	if err := os.WriteFile(checkpointPath, data, 0644); err != nil {
		log.Printf("Failed to write checkpoint: %v", err)
	}
}

func loadMTBenchCheckpoint(outputDir string) *MTBenchResults {
	checkpointPath := filepath.Join(outputDir, "checkpoint.json")
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		return nil
	}
	var results MTBenchResults
	if err := json.Unmarshal(data, &results); err != nil {
		log.Printf("Failed to parse checkpoint: %v", err)
		return nil
	}
	return &results
}

func parseMTBenchRetention(result *evaluator.EvaluateResult) *MTBenchRetention {
	if result == nil {
		return &MTBenchRetention{}
	}

	r := &MTBenchRetention{RetentionRate: result.OverallScore}
	if result.Details == nil {
		return r
	}

	details := result.Details
	r.KeyInfoCount = asInt(firstNonNil(details["key_info_count"], details["total_key_info"]))
	r.RetainedCount = asInt(firstNonNil(details["retained_count"], details["total_retained"]))
	r.MissingInfo = asStringSlice(firstNonNil(details["missing_info"], details["unique_missing"]))
	r.PerTurn = asFloat64Slice(details["per_turn_retention"])
	r.PerRun = asFloat64Slice(details["per_run_retention"])
	return r
}

func validMTBench101TaskCodes() []string {
	return []string{
		"AR",
		"CC",
		"CM",
		"CR",
		"FR",
		"GR",
		"IC",
		"MR",
		"PI",
		"SA",
		"SC",
		"SI",
		"TS",
	}
}
