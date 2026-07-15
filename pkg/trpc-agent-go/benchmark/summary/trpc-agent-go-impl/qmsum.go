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
	"sort"
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
	qmsumAppLongContext = "summary-qmsum-long-context"
	qmsumAppSummary     = "summary-qmsum-summary"
	qmsumAppOnDemand    = "summary-qmsum-ondemand"

	qmsumTablePrefix = "summary_qmsum"
)

type qmsumRunMode string

const (
	qmsumModeLongContext qmsumRunMode = "long_context"
	qmsumModeSummary     qmsumRunMode = "summary"
	qmsumModeOnDemand    qmsumRunMode = "summary_ondemand"
)

const qmsumInstruction = `You answer query-based meeting summarization questions from the current session transcript.

Rules:
- Answer the user's query directly and faithfully using only transcript-supported information.
- For a general query, summarize the whole meeting concisely.
- For a specific query, summarize only the requested discussion or point of view.
- Preserve important people, decisions, numbers, dates, and causal relations.
- Prefer one concise paragraph. Do not add bullet points or meta commentary.
- If tools are available and important details may be hidden by session summary, inspect historical events before answering.`

const qmsumOnDemandInstruction = `When session tools are available, treat this as a hidden-context recovery task.

Required behavior for detail questions:
- For specific/detail questions, call session_search with scope=current_hidden before answering unless the visible summary already states the exact requested fact.
- Use the user's real entities, names, and topics in the search query. If the first search is empty, rewrite the query once and search again.
- session_search results may already contain a context field with raw nearby turns. Read that context first and answer from it directly when it already contains the needed evidence.
- If session_search returns hits but the provided context is still insufficient, call session_load on the best hit before answering.
- Do not claim the topic was not discussed, that a person did not participate, or that the answer is unavailable if session_search returned relevant hits mentioning the requested entities or topic.
- Never copy raw tool JSON, snippets, or schema-shaped output into the final answer. Use tool results only as evidence for a natural-language answer.`

