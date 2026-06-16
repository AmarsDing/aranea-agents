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
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	embedopenai "trpc.group/trpc-go/trpc-agent-go/knowledge/embedder/openai"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session"
	sessioninmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	sessionpgvector "trpc.group/trpc-go/trpc-agent-go/session/pgvector"
	sessionsummary "trpc.group/trpc-go/trpc-agent-go/session/summary"
)

const (
	lmeAppLongContext = "summary-lme-long-context"
	lmeAppSummary     = "summary-lme-summary"
	lmeAppOnDemand    = "summary-lme-ondemand"
	lmeTablePrefix    = "summary_lme"
	lmeAgentName      = "lme-eval-agent"
)

type lmeRunMode string

const (
	lmeModeLongContext lmeRunMode = "long_context"
	lmeModeSummary     lmeRunMode = "summary"
	lmeModeOnDemand    lmeRunMode = "summary_ondemand"
)

const lmeInstruction = `You are a helpful assistant with access to prior conversation history. Answer the user's question based on what was discussed in past conversations.

Rules:
- Answer directly and concisely using only information from the conversation history.
- If tools are available and important details may be hidden by session summary, inspect historical events before answering.
- Do not fabricate information not present in the history.`

const lmeOnDemandInstruction = `When session tools are available, treat this as a memory recall task.

Required behavior:
- Call session_search with scope=current_hidden before answering unless the visible summary already states the exact fact.
- Use specific entities, names, and topics from the question in the search query.
- If session_search returns hits but the context is insufficient, call session_load on the best hit.
- Do not claim information is unavailable if session_search returned relevant hits.`

// LMEModeResult stores one mode's answer, metrics, and token usage.
type LMEModeResult struct {
	Mode                   string        `json:"mode"`
	Answer                 string        `json:"answer"`
	Metrics                *QMSumMetrics `json:"metrics,omitempty"`
	TokenUsage             *TokenUsage   `json:"token_usage,omitempty"`
	ToolTraces             []ToolTrace   `json:"tool_traces,omitempty"`
	DurationMs             int64         `json:"duration_ms"`
	SeedDurationMs         int64         `json:"seed_duration_ms,omitempty"`
	SummaryBuildDurationMs int64         `json:"summary_build_duration_ms,omitempty"`
	QueryDurationMs        int64         `json:"query_duration_ms,omitempty"`
	SummaryAvailable       bool          `json:"summary_available,omitempty"`
	SummaryChars           int           `json:"summary_chars,omitempty"`
	SessionSearchCalls     int           `json:"session_search_calls,omitempty"`
	SessionLoadCalls       int           `json:"session_load_calls,omitempty"`
	Error                  string        `json:"error,omitempty"`
}

// LMECaseResult stores the three-mode comparison for one question.
type LMECaseResult struct {
	QuestionID   string         `json:"question_id"`
	QuestionType string         `json:"question_type"`
	Question     string         `json:"question"`
	Answer       string         `json:"answer"`
	TotalTurns   int            `json:"total_turns"`
	NumSessions  int            `json:"num_sessions"`
	LongContext  *LMEModeResult `json:"long_context,omitempty"`
	Summary      *LMEModeResult `json:"summary,omitempty"`
	OnDemand     *LMEModeResult `json:"summary_ondemand,omitempty"`
	OnDemandGain float64        `json:"ondemand_rougel_gain,omitempty"`
}

// LMEAggregate stores averaged metrics for one mode across cases.
type LMEAggregate struct {
	Count                     int     `json:"count"`
	AvgF1                     float64 `json:"avg_f1"`
	AvgBLEU                   float64 `json:"avg_bleu"`
	AvgROUGE1                 float64 `json:"avg_rouge_1"`
	AvgROUGE2                 float64 `json:"avg_rouge_2"`
	AvgROUGEL                 float64 `json:"avg_rouge_l"`
	AvgLLMScore               float64 `json:"avg_llm_score,omitempty"`
	AvgPromptTokens           float64 `json:"avg_prompt_tokens"`
	AvgCompletionTokens       float64 `json:"avg_completion_tokens"`
	AvgTotalTokens            float64 `json:"avg_total_tokens"`
	AvgLatencyMs              float64 `json:"avg_latency_ms"`
	AvgSeedDurationMs         float64 `json:"avg_seed_duration_ms,omitempty"`
	AvgSummaryBuildDurationMs float64 `json:"avg_summary_build_duration_ms,omitempty"`
	AvgQueryLatencyMs         float64 `json:"avg_query_latency_ms,omitempty"`
	AvgSummaryChars           float64 `json:"avg_summary_chars,omitempty"`
	MaxSummaryChars           int     `json:"max_summary_chars,omitempty"`
	MaxPromptTokens           int     `json:"max_prompt_tokens,omitempty"`
	SummaryAvailableRate      float64 `json:"summary_available_rate,omitempty"`
	AvgSessionSearchCalls     float64 `json:"avg_session_search_calls,omitempty"`
	AvgSessionLoadCalls       float64 `json:"avg_session_load_calls,omitempty"`
	PromptSavingsVsLong       float64 `json:"prompt_savings_vs_long,omitempty"`
	AvgExactMatch             float64 `json:"avg_exact_match,omitempty"`
}

// LMEResults is the output JSON payload for LongMemEval runs.
type LMEResults struct {
	Timestamp      string           `json:"timestamp"`
	Model          string           `json:"model"`
	DatasetFormat  string           `json:"dataset_format"`
	Dataset        string           `json:"dataset"`
	QuestionTypes  string           `json:"question_types"`
	LoadedCases    int              `json:"loaded_cases"`
	NumCases       int              `json:"num_cases"`
	EventThreshold int              `json:"event_threshold"`
	VisibleEvents  int              `json:"visible_events"`
	Cases          []*LMECaseResult `json:"cases"`

	LongContext *LMEAggregate `json:"long_context,omitempty"`
	Summary     *LMEAggregate `json:"summary,omitempty"`
	OnDemand    *LMEAggregate `json:"summary_ondemand,omitempty"`

	OnDemandROUGELGainAvg float64 `json:"ondemand_rouge_l_gain_avg,omitempty"`
}

// LongMemEvalBenchmark evaluates session recall on the LongMemEval dataset.
type LongMemEvalBenchmark struct {
	cfg *appConfig
}

func newLongMemEvalBenchmark(cfg *appConfig) *LongMemEvalBenchmark {
	return &LongMemEvalBenchmark{cfg: cfg}
}

func (b *LongMemEvalBenchmark) Run(ctx context.Context) error {
	log.Printf("=== Summary Evaluation (LongMemEval) ===")
	log.Printf("Model: %s", b.cfg.ModelName)
	log.Printf("Dataset: %s", b.cfg.DatasetPath)
	log.Printf("QuestionTypes: %s", b.cfg.LongMemEval.QuestionTypes)
	log.Printf("Output: %s", b.cfg.OutputDir)
	log.Printf("Event Threshold: %d", b.cfg.Events)
	log.Printf("Visible Events: %d", b.cfg.LongMemEval.VisibleEvents)
	logDetailedPromptConfig(b.cfg.DetailedPrompt)
	log.Printf("Agent options applied to summary/ondemand modes:")
	log.Printf("  - llmagent.WithAddSessionSummary(true)")
	log.Printf("  - llmagent.WithMessageBranchFilterMode(BranchFilterModeAll)  // see seedLongMemEvalSession comment")
	log.Printf("LLM Evaluation: %v", b.cfg.UseLLMEval)

	instances, err := dataset.LoadLongMemEval(b.cfg.DatasetPath)
	if err != nil {
		return fmt.Errorf("load LongMemEval: %w", err)
	}

	var questionTypes []string
	if b.cfg.LongMemEval.QuestionTypes != "" {
		for _, qt := range strings.Split(b.cfg.LongMemEval.QuestionTypes, ",") {
			qt = strings.TrimSpace(qt)
			if qt != "" {
				questionTypes = append(questionTypes, qt)
			}
		}
	}
	cases := dataset.FilterLongMemEval(instances, questionTypes)
	if len(cases) == 0 {
		return fmt.Errorf("no LongMemEval cases remain after filtering (types=%s)",
			b.cfg.LongMemEval.QuestionTypes)
	}
	if b.cfg.NumCases > 0 && b.cfg.NumCases < len(cases) {
		cases = cases[:b.cfg.NumCases]
	}
	log.Printf("Loaded %d LongMemEval instances, selected %d for evaluation",
		len(instances), len(cases))

	llm := openai.New(b.cfg.ModelName)
	var judge *qmsumLLMJudge
	if b.cfg.UseLLMEval {
		judge = newQMSumLLMJudge(llm)
	}

	longSvc := sessioninmemory.NewSessionService()
	defer func() {
		if err := longSvc.Close(); err != nil {
			log.Printf("close long-context session service: %v", err)
		}
	}()

	summarySvc, err := b.createLMESummaryService(llm)
	if err != nil {
		return err
	}
	defer func() {
		if err := summarySvc.Close(); err != nil {
			log.Printf("close LME summary session service: %v", err)
		}
	}()

	results := &LMEResults{
		Timestamp:      time.Now().Format(time.RFC3339),
		Model:          b.cfg.ModelName,
		DatasetFormat:  "longmemeval",
		Dataset:        b.cfg.DatasetPath,
		QuestionTypes:  b.cfg.LongMemEval.QuestionTypes,
		LoadedCases:    len(instances),
		NumCases:       len(cases),
		EventThreshold: b.cfg.Events,
		VisibleEvents:  b.cfg.LongMemEval.VisibleEvents,
		Cases:          make([]*LMECaseResult, 0, len(cases)),
	}

	start := time.Now()
	completed := make(map[string]bool)
	if b.cfg.Resume {
		if checkpoint := loadLMECheckpoint(b.cfg.OutputDir); checkpoint != nil {
			results = checkpoint
			completed = make(map[string]bool, len(results.Cases))
			for _, cr := range results.Cases {
				completed[cr.QuestionID] = true
			}
			log.Printf("Resumed LME checkpoint with %d completed cases", len(completed))
		}
	}

	for i, inst := range cases {
		if completed[inst.QuestionID] {
			log.Printf("[%d/%d] Case %s - SKIPPED", i+1, len(cases), inst.QuestionID)
			continue
		}

		log.Printf("")
		log.Printf("[%d/%d] Case %s (%s)", i+1, len(cases),
			inst.QuestionID, inst.QuestionType)

		caseResult, err := b.evaluateLMECase(
			ctx, llm, judge, longSvc, summarySvc, inst,
		)
		if err != nil {
			log.Printf("  Error: %v", err)
			continue
		}

		results.Cases = append(results.Cases, caseResult)
		saveLMECheckpoint(b.cfg.OutputDir, results)
		saveLMECaseLog(b.cfg.OutputDir, caseResult)
		logLMECaseResult(caseResult)

		elapsed := time.Since(start)
		avgPerCase := elapsed / time.Duration(len(results.Cases))
		remaining := avgPerCase * time.Duration(len(cases)-i-1)
		log.Printf("  Progress: %d/%d | Elapsed: %v | ETA: %v",
			i+1, len(cases), elapsed.Round(time.Second), remaining.Round(time.Second))
	}

	aggregateLMEResults(results)
	printLMEResults(results)
	saveLMEResults(b.cfg.OutputDir, results)
	return nil
}