// QMSumModeResult stores one mode's answer, metrics, and token usage.
type QMSumModeResult struct {
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

// QMSumCaseResult stores the three-mode comparison for one query.
type QMSumCaseResult struct {
	CaseID                 string `json:"case_id"`
	MeetingID              string `json:"meeting_id"`
	Domain                 string `json:"domain"`
	QueryType              string `json:"query_type"`
	Query                  string `json:"query"`
	Reference              string `json:"reference"`
	Turns                  int    `json:"turns"`
	Transcript             int    `json:"transcript_chars"`
	SupportWindowAvailable bool   `json:"support_window_available"`
	SupportStartTurn       int    `json:"support_start_turn"`
	SupportEndTurn         int    `json:"support_end_turn"`
	SupportDistanceFromEnd int    `json:"support_distance_from_end"`

	LongContext *QMSumModeResult `json:"long_context,omitempty"`
	Summary     *QMSumModeResult `json:"summary,omitempty"`
	OnDemand    *QMSumModeResult `json:"summary_ondemand,omitempty"`

	SummaryPromptSavings  float64 `json:"summary_prompt_savings,omitempty"`
	OnDemandPromptSavings float64 `json:"ondemand_prompt_savings,omitempty"`
	OnDemandROUGELGain    float64 `json:"ondemand_rougel_gain,omitempty"`
}

// QMSumAggregate stores averaged metrics for one mode across cases.
type QMSumAggregate struct {
	Count                     int           `json:"count"`
	AvgF1                     float64       `json:"avg_f1"`
	AvgBLEU                   float64       `json:"avg_bleu"`
	AvgROUGE1                 float64       `json:"avg_rouge_1"`
	AvgROUGE2                 float64       `json:"avg_rouge_2"`
	AvgROUGEL                 float64       `json:"avg_rouge_l"`
	AvgLLMScore               float64       `json:"avg_llm_score,omitempty"`
	AvgPromptTokens           float64       `json:"avg_prompt_tokens"`
	AvgCompletionTokens       float64       `json:"avg_completion_tokens"`
	AvgTotalTokens            float64       `json:"avg_total_tokens"`
	AvgLatencyMs              float64       `json:"avg_latency_ms"`
	AvgSeedDurationMs         float64       `json:"avg_seed_duration_ms,omitempty"`
	AvgSummaryBuildDurationMs float64       `json:"avg_summary_build_duration_ms,omitempty"`
	AvgQueryLatencyMs         float64       `json:"avg_query_latency_ms,omitempty"`
	AvgSummaryChars           float64       `json:"avg_summary_chars,omitempty"`
	SummaryAvailableRate      float64       `json:"summary_available_rate,omitempty"`
	AvgSessionSearchCalls     float64       `json:"avg_session_search_calls,omitempty"`
	AvgSessionLoadCalls       float64       `json:"avg_session_load_calls,omitempty"`
	PromptSavingsVsLong       float64       `json:"prompt_savings_vs_long,omitempty"`
	Duration                  time.Duration `json:"-"`
}

// QMSumResults is the output JSON payload for QMSum runs.
type QMSumResults struct {
	Timestamp          string             `json:"timestamp"`
	Model              string             `json:"model"`
	DatasetFormat      string             `json:"dataset_format"`
	Dataset            string             `json:"dataset"`
	Split              string             `json:"split"`
	Domain             string             `json:"domain"`
	QueryType          string             `json:"query_type"`
	LoadedCases        int                `json:"loaded_cases"`
	NumCases           int                `json:"num_cases"`
	EventThreshold     int                `json:"event_threshold"`
	VisibleEvents      int                `json:"visible_events"`
	MinDistanceFromEnd int                `json:"min_distance_from_end"`
	Cases              []*QMSumCaseResult `json:"cases"`

	LongContext *QMSumAggregate `json:"long_context,omitempty"`
	Summary     *QMSumAggregate `json:"summary,omitempty"`
	OnDemand    *QMSumAggregate `json:"summary_ondemand,omitempty"`

	OnDemandROUGELGainAvg float64 `json:"ondemand_rouge_l_gain_avg,omitempty"`
}

type QMSumBenchmark struct {
	cfg *appConfig
}

func newQMSumBenchmark(cfg *appConfig) *QMSumBenchmark {
	return &QMSumBenchmark{cfg: cfg}
}

func (b *QMSumBenchmark) Run(ctx context.Context) error {
	log.Printf("=== Summary Evaluation (QMSum) ===")
	log.Printf("Model: %s", b.cfg.ModelName)
	log.Printf("Dataset: %s", b.cfg.DatasetPath)
	log.Printf("Split: %s | Domain: %s | QueryType: %s",
		b.cfg.QMSum.Split, b.cfg.QMSum.Domain, b.cfg.QMSum.QueryType)
	log.Printf("Output: %s", b.cfg.OutputDir)
	log.Printf("Event Threshold: %d", b.cfg.Events)
	log.Printf("Visible Events: %d", b.cfg.QMSum.VisibleEvents)
	logDetailedPromptConfig(b.cfg.DetailedPrompt)
	log.Printf("Agent options applied to summary/ondemand modes:")
	log.Printf("  - llmagent.WithAddSessionSummary(true)")
	log.Printf("  - llmagent.WithMessageBranchFilterMode(BranchFilterModeAll)")
	log.Printf("Min Support Distance From End: %d", b.cfg.QMSum.MinDistanceFromEnd)
	log.Printf("LLM Evaluation: %v", b.cfg.UseLLMEval)

	loader := dataset.NewDatasetLoader(b.cfg.DatasetPath)
	loadedCases, err := loader.LoadQMSum(
		b.cfg.QMSum.Split,
		b.cfg.QMSum.Domain,
		b.cfg.QMSum.QueryType,
	)
	if err != nil {
		return fmt.Errorf("load QMSum: %w", err)
	}

	cases := b.selectQMSumCases(loadedCases)
	if len(cases) == 0 {
		return fmt.Errorf(
			"no QMSum cases remain after filtering (min_distance_from_end=%d)",
			b.cfg.QMSum.MinDistanceFromEnd,
		)
	}
	log.Printf("Loaded %d QMSum cases, selected %d for evaluation",
		len(loadedCases), len(cases))

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

	summarySvc, err := b.createQMSumSummaryService(llm)
	if err != nil {
		return err
	}
	defer func() {
		if err := summarySvc.Close(); err != nil {
			log.Printf("close summary session service: %v", err)
		}
	}()

	results := &QMSumResults{
		Timestamp:          time.Now().Format(time.RFC3339),
		Model:              b.cfg.ModelName,
		DatasetFormat:      "qmsum",
		Dataset:            b.cfg.DatasetPath,
		Split:              b.cfg.QMSum.Split,
		Domain:             b.cfg.QMSum.Domain,
		QueryType:          b.cfg.QMSum.QueryType,
		LoadedCases:        len(loadedCases),
		NumCases:           len(cases),
		EventThreshold:     b.cfg.Events,
		VisibleEvents:      b.cfg.QMSum.VisibleEvents,
		MinDistanceFromEnd: b.cfg.QMSum.MinDistanceFromEnd,
		Cases:              make([]*QMSumCaseResult, 0, len(cases)),
	}

	start := time.Now()
	completed := make(map[string]bool)
	if b.cfg.Resume {
		if checkpoint := loadQMSumCheckpoint(b.cfg.OutputDir); checkpoint != nil {
			results = checkpoint
			completed = make(map[string]bool, len(results.Cases))
			for _, cr := range results.Cases {
				completed[cr.CaseID] = true
			}
			log.Printf("Resumed QMSum checkpoint with %d completed cases", len(completed))
		}
	}

	for i, qcase := range cases {
		if completed[qcase.CaseID] {
			log.Printf("[%d/%d] Case %s - SKIPPED", i+1, len(cases), qcase.CaseID)
			continue
		}

		log.Printf("")
		log.Printf("[%d/%d] Case %s (%s/%s)", i+1, len(cases),
			qcase.CaseID, qcase.Domain, qcase.QueryType)

		caseResult, err := b.evaluateQMSumCase(
			ctx,
			llm,
			judge,
			longSvc,
			summarySvc,
			qcase,
		)
		if err != nil {
			log.Printf("  Error: %v", err)
			continue
		}

		results.Cases = append(results.Cases, caseResult)
		saveQMSumCheckpoint(b.cfg.OutputDir, results)
		saveQMSumCaseLog(b.cfg.OutputDir, caseResult)
		logQMSumCaseResult(caseResult)

		elapsed := time.Since(start)
		avgPerCase := elapsed / time.Duration(len(results.Cases))
		remaining := avgPerCase * time.Duration(len(cases)-i-1)
		log.Printf("  Progress: %d/%d | Elapsed: %v | ETA: %v",
			i+1, len(cases), elapsed.Round(time.Second), remaining.Round(time.Second))
	}

	aggregateQMSumResults(results)
	printQMSumResults(results)
	saveQMSumResults(b.cfg.OutputDir, results)
	return nil
}

func (b *QMSumBenchmark) selectQMSumCases(
	cases []*dataset.QMSumCase,
) []*dataset.QMSumCase {
	selected := filterQMSumCasesBySupportDistance(
		cases,
		b.cfg.QMSum.MinDistanceFromEnd,
	)
	selected = roundRobinQMSumCasesByMeeting(selected)
	if b.cfg.NumCases > 0 && b.cfg.NumCases < len(selected) {
		selected = selected[:b.cfg.NumCases]
	}
	return selected
}

func filterQMSumCasesBySupportDistance(
	cases []*dataset.QMSumCase,
	minDistance int,
) []*dataset.QMSumCase {
	if minDistance <= 0 {
		return append([]*dataset.QMSumCase(nil), cases...)
	}

	filtered := make([]*dataset.QMSumCase, 0, len(cases))
	for _, qcase := range cases {
		distance, ok := qcase.SupportDistanceFromEnd()
		if ok && distance >= minDistance {
			filtered = append(filtered, qcase)
		}
	}
	return filtered
}

func roundRobinQMSumCasesByMeeting(
	cases []*dataset.QMSumCase,
) []*dataset.QMSumCase {
	if len(cases) <= 1 {
		return append([]*dataset.QMSumCase(nil), cases...)
	}

	grouped := make(map[string][]*dataset.QMSumCase)
	meetingIDs := make([]string, 0)
	for _, qcase := range cases {
		if qcase == nil {
			continue
		}
		if _, ok := grouped[qcase.MeetingID]; !ok {
			meetingIDs = append(meetingIDs, qcase.MeetingID)
		}
		grouped[qcase.MeetingID] = append(grouped[qcase.MeetingID], qcase)
	}

	sort.Strings(meetingIDs)
	for _, meetingID := range meetingIDs {
		sort.Slice(grouped[meetingID], func(i, j int) bool {
			return grouped[meetingID][i].CaseID < grouped[meetingID][j].CaseID
		})
	}

	ordered := make([]*dataset.QMSumCase, 0, len(cases))
	for added := true; added; {
		added = false
		for _, meetingID := range meetingIDs {
			bucket := grouped[meetingID]
			if len(bucket) == 0 {
				continue
			}
			ordered = append(ordered, bucket[0])
			grouped[meetingID] = bucket[1:]
			added = true
		}
	}
	return ordered
}

func (b *QMSumBenchmark) createQMSumSummaryService(
	llm model.Model,
) (session.Service, error) {
	dsn := b.cfg.QMSum.PGVectorDSN
	if dsn == "" {
		return nil, fmt.Errorf(
			"pgvector-dsn or PGVECTOR_DSN is required for QMSum summary benchmark",
		)
	}

	embedModelName := b.cfg.QMSum.EmbedModel
	emb := newQMSumEmbeddingEmbedder(embedModelName)
	sum := sessionsummary.NewSummarizer(llm, summaryOptions(b.cfg)...)

	log.Printf(
		"Creating QMSum pgvector session service (embed_model=%s, visible_events=%d, detailed_prompt=%v)",
		embedModelName,
		b.cfg.QMSum.VisibleEvents,
		b.cfg.DetailedPrompt,
	)

	return sessionpgvector.NewService(
		sessionpgvector.WithPostgresClientDSN(dsn),
		sessionpgvector.WithEmbedder(emb),
		sessionpgvector.WithIndexDimension(emb.GetDimensions()),
		sessionpgvector.WithTablePrefix(qmsumTablePrefix),
		sessionpgvector.WithSessionEventLimit(b.cfg.QMSum.VisibleEvents),
		sessionpgvector.WithSyncIndexing(true),
		sessionpgvector.WithMaxResults(10),
		sessionpgvector.WithSummarizer(sum),
		sessionpgvector.WithAsyncSummaryNum(1),
		sessionpgvector.WithSummaryQueueSize(16),
		sessionpgvector.WithSummaryJobTimeout(b.cfg.QMSum.SummaryWait),
	)
}

func (b *QMSumBenchmark) evaluateQMSumCase(
	ctx context.Context,
	llm model.Model,
	judge *qmsumLLMJudge,
	longSvc session.Service,
	summarySvc session.Service,
	qcase *dataset.QMSumCase,
) (*QMSumCaseResult, error) {
	if qcase == nil {
		return nil, fmt.Errorf("QMSum case is nil")
	}

	longResult, err := b.runQMSumMode(
		ctx, llm, judge, longSvc, qcase, qmsumModeLongContext,
	)
	if err != nil {
		return nil, fmt.Errorf("long_context: %w", err)
	}

	summaryResult, err := b.runQMSumMode(
		ctx, llm, judge, summarySvc, qcase, qmsumModeSummary,
	)
	if err != nil {
		return nil, fmt.Errorf("summary: %w", err)
	}

	onDemandResult, err := b.runQMSumMode(
		ctx, llm, judge, summarySvc, qcase, qmsumModeOnDemand,
	)
	if err != nil {
		return nil, fmt.Errorf("summary_ondemand: %w", err)
	}

	supportStart, supportEnd, supportOK := qcase.SupportTurnWindow()
	supportDistance, distanceOK := qcase.SupportDistanceFromEnd()
	if !distanceOK {
		supportDistance = 0
	}

	result := &QMSumCaseResult{
		CaseID:                 qcase.CaseID,
		MeetingID:              qcase.MeetingID,
		Domain:                 qcase.Domain,
		QueryType:              qcase.QueryType,
		Query:                  qcase.Query,
		Reference:              qcase.Answer,
		Turns:                  len(qcase.Transcript),
		Transcript:             transcriptCharCount(qcase.Transcript),
		SupportWindowAvailable: supportOK,
		SupportStartTurn:       supportStart,
		SupportEndTurn:         supportEnd,
		SupportDistanceFromEnd: supportDistance,
		LongContext:            longResult,
		Summary:                summaryResult,
		OnDemand:               onDemandResult,
	}
	if longResult != nil && summaryResult != nil &&
		longResult.TokenUsage != nil && summaryResult.TokenUsage != nil &&
		longResult.TokenUsage.PromptTokens > 0 {
		result.SummaryPromptSavings = 100 * float64(
			longResult.TokenUsage.PromptTokens-summaryResult.TokenUsage.PromptTokens,
		) / float64(longResult.TokenUsage.PromptTokens)
	}
	if longResult != nil && onDemandResult != nil &&
		longResult.TokenUsage != nil && onDemandResult.TokenUsage != nil &&
		longResult.TokenUsage.PromptTokens > 0 {
		result.OnDemandPromptSavings = 100 * float64(
			longResult.TokenUsage.PromptTokens-onDemandResult.TokenUsage.PromptTokens,
		) / float64(longResult.TokenUsage.PromptTokens)
	}
	if summaryResult != nil && onDemandResult != nil &&
		summaryResult.Metrics != nil && onDemandResult.Metrics != nil {
		result.OnDemandROUGELGain = onDemandResult.Metrics.ROUGEL -
			summaryResult.Metrics.ROUGEL
	}
	return result, nil
}

func (b *QMSumBenchmark) runQMSumMode(
	ctx context.Context,
	llm model.Model,
	judge *qmsumLLMJudge,
	svc session.Service,
	qcase *dataset.QMSumCase,
	mode qmsumRunMode,
) (*QMSumModeResult, error) {
	totalStart := time.Now()
	appName := qmsumAppName(mode)
	userID := qcase.CaseID
	sessionID := fmt.Sprintf("%s-%s", string(mode), qcase.CaseID)

	key := session.Key{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	}
	defer func() {
		_ = svc.DeleteSession(context.Background(), key)
	}()

	seedStart := time.Now()
	sess, err := seedQMSumTranscript(ctx, svc, key, qcase)
	seedDurationMs := time.Since(seedStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("seed transcript: %w", err)
	}

	var (
		summaryText            string
		summaryAvailable       bool
		summaryBuildDurationMs int64
	)
	if mode == qmsumModeSummary || mode == qmsumModeOnDemand {
		summaryStart := time.Now()
		sess, summaryText, summaryAvailable, err = b.ensureQMSumSummary(
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

	ag := b.newQMSumAgent(llm, mode)
	r := runner.NewRunner(appName, ag, runner.WithSessionService(svc))
	defer r.Close()

	queryStart := time.Now()
	evtCh, err := r.Run(ctx, userID, sessionID, model.NewUserMessage(qcase.Query))
	if err != nil {
		return &QMSumModeResult{
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
	return &QMSumModeResult{
		Mode:                   string(mode),
		Answer:                 strings.TrimSpace(answer),
		Metrics:                evaluateQMSumMetrics(ctx, judge, qcase.Query, qcase.Answer, strings.TrimSpace(answer)),
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

func (b *QMSumBenchmark) newQMSumAgent(
	llm model.Model,
	mode qmsumRunMode,
) agent.Agent {
	instruction := qmsumInstruction
	if mode == qmsumModeOnDemand {
		instruction += "\n\n" + qmsumOnDemandInstruction
	}

	opts := []llmagent.Option{
		llmagent.WithModel(llm),
		llmagent.WithInstruction(instruction),
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream:      false,
			MaxTokens:   intPtr(b.cfg.QMSum.MaxTokens),
			Temperature: float64Ptr(0),
		}),
	}

	if mode == qmsumModeSummary || mode == qmsumModeOnDemand {
		opts = append(opts,
			llmagent.WithAddSessionSummary(true),
			llmagent.WithMessageBranchFilterMode(llmagent.BranchFilterModeAll),
		)
	}
	if mode == qmsumModeOnDemand {
		opts = append(opts,
			llmagent.WithEnableOnDemandSession(true),
			llmagent.WithMaxToolIterations(b.cfg.QMSum.MaxToolIterations),
		)
	}

	return llmagent.New("qmsum-eval-agent", opts...)
}

func qmsumAppName(mode qmsumRunMode) string {
	switch mode {
	case qmsumModeSummary:
		return qmsumAppSummary
	case qmsumModeOnDemand:
		return qmsumAppOnDemand
	case qmsumModeLongContext:
		fallthrough
	default:
		return qmsumAppLongContext
	}
}

func seedQMSumTranscript(
	ctx context.Context,
	svc session.Service,
	key session.Key,
	qcase *dataset.QMSumCase,
) (*session.Session, error) {
	_ = svc.DeleteSession(ctx, key)
	sess, err := svc.CreateSession(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	baseTime := sess.CreatedAt.UTC().Add(time.Millisecond)
	for i, turn := range qcase.Transcript {
		content := fmt.Sprintf(
			"[Turn %03d] %s: %s",
			i+1,
			strings.TrimSpace(turn.Speaker),
			strings.TrimSpace(turn.Content),
		)
		evt := event.New(
			fmt.Sprintf("%s-%04d", key.SessionID, i),
			"user",
			event.WithResponse(&model.Response{
				Done: true,
				Choices: []model.Choice{
					{
						Message: model.Message{
							Role:    model.RoleUser,
							Content: content,
						},
					},
				},
			}),
		)
		// Keep transcript order stable while ensuring seeded event timestamps are
		// newer than session.CreatedAt, otherwise summary retrieval treats the
		// generated summary as stale and filters it out.
		evt.Timestamp = baseTime.Add(time.Duration(i) * time.Millisecond)
		if err := svc.AppendEvent(ctx, sess, evt); err != nil {
			return nil, fmt.Errorf("append seed event %d: %w", i, err)
		}
	}
	return sess, nil
}

func (b *QMSumBenchmark) ensureQMSumSummary(
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

	text, ok, err := b.waitForQMSumSummary(ctx, svc, key)
	if err != nil {
		return nil, "", false, err
	}
	refreshed, refreshErr := svc.GetSession(ctx, key)
	if refreshErr != nil {
		return nil, "", false, fmt.Errorf("refresh session: %w", refreshErr)
	}
	return refreshed, text, ok, nil
}

func (b *QMSumBenchmark) waitForQMSumSummary(
	ctx context.Context,
	svc session.Service,
	key session.Key,
) (string, bool, error) {
	deadline := time.Now().Add(b.cfg.QMSum.SummaryWait)
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

func transcriptCharCount(turns []dataset.QMSumTranscriptTurn) int {
	total := 0
	for _, turn := range turns {
		total += len(turn.Speaker) + len(turn.Content)
	}
	return total
}

func aggregateQMSumResults(results *QMSumResults) {
	results.LongContext = aggregateQMSumMode(results.Cases, qmsumModeLongContext)
	results.Summary = aggregateQMSumMode(results.Cases, qmsumModeSummary)
	results.OnDemand = aggregateQMSumMode(results.Cases, qmsumModeOnDemand)
	if len(results.Cases) == 0 {
		return
	}

	var totalGain float64
	for _, cr := range results.Cases {
		totalGain += cr.OnDemandROUGELGain
	}
	results.OnDemandROUGELGainAvg = totalGain / float64(len(results.Cases))

	if results.LongContext != nil {
		if results.Summary != nil {
			results.Summary.PromptSavingsVsLong = averagePromptSavings(
				results.Cases,
				func(cr *QMSumCaseResult) float64 { return cr.SummaryPromptSavings },
			)
		}
		if results.OnDemand != nil {
			results.OnDemand.PromptSavingsVsLong = averagePromptSavings(
				results.Cases,
				func(cr *QMSumCaseResult) float64 { return cr.OnDemandPromptSavings },
			)
		}
	}
}

func aggregateQMSumMode(
	cases []*QMSumCaseResult,
	mode qmsumRunMode,
) *QMSumAggregate {
	agg := &QMSumAggregate{}
	var summaryAvailableCount int
	for _, cr := range cases {
		var mr *QMSumModeResult
		switch mode {
		case qmsumModeSummary:
			mr = cr.Summary
		case qmsumModeOnDemand:
			mr = cr.OnDemand
		case qmsumModeLongContext:
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
		agg.AvgSessionSearchCalls += float64(mr.SessionSearchCalls)
		agg.AvgSessionLoadCalls += float64(mr.SessionLoadCalls)
		if mr.SummaryAvailable {
			summaryAvailableCount++
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
	return agg
}

func averagePromptSavings(
	cases []*QMSumCaseResult,
	getter func(*QMSumCaseResult) float64,
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

func printQMSumResults(results *QMSumResults) {
	fmt.Println("\n" + strings.Repeat("=", 72))
	fmt.Println("Summary Evaluation Results (QMSum)")
	fmt.Println(strings.Repeat("=", 72))
	fmt.Printf("Model: %s\n", results.Model)
	fmt.Printf("Cases: %d/%d | Split: %s | Domain: %s | QueryType: %s\n",
		results.NumCases, results.LoadedCases, results.Split, results.Domain, results.QueryType)
	fmt.Printf("Visible Events: %d | Min Support Distance From End: %d\n",
		results.VisibleEvents, results.MinDistanceFromEnd)

	printQMSumAggregate("Long Context", results.LongContext)
	printQMSumAggregate("Summary", results.Summary)
	printQMSumAggregate("Summary On-Demand", results.OnDemand)

	fmt.Printf("\nOn-Demand ROUGE-L Gain vs Summary: %.4f\n", results.OnDemandROUGELGainAvg)
	fmt.Println(strings.Repeat("=", 72))
}

func printQMSumAggregate(title string, agg *QMSumAggregate) {
	if agg == nil || agg.Count == 0 {
		fmt.Printf("\n--- %s ---\n(no successful cases)\n", title)
		return
	}
	fmt.Printf("\n--- %s ---\n", title)
	fmt.Printf("Cases: %d\n", agg.Count)
	fmt.Printf("ROUGE-1/2/L: %.4f / %.4f / %.4f\n",
		agg.AvgROUGE1, agg.AvgROUGE2, agg.AvgROUGEL)
	fmt.Printf("F1/BLEU: %.4f / %.4f\n", agg.AvgF1, agg.AvgBLEU)
	if agg.AvgLLMScore > 0 {
		fmt.Printf("LLM Score: %.4f\n", agg.AvgLLMScore)
	}
	fmt.Printf("Avg Tokens (prompt/completion/total): %.0f / %.0f / %.0f\n",
		agg.AvgPromptTokens, agg.AvgCompletionTokens, agg.AvgTotalTokens)
	fmt.Printf("Avg Latency (total/query): %.0f ms / %.0f ms\n",
		agg.AvgLatencyMs, agg.AvgQueryLatencyMs)
	if agg.AvgSeedDurationMs > 0 || agg.AvgSummaryBuildDurationMs > 0 {
		fmt.Printf("Avg Prep (seed/summary): %.0f ms / %.0f ms\n",
			agg.AvgSeedDurationMs, agg.AvgSummaryBuildDurationMs)
	}
	if agg.AvgSummaryChars > 0 || agg.SummaryAvailableRate > 0 {
		fmt.Printf("Summary Available Rate: %.2f%% | Avg Summary Chars: %.0f\n",
			100*agg.SummaryAvailableRate, agg.AvgSummaryChars)
	}
	if agg.AvgSessionSearchCalls > 0 || agg.AvgSessionLoadCalls > 0 {
		fmt.Printf("Avg Tool Calls (search/load): %.2f / %.2f\n",
			agg.AvgSessionSearchCalls, agg.AvgSessionLoadCalls)
	}
	if agg.PromptSavingsVsLong != 0 {
		fmt.Printf("Prompt Savings vs Long Context: %.2f%%\n", agg.PromptSavingsVsLong)
	}
}

func logQMSumCaseResult(cr *QMSumCaseResult) {
	log.Printf("  Query: %s", truncateStr(cr.Query, 180))
	if cr.SupportWindowAvailable {
		log.Printf("  Support turns: %d-%d | distance_from_end=%d",
			cr.SupportStartTurn, cr.SupportEndTurn, cr.SupportDistanceFromEnd)
	}
	if cr.LongContext != nil && cr.LongContext.Metrics != nil && cr.LongContext.TokenUsage != nil {
		log.Printf("  LongContext       R-L=%.4f p=%d c=%d t=%d total=%dms query=%dms",
			cr.LongContext.Metrics.ROUGEL,
			cr.LongContext.TokenUsage.PromptTokens,
			cr.LongContext.TokenUsage.CompletionTokens,
			cr.LongContext.TokenUsage.TotalTokens,
			cr.LongContext.DurationMs,
			cr.LongContext.QueryDurationMs,
		)
	}
	if cr.Summary != nil && cr.Summary.Metrics != nil && cr.Summary.TokenUsage != nil {
		log.Printf("  Summary           R-L=%.4f p=%d c=%d t=%d saved=%.2f%% summary=%v chars=%d total=%dms query=%dms",
			cr.Summary.Metrics.ROUGEL,
			cr.Summary.TokenUsage.PromptTokens,
			cr.Summary.TokenUsage.CompletionTokens,
			cr.Summary.TokenUsage.TotalTokens,
			cr.SummaryPromptSavings,
			cr.Summary.SummaryAvailable,
			cr.Summary.SummaryChars,
			cr.Summary.DurationMs,
			cr.Summary.QueryDurationMs,
		)
	}
	if cr.OnDemand != nil && cr.OnDemand.Metrics != nil && cr.OnDemand.TokenUsage != nil {
		log.Printf("  Summary+OnDemand  R-L=%.4f p=%d c=%d t=%d saved=%.2f%% gain=%.4f tools=(%d/%d) total=%dms query=%dms",
			cr.OnDemand.Metrics.ROUGEL,
			cr.OnDemand.TokenUsage.PromptTokens,
			cr.OnDemand.TokenUsage.CompletionTokens,
			cr.OnDemand.TokenUsage.TotalTokens,
			cr.OnDemandPromptSavings,
			cr.OnDemandROUGELGain,
			cr.OnDemand.SessionSearchCalls,
			cr.OnDemand.SessionLoadCalls,
			cr.OnDemand.DurationMs,
			cr.OnDemand.QueryDurationMs,
		)
	}
}

func saveQMSumCaseLog(outputDir string, cr *QMSumCaseResult) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("mkdir QMSum output dir: %v", err)
		return
	}

	path := filepath.Join(outputDir, cr.CaseID+".log")
	f, err := os.Create(path)
	if err != nil {
		log.Printf("create QMSum case log: %v", err)
		return
	}
	defer f.Close()

	writeMode := func(title string, mr *QMSumModeResult) {
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
		fmt.Fprintf(f, "Answer:\n%s\n\n", mr.Answer)
	}

	fmt.Fprintf(f, "CaseID: %s\nMeetingID: %s\nDomain: %s\nQueryType: %s\n\n",
		cr.CaseID, cr.MeetingID, cr.Domain, cr.QueryType)
	fmt.Fprintf(f, "Turns: %d\nTranscriptChars: %d\n", cr.Turns, cr.Transcript)
	fmt.Fprintf(f, "SupportWindowAvailable: %v\n", cr.SupportWindowAvailable)
	fmt.Fprintf(f, "SupportTurns: %d-%d\n", cr.SupportStartTurn, cr.SupportEndTurn)
	fmt.Fprintf(f, "SupportDistanceFromEnd: %d\n\n", cr.SupportDistanceFromEnd)
	fmt.Fprintf(f, "Query:\n%s\n\nReference:\n%s\n\n",
		cr.Query, cr.Reference)
	writeMode("LONG CONTEXT", cr.LongContext)
	writeMode("SUMMARY", cr.Summary)
	writeMode("SUMMARY ON-DEMAND", cr.OnDemand)
}

func compactToolTraceText(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" || limit <= 0 {
		return "-"
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func saveQMSumResults(outputDir string, results *QMSumResults) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("mkdir QMSum output dir: %v", err)
		return
	}

	path := filepath.Join(outputDir, "results.json")
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("marshal QMSum results: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("write QMSum results: %v", err)
		return
	}
	log.Printf("Results saved to: %s", path)
}

func saveQMSumCheckpoint(outputDir string, results *QMSumResults) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("mkdir QMSum output dir: %v", err)
		return
	}

	path := filepath.Join(outputDir, "checkpoint.json")
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("marshal QMSum checkpoint: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("write QMSum checkpoint: %v", err)
	}
}

func loadQMSumCheckpoint(outputDir string) *QMSumResults {
	path := filepath.Join(outputDir, "checkpoint.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var results QMSumResults
	if err := json.Unmarshal(data, &results); err != nil {
		log.Printf("parse QMSum checkpoint: %v", err)
		return nil
	}
	return &results
}

func newQMSumEmbeddingEmbedder(modelName string) *embedopenai.Embedder {
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