func (b *LongMemEvalBenchmark) createLMESummaryService(
	llm model.Model,
) (session.Service, error) {
	dsn := b.cfg.LongMemEval.PGVectorDSN
	if dsn == "" {
		return nil, fmt.Errorf(
			"pgvector-dsn or PGVECTOR_DSN is required for LongMemEval summary benchmark",
		)
	}

	embedModelName := b.cfg.LongMemEval.EmbedModel
	emb := newLMEEmbeddingEmbedder(embedModelName)
	sum := sessionsummary.NewSummarizer(llm, summaryOptions(b.cfg)...)

	log.Printf(
		"Creating LME pgvector session service (embed_model=%s, visible_events=%d, detailed_prompt=%v)",
		embedModelName,
		b.cfg.LongMemEval.VisibleEvents,
		b.cfg.DetailedPrompt,
	)

	return sessionpgvector.NewService(
		sessionpgvector.WithPostgresClientDSN(dsn),
		sessionpgvector.WithEmbedder(emb),
		sessionpgvector.WithIndexDimension(emb.GetDimensions()),
		sessionpgvector.WithTablePrefix(lmeTablePrefix),
		sessionpgvector.WithSessionEventLimit(b.cfg.LongMemEval.VisibleEvents),
		sessionpgvector.WithSyncIndexing(true),
		sessionpgvector.WithMaxResults(10),
		sessionpgvector.WithSummarizer(sum),
		sessionpgvector.WithAsyncSummaryNum(1),
		sessionpgvector.WithSummaryQueueSize(16),
		sessionpgvector.WithSummaryJobTimeout(b.cfg.LongMemEval.SummaryWait),
	)
}

func (b *LongMemEvalBenchmark) evaluateLMECase(
	ctx context.Context,
	llm model.Model,
	judge *qmsumLLMJudge,
	longSvc session.Service,
	summarySvc session.Service,
	inst *dataset.LongMemEvalInstance,
) (*LMECaseResult, error) {
	if inst == nil {
		return nil, fmt.Errorf("LongMemEval instance is nil")
	}

	longResult, err := b.runLMEMode(
		ctx, llm, judge, longSvc, inst, lmeModeLongContext,
	)
	if err != nil {
		return nil, fmt.Errorf("long_context: %w", err)
	}

	summaryResult, err := b.runLMEMode(
		ctx, llm, judge, summarySvc, inst, lmeModeSummary,
	)
	if err != nil {
		return nil, fmt.Errorf("summary: %w", err)
	}

	onDemandResult, err := b.runLMEMode(
		ctx, llm, judge, summarySvc, inst, lmeModeOnDemand,
	)
	if err != nil {
		return nil, fmt.Errorf("summary_ondemand: %w", err)
	}

	result := &LMECaseResult{
		QuestionID:   inst.QuestionID,
		QuestionType: inst.QuestionType,
		Question:     inst.Question,
		Answer:       inst.Answer,
		TotalTurns:   inst.TotalTurns(),
		NumSessions:  len(inst.HaystackSessions),
		LongContext:  longResult,
		Summary:      summaryResult,
		OnDemand:     onDemandResult,
	}
	if summaryResult != nil && onDemandResult != nil &&
		summaryResult.Metrics != nil && onDemandResult.Metrics != nil {
		result.OnDemandGain = onDemandResult.Metrics.ROUGEL - summaryResult.Metrics.ROUGEL
	}
	return result, nil
}

func (b *LongMemEvalBenchmark) runLMEMode(
	ctx context.Context,
	llm model.Model,
	judge *qmsumLLMJudge,
	svc session.Service,
	inst *dataset.LongMemEvalInstance,
	mode lmeRunMode,
) (*LMEModeResult, error) {
	totalStart := time.Now()
	appName := lmeAppName(mode)
	userID := inst.QuestionID
	sessionID := fmt.Sprintf("%s-%s", string(mode), inst.QuestionID)

	key := session.Key{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	}
	defer func() {
		_ = svc.DeleteSession(context.Background(), key)
	}()

	seedStart := time.Now()
	sess, err := b.seedLongMemEvalSession(ctx, svc, key, inst)
	seedDurationMs := time.Since(seedStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("seed session: %w", err)
	}

	var (
		summaryText            string
		summaryAvailable       bool
		summaryBuildDurationMs int64
	)
	if mode == lmeModeSummary || mode == lmeModeOnDemand {
		summaryStart := time.Now()
		sess, summaryText, summaryAvailable, err = b.ensureLMESummary(
			ctx, svc, key, sess,
		)
		summaryBuildDurationMs = time.Since(summaryStart).Milliseconds()
		if err != nil {
			return nil, fmt.Errorf("prepare session summary: %w", err)
		}
		if !summaryAvailable {
			return nil, fmt.Errorf("session summary unavailable after force build")
		}
	}
	_ = sess

	ag := b.newLMEAgent(llm, mode)
	r := runner.NewRunner(appName, ag, runner.WithSessionService(svc))
	defer r.Close()

	queryStart := time.Now()
	evtCh, err := r.Run(ctx, userID, sessionID, model.NewUserMessage(inst.Question))
	if err != nil {
		return &LMEModeResult{
			Mode:                   string(mode),
			DurationMs:             time.Since(totalStart).Milliseconds(),
			SeedDurationMs:         seedDurationMs,
			SummaryBuildDurationMs: summaryBuildDurationMs,
			QueryDurationMs:        time.Since(queryStart).Milliseconds(),
			SummaryAvailable:       summaryAvailable,
			SummaryChars:           len(summaryText),
			Error:                  err.Error(),
		}, err
	}

	answer, usage, toolStats, toolTraces := consumeFinalAssistantAnswerWithDetails(evtCh)
	queryDurationMs := time.Since(queryStart).Milliseconds()
	trimmedAnswer := strings.TrimSpace(answer)

	return &LMEModeResult{
		Mode:                   string(mode),
		Answer:                 trimmedAnswer,
		Metrics:                evaluateLMEMetrics(ctx, judge, inst.Question, inst.Answer, trimmedAnswer),
		TokenUsage:             usage,
		ToolTraces:             toolTraces,
		DurationMs:             time.Since(totalStart).Milliseconds(),
		SeedDurationMs:         seedDurationMs,
		SummaryBuildDurationMs: summaryBuildDurationMs,
		QueryDurationMs:        queryDurationMs,
		SummaryAvailable:       summaryAvailable,
		SummaryChars:           len(summaryText),
		SessionSearchCalls:     toolStats.Count("session_search"),
		SessionLoadCalls:       toolStats.Count("session_load"),
	}, nil
}

func (b *LongMemEvalBenchmark) newLMEAgent(
	llm model.Model,
	mode lmeRunMode,
) agent.Agent {
	instruction := lmeInstruction
	if mode == lmeModeOnDemand {
		instruction += "\n\n" + lmeOnDemandInstruction
	}

	opts := []llmagent.Option{
		llmagent.WithModel(llm),
		llmagent.WithInstruction(instruction),
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream:      false,
			MaxTokens:   intPtr(b.cfg.LongMemEval.MaxTokens),
			Temperature: float64Ptr(0),
		}),
		// LongMemEval is a single-branch evaluation; force "all" mode so the
		// content processor reads summaries persisted under
		// SummaryFilterKeyAllContents (empty string key) instead of trying to
		// prefix-match the invocation's auto-derived filter key (the app
		// name), which would never match an empty-string summary key.
		llmagent.WithMessageBranchFilterMode(llmagent.BranchFilterModeAll),
	}

	if mode == lmeModeSummary || mode == lmeModeOnDemand {
		opts = append(opts, llmagent.WithAddSessionSummary(true))
	}
	if mode == lmeModeOnDemand {
		opts = append(opts,
			llmagent.WithEnableOnDemandSession(true),
			llmagent.WithMaxToolIterations(b.cfg.LongMemEval.MaxToolIterations),
		)
	}

	return llmagent.New(lmeAgentName, opts...)
}

func lmeAppName(mode lmeRunMode) string {
	switch mode {
	case lmeModeSummary:
		return lmeAppSummary
	case lmeModeOnDemand:
		return lmeAppOnDemand
	case lmeModeLongContext:
		fallthrough
	default:
		return lmeAppLongContext
	}
}

func (b *LongMemEvalBenchmark) seedLongMemEvalSession(
	ctx context.Context,
	svc session.Service,
	key session.Key,
	inst *dataset.LongMemEvalInstance,
) (*session.Session, error) {
	_ = svc.DeleteSession(ctx, key)
	sess, err := svc.CreateSession(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	baseTime := sess.CreatedAt.UTC().Add(time.Millisecond)
	turnIndex := 0
	for _, sessionTurns := range inst.HaystackSessions {
		for _, turn := range sessionTurns {
			role := model.RoleUser
			author := "user"
			if turn.Role == "assistant" {
				role = model.RoleAssistant
				// Author must equal the consuming agent's name to avoid
				// being treated as a foreign-agent reply by the content
				// processor (which would otherwise rewrite all assistant
				// turns into user-role context messages).
				author = lmeAgentName
			}
			evt := event.New(
				fmt.Sprintf("%s-%04d", key.SessionID, turnIndex),
				author,
				event.WithResponse(&model.Response{
					Done: true,
					Choices: []model.Choice{{
						Message: model.Message{
							Role:    role,
							Content: turn.Content,
						},
					}},
				}),
			)
			evt.Timestamp = baseTime.Add(time.Duration(turnIndex) * time.Millisecond)
			if err := svc.AppendEvent(ctx, sess, evt); err != nil {
				return nil, fmt.Errorf("append seed event %d: %w", turnIndex, err)
			}
			turnIndex++
		}
	}
	return sess, nil
}

func (b *LongMemEvalBenchmark) ensureLMESummary(
	ctx context.Context,
	svc session.Service,
	key session.Key,
	sess *session.Session,
) (*session.Session, string, bool, error) {
	if err := svc.CreateSessionSummary(
		ctx,
		sess,
		session.SummaryFilterKeyAllContents,
		true,
	); err != nil {
		return nil, "", false, fmt.Errorf("force summary build: %w", err)
	}

	if text, ok := svc.GetSessionSummaryText(ctx, sess); ok && strings.TrimSpace(text) != "" {
		refreshed, err := svc.GetSession(ctx, key)
		if err != nil {
			return nil, "", false, fmt.Errorf("refresh session: %w", err)
		}
		return refreshed, text, true, nil
	}

	text, ok, err := b.waitForLMESummary(ctx, svc, key)
	if err != nil {
		return nil, "", false, err
	}
	refreshed, refreshErr := svc.GetSession(ctx, key)
	if refreshErr != nil {
		return nil, "", false, fmt.Errorf("refresh session: %w", refreshErr)
	}
	return refreshed, text, ok, nil
}

func (b *LongMemEvalBenchmark) waitForLMESummary(
	ctx context.Context,
	svc session.Service,
	key session.Key,
) (string, bool, error) {
	deadline := time.Now().Add(b.cfg.LongMemEval.SummaryWait)
	for {
		sess, err := svc.GetSession(ctx, key)
		if err == nil && sess != nil {
			if text, ok := svc.GetSessionSummaryText(ctx, sess); ok && strings.TrimSpace(text) != "" {
				return text, true, nil
			}
		}
		if time.Now().After(deadline) {
			return "", false, nil
		}
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// evaluateLMEMetrics computes F1, ROUGE, BLEU, and exact-match metrics for LME.
func evaluateLMEMetrics(
	ctx context.Context,
	judge *qmsumLLMJudge,
	query, reference, prediction string,
) *QMSumMetrics {
	result := &QMSumMetrics{
		F1:     calculateF1(prediction, reference),
		BLEU:   calculateBLEU(prediction, reference),
		ROUGE1: calculateROUGE1(prediction, reference),
		ROUGE2: calculateROUGE2(prediction, reference),
		ROUGEL: calculateROUGEL(prediction, reference),
	}
	// Exact match: check if the reference answer appears as a substring in prediction.
	if containsExactMatch(prediction, reference) {
		result.LLMScore = 1.0
	} else if judge != nil {
		if eval, err := judge.Evaluate(ctx, query, reference, prediction); err == nil && eval.Correct {
			result.LLMScore = eval.Confidence
		}
	}
	return result
}

// containsExactMatch checks if the normalized reference answer is a substring
// of the normalized prediction. This is appropriate for LongMemEval since
// answers are often short factual strings (names, dates, numbers).
func containsExactMatch(prediction, reference string) bool {
	normPred := strings.ToLower(strings.TrimSpace(prediction))
	normRef := strings.ToLower(strings.TrimSpace(reference))
	if normRef == "" {
		return normPred == ""
	}
	return strings.Contains(normPred, normRef)
}

// --- Aggregation ---

func aggregateLMEResults(results *LMEResults) {
	results.LongContext = aggregateLMEMode(results.Cases, lmeModeLongContext)
	results.Summary = aggregateLMEMode(results.Cases, lmeModeSummary)
	results.OnDemand = aggregateLMEMode(results.Cases, lmeModeOnDemand)
	if len(results.Cases) == 0 {
		return
	}

	var totalGain float64
	for _, cr := range results.Cases {
		totalGain += cr.OnDemandGain
	}
	results.OnDemandROUGELGainAvg = totalGain / float64(len(results.Cases))

	if results.LongContext != nil {
		if results.Summary != nil {
			results.Summary.PromptSavingsVsLong = averageLMEPromptSavings(
				results.Cases,
				func(cr *LMECaseResult) float64 {
					if cr.LongContext == nil || cr.Summary == nil ||
						cr.LongContext.TokenUsage == nil || cr.Summary.TokenUsage == nil ||
						cr.LongContext.TokenUsage.PromptTokens == 0 {
						return 0
					}
					return 100 * float64(
						cr.LongContext.TokenUsage.PromptTokens-cr.Summary.TokenUsage.PromptTokens,
					) / float64(cr.LongContext.TokenUsage.PromptTokens)
				},
			)
		}
		if results.OnDemand != nil {
			results.OnDemand.PromptSavingsVsLong = averageLMEPromptSavings(
				results.Cases,
				func(cr *LMECaseResult) float64 {
					if cr.LongContext == nil || cr.OnDemand == nil ||
						cr.LongContext.TokenUsage == nil || cr.OnDemand.TokenUsage == nil ||
						cr.LongContext.TokenUsage.PromptTokens == 0 {
						return 0
					}
					return 100 * float64(
						cr.LongContext.TokenUsage.PromptTokens-cr.OnDemand.TokenUsage.PromptTokens,
					) / float64(cr.LongContext.TokenUsage.PromptTokens)
				},
			)
		}
	}
}

func aggregateLMEMode(
	cases []*LMECaseResult,
	mode lmeRunMode,
) *LMEAggregate {
	agg := &LMEAggregate{}
	var summaryAvailableCount int
	var exactMatchSum float64
	for _, cr := range cases {
		var mr *LMEModeResult
		switch mode {
		case lmeModeSummary:
			mr = cr.Summary
		case lmeModeOnDemand:
			mr = cr.OnDemand
		case lmeModeLongContext:
			fallthrough
		default:
			mr = cr.LongContext
		}
		if mr == nil || mr.Metrics == nil || mr.TokenUsage == nil {
			continue
		}

		agg.Count++
		agg.AvgF1 += mr.Metrics.F1
		agg.AvgBLEU += mr.Metrics.BLEU
		agg.AvgROUGE1 += mr.Metrics.ROUGE1
		agg.AvgROUGE2 += mr.Metrics.ROUGE2
		agg.AvgROUGEL += mr.Metrics.ROUGEL
		agg.AvgLLMScore += mr.Metrics.LLMScore
		agg.AvgPromptTokens += float64(mr.TokenUsage.PromptTokens)
		agg.AvgCompletionTokens += float64(mr.TokenUsage.CompletionTokens)
		agg.AvgTotalTokens += float64(mr.TokenUsage.TotalTokens)
		agg.AvgLatencyMs += float64(mr.DurationMs)
		agg.AvgSeedDurationMs += float64(mr.SeedDurationMs)
		agg.AvgSummaryBuildDurationMs += float64(mr.SummaryBuildDurationMs)
		agg.AvgQueryLatencyMs += float64(mr.QueryDurationMs)
		agg.AvgSummaryChars += float64(mr.SummaryChars)
		if mr.SummaryChars > agg.MaxSummaryChars {
			agg.MaxSummaryChars = mr.SummaryChars
		}
		if mr.TokenUsage.PromptTokens > agg.MaxPromptTokens {
			agg.MaxPromptTokens = mr.TokenUsage.PromptTokens
		}
		agg.AvgSessionSearchCalls += float64(mr.SessionSearchCalls)
		agg.AvgSessionLoadCalls += float64(mr.SessionLoadCalls)
		if mr.SummaryAvailable {
			summaryAvailableCount++
		}
		if containsExactMatch(mr.Answer, cr.Answer) {
			exactMatchSum += 1.0
		}
	}
	if agg.Count == 0 {
		return agg
	}

	n := float64(agg.Count)
	agg.AvgF1 /= n
	agg.AvgBLEU /= n
	agg.AvgROUGE1 /= n
	agg.AvgROUGE2 /= n
	agg.AvgROUGEL /= n
	agg.AvgLLMScore /= n
	agg.AvgPromptTokens /= n
	agg.AvgCompletionTokens /= n
	agg.AvgTotalTokens /= n
	agg.AvgLatencyMs /= n
	agg.AvgSeedDurationMs /= n
	agg.AvgSummaryBuildDurationMs /= n
	agg.AvgQueryLatencyMs /= n
	agg.AvgSummaryChars /= n
	agg.AvgSessionSearchCalls /= n
	agg.AvgSessionLoadCalls /= n
	agg.SummaryAvailableRate = float64(summaryAvailableCount) / n
	agg.AvgExactMatch = exactMatchSum / n
	return agg
}

func averageLMEPromptSavings(
	cases []*LMECaseResult,
	getter func(*LMECaseResult) float64,
) float64 {
	if len(cases) == 0 {
		return 0
	}
	var total float64
	for _, cr := range cases {
		total += getter(cr)
	}
	return total / float64(len(cases))
}

// --- Output / Printing ---

func printLMEResults(results *LMEResults) {
	fmt.Println("\n" + strings.Repeat("=", 72))
	fmt.Println("Summary Evaluation Results (LongMemEval)")
	fmt.Println(strings.Repeat("=", 72))
	fmt.Printf("Model: %s\n", results.Model)
	fmt.Printf("Cases: %d/%d | QuestionTypes: %s\n",
		results.NumCases, results.LoadedCases, results.QuestionTypes)
	fmt.Printf("Visible Events: %d\n", results.VisibleEvents)

	printLMEAggregate("Long Context", results.LongContext)
	printLMEAggregate("Summary", results.Summary)
	printLMEAggregate("Summary On-Demand", results.OnDemand)

	fmt.Printf("\nOn-Demand ROUGE-L Gain vs Summary: %.4f\n", results.OnDemandROUGELGainAvg)
	fmt.Println(strings.Repeat("=", 72))
}

func printLMEAggregate(title string, agg *LMEAggregate) {
	if agg == nil || agg.Count == 0 {
		fmt.Printf("\n--- %s ---\n(no successful cases)\n", title)
		return
	}
	fmt.Printf("\n--- %s ---\n", title)
	fmt.Printf("Cases: %d\n", agg.Count)
	fmt.Printf("ROUGE-1/2/L: %.4f / %.4f / %.4f\n",
		agg.AvgROUGE1, agg.AvgROUGE2, agg.AvgROUGEL)
	fmt.Printf("F1/BLEU: %.4f / %.4f\n", agg.AvgF1, agg.AvgBLEU)
	fmt.Printf("Exact Match: %.4f\n", agg.AvgExactMatch)
	if agg.AvgLLMScore > 0 {
		fmt.Printf("LLM Score: %.4f\n", agg.AvgLLMScore)
	}
	fmt.Printf("Avg Tokens (prompt/completion/total): %.0f / %.0f / %.0f\n",
		agg.AvgPromptTokens, agg.AvgCompletionTokens, agg.AvgTotalTokens)
	if agg.MaxPromptTokens > 0 {
		fmt.Printf("Max Prompt Tokens: %d\n", agg.MaxPromptTokens)
	}
	fmt.Printf("Avg Latency (total/query): %.0f ms / %.0f ms\n",
		agg.AvgLatencyMs, agg.AvgQueryLatencyMs)
	if agg.AvgSeedDurationMs > 0 || agg.AvgSummaryBuildDurationMs > 0 {
		fmt.Printf("Avg Prep (seed/summary): %.0f ms / %.0f ms\n",
			agg.AvgSeedDurationMs, agg.AvgSummaryBuildDurationMs)
	}
	if agg.AvgSummaryChars > 0 || agg.SummaryAvailableRate > 0 {
		fmt.Printf("Summary Available Rate: %.2f%% | Avg/Max Summary Chars: %.0f / %d\n",
			100*agg.SummaryAvailableRate, agg.AvgSummaryChars, agg.MaxSummaryChars)
	}
	if agg.AvgSessionSearchCalls > 0 || agg.AvgSessionLoadCalls > 0 {
		fmt.Printf("Avg Tool Calls (search/load): %.2f / %.2f\n",
			agg.AvgSessionSearchCalls, agg.AvgSessionLoadCalls)
	}
	if agg.PromptSavingsVsLong != 0 {
		fmt.Printf("Prompt Savings vs Long Context: %.2f%%\n", agg.PromptSavingsVsLong)
	}
}

func logLMECaseResult(cr *LMECaseResult) {
	log.Printf("  Question: %s", truncateStr(cr.Question, 180))
	log.Printf("  Type: %s | Turns: %d | Sessions: %d",
		cr.QuestionType, cr.TotalTurns, cr.NumSessions)
	if cr.LongContext != nil && cr.LongContext.Metrics != nil && cr.LongContext.TokenUsage != nil {
		log.Printf("  LongContext       R-L=%.4f EM=%v p=%d c=%d t=%d total=%dms query=%dms",
			cr.LongContext.Metrics.ROUGEL,
			containsExactMatch(cr.LongContext.Answer, cr.Answer),
			cr.LongContext.TokenUsage.PromptTokens,
			cr.LongContext.TokenUsage.CompletionTokens,
			cr.LongContext.TokenUsage.TotalTokens,
			cr.LongContext.DurationMs,
			cr.LongContext.QueryDurationMs,
		)
	}
	if cr.Summary != nil && cr.Summary.Metrics != nil && cr.Summary.TokenUsage != nil {
		log.Printf("  Summary           R-L=%.4f EM=%v p=%d c=%d t=%d summary=%v chars=%d total=%dms query=%dms",
			cr.Summary.Metrics.ROUGEL,
			containsExactMatch(cr.Summary.Answer, cr.Answer),
			cr.Summary.TokenUsage.PromptTokens,
			cr.Summary.TokenUsage.CompletionTokens,
			cr.Summary.TokenUsage.TotalTokens,
			cr.Summary.SummaryAvailable,
			cr.Summary.SummaryChars,
			cr.Summary.DurationMs,
			cr.Summary.QueryDurationMs,
		)
	}
	if cr.OnDemand != nil && cr.OnDemand.Metrics != nil && cr.OnDemand.TokenUsage != nil {
		log.Printf("  Summary+OnDemand  R-L=%.4f EM=%v gain=%.4f tools=(%d/%d) p=%d c=%d t=%d total=%dms query=%dms",
			cr.OnDemand.Metrics.ROUGEL,
			containsExactMatch(cr.OnDemand.Answer, cr.Answer),
			cr.OnDemandGain,
			cr.OnDemand.SessionSearchCalls,
			cr.OnDemand.SessionLoadCalls,
			cr.OnDemand.TokenUsage.PromptTokens,
			cr.OnDemand.TokenUsage.CompletionTokens,
			cr.OnDemand.TokenUsage.TotalTokens,
			cr.OnDemand.DurationMs,
			cr.OnDemand.QueryDurationMs,
		)
	}
}

func saveLMECaseLog(outputDir string, cr *LMECaseResult) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("mkdir LME output dir: %v", err)
		return
	}

	path := filepath.Join(outputDir, cr.QuestionID+".log")
	f, err := os.Create(path)
	if err != nil {
		log.Printf("create LME case log: %v", err)
		return
	}
	defer f.Close()

	writeMode := func(title string, mr *LMEModeResult) {
		if mr == nil {
			return
		}
		fmt.Fprintf(f, "=== %s ===\n", title)
		fmt.Fprintf(f, "Duration: %dms\n", mr.DurationMs)
		fmt.Fprintf(f, "SeedDuration: %dms\n", mr.SeedDurationMs)
		fmt.Fprintf(f, "SummaryBuildDuration: %dms\n", mr.SummaryBuildDurationMs)
		fmt.Fprintf(f, "QueryDuration: %dms\n", mr.QueryDurationMs)
		fmt.Fprintf(f, "SummaryAvailable: %v\n", mr.SummaryAvailable)
		fmt.Fprintf(f, "SummaryChars: %d\n", mr.SummaryChars)
		fmt.Fprintf(f, "ToolCalls: session_search=%d session_load=%d\n",
			mr.SessionSearchCalls, mr.SessionLoadCalls)
		if len(mr.ToolTraces) > 0 {
			fmt.Fprintf(f, "ToolTrace:\n")
			for _, trace := range mr.ToolTraces {
				fmt.Fprintf(
					f,
					"- %s args=%s\n  response=%s\n",
					compactToolTraceText(trace.Name, 48),
					compactToolTraceText(trace.Arguments, 320),
					compactToolTraceText(trace.Response, 600),
				)
			}
		}
		if mr.TokenUsage != nil {
			fmt.Fprintf(f, "Tokens: prompt=%d completion=%d total=%d\n",
				mr.TokenUsage.PromptTokens,
				mr.TokenUsage.CompletionTokens,
				mr.TokenUsage.TotalTokens,
			)
		}
		if mr.Metrics != nil {
			fmt.Fprintf(f, "Metrics: F1=%.4f BLEU=%.4f R1=%.4f R2=%.4f RL=%.4f LLM=%.4f\n",
				mr.Metrics.F1, mr.Metrics.BLEU, mr.Metrics.ROUGE1,
				mr.Metrics.ROUGE2, mr.Metrics.ROUGEL, mr.Metrics.LLMScore,
			)
		}
		if mr.Error != "" {
			fmt.Fprintf(f, "Error: %s\n", mr.Error)
		}
		fmt.Fprintf(f, "ExactMatch: %v\n", containsExactMatch(mr.Answer, cr.Answer))
		fmt.Fprintf(f, "Answer:\n%s\n\n", mr.Answer)
	}

	fmt.Fprintf(f, "QuestionID: %s\nQuestionType: %s\n\n",
		cr.QuestionID, cr.QuestionType)
	fmt.Fprintf(f, "TotalTurns: %d\nNumSessions: %d\n", cr.TotalTurns, cr.NumSessions)
	fmt.Fprintf(f, "\nQuestion:\n%s\n\nReference Answer:\n%s\n\n",
		cr.Question, cr.Answer)
	writeMode("LONG CONTEXT", cr.LongContext)
	writeMode("SUMMARY", cr.Summary)
	writeMode("SUMMARY ON-DEMAND", cr.OnDemand)
}

func saveLMEResults(outputDir string, results *LMEResults) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("mkdir LME output dir: %v", err)
		return
	}

	path := filepath.Join(outputDir, "results.json")
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("marshal LME results: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("write LME results: %v", err)
		return
	}
	log.Printf("Results saved to: %s", path)
}

func saveLMECheckpoint(outputDir string, results *LMEResults) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("mkdir LME output dir: %v", err)
		return
	}

	path := filepath.Join(outputDir, "checkpoint.json")
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("marshal LME checkpoint: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("write LME checkpoint: %v", err)
	}
}

func loadLMECheckpoint(outputDir string) *LMEResults {
	path := filepath.Join(outputDir, "checkpoint.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var results LMEResults
	if err := json.Unmarshal(data, &results); err != nil {
		log.Printf("parse LME checkpoint: %v", err)
		return nil
	}
	return &results
}

func newLMEEmbeddingEmbedder(modelName string) *embedopenai.Embedder {
	opts := []embedopenai.Option{
		embedopenai.WithModel(modelName),
	}
	if apiKey := os.Getenv("OPENAI_EMBEDDING_API_KEY"); apiKey != "" {
		opts = append(opts, embedopenai.WithAPIKey(apiKey))
	}
	baseURL := os.Getenv("OPENAI_EMBEDDING_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if baseURL != "" {
		opts = append(opts, embedopenai.WithBaseURL(baseURL))
	}
	return embedopenai.New(opts...)
}
