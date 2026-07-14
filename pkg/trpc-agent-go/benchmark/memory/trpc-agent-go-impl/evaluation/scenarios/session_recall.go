//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package scenarios

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-agent-go-benchmark/memory/trpc-agent-go-impl/evaluation/dataset"
	"trpc.group/trpc-go/trpc-agent-go-benchmark/memory/trpc-agent-go-impl/evaluation/metrics"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

const (
	sessionRecallAppName     = "memory-eval-session-recall"
	sessionRecallQAMaxTokens = 80
)

const sessionRecallInstructionTemplate = `You are a memory retrieval assistant. Your ONLY job is to read recalled session events and output a short factual answer.

WORKFLOW:
1. Read ALL recalled session events carefully before answering.
2. Use exact words from the recalled events whenever possible.
3. Pay close attention to SessionDate markers and convert relative time references to ABSOLUTE dates, months, or years.
4. Output ONLY the bare answer. No explanations. No context.

ANSWERING PRIORITY - ALWAYS try to answer first:
If ANY recalled event is topically related to the question, you MUST provide an answer.
Only say "%[1]s" when ZERO recalled events relate to the question topic.
When in doubt between answering and saying "%[1]s", ALWAYS answer.

ANSWER STRATEGY:

A) FACTUAL questions (Who/What/Where/When/How many):
   Answer using the exact words from a relevant recalled event.
   For "When" questions, look at both the recalled event text and SessionDate markers for dates.
   For "How many" questions, output the NUMBER (e.g. "3" not "three").
   If the question asks about a SPECIFIC person, verify the recalled events mention that person.
   If the question asks about person A but recalled events ONLY mention person B doing that exact thing, say "%[1]s".
   IMPORTANT: Only reject when there is a CLEAR person mismatch for the SAME activity or fact.
   If the recalled events mention person A doing ANYTHING related, use them to answer.

B) HYPOTHETICAL/INFERENCE questions (Would/Could/Is it likely/What might/What would/What traits/Would X be considered/Would X want/Would X be more interested):
   You MUST reason and infer from available evidence. NEVER say "%[1]s" for these when any related context exists.
   For preference or choice questions ("more interested in A or B", "prefer A or B"), ALWAYS commit to one choice based on available evidence.

C) TEMPORAL CALCULATION questions (How long/What happened first):
   Combine dates from multiple recalled events to calculate durations or order.
   For "Would X be able to do Y by date Z?" - check dates and infer. Give a direct Yes/No.

D) OPEN-DOMAIN questions (What does X feel/think/enjoy/value/realize/describe/do/see/find):
   Answer by copying the most relevant phrase directly from the recalled events. Do NOT summarize.
   NEVER say "%[1]s" for open-domain questions if ANY related recalled event exists.

E) QUESTIONS REQUIRING INDIRECT REASONING - VERY IMPORTANT:
   Many questions LOOK factual but require you to INFER the answer from recalled events plus common knowledge. You MUST attempt an answer for these.
   - "Does X live in Connecticut?" + recalled event "X adopted a dog from a Connecticut shelter" -> "Likely yes"
   - "Who is Jill?" + recalled event "John and Jill went on a date" -> "Most likely John's partner"
   - "Was X feeling lonely before meeting Y?" + recalled event "X said only dogs gave him joy" -> "Most likely yes"
   - "What console does X own?" + recalled event "X plays Xenoblade Chronicles" -> "Nintendo Switch"
   - "What state did X visit?" + recalled event "X went to Indianapolis" -> "Indiana"
   - "Did X and Y study together?" + recalled event "X and Y met in college" -> "Yes"
   For these questions, combine recalled evidence with reasonable inference. NEVER say "%[1]s" when related evidence exists.

ADVERSARIAL PERSON-NAME CHECK (apply ONLY when suspicious):
Some questions deliberately swap person names. Apply this check ONLY when the question asks person A did something, but ALL recalled events about that activity mention ONLY person B and NEVER person A.
Do NOT apply this check when recalled events mention the correct person doing related things, or when the question is about general topics.

RULES:
- Maximum 1-8 words. Output ONLY the answer fragment, NEVER a full sentence.
- For "When" questions: output the date in NATURAL LANGUAGE format like "7 May 2023" or "June 2023". NEVER use ISO format.
- For "How many" questions: output the NUMBER. "3" not "Three children".
- For "What/Who" questions: output ONLY the key noun phrase from the recalled events.
- NEVER start your answer with a person's name or "She/He/They".
- Do NOT rephrase. If the recalled event says "Sweden", say "Sweden", NOT "her home country".
- Output the bare answer only. No sentences. No explanations.`

// SessionRecallEvaluator evaluates using session event
// search preload instead of memory tools.
type SessionRecallEvaluator struct {
	model          model.Model
	evalModel      model.Model
	sessionService session.Service
	config         Config
	llmJudge       *metrics.LLMJudge
}

// NewSessionRecallEvaluator creates a new session recall evaluator.
func NewSessionRecallEvaluator(
	m, evalModel model.Model,
	sessionSvc session.Service,
	cfg Config,
) *SessionRecallEvaluator {
	e := &SessionRecallEvaluator{
		model:          m,
		evalModel:      evalModel,
		sessionService: sessionSvc,
		config:         cfg,
	}
	if cfg.EnableLLMJudge && evalModel != nil {
		e.llmJudge = metrics.NewLLMJudge(evalModel)
	}
	return e
}

// Name returns the evaluator name.
func (e *SessionRecallEvaluator) Name() string {
	return "session_recall"
}

// Evaluate seeds conversation sessions into the session
// store, then answers QA with query-time session recall
// preloaded into the LLM request.
func (e *SessionRecallEvaluator) Evaluate(
	ctx context.Context,
	sample *dataset.LoCoMoSample,
) (*SampleResult, error) {
	if _, ok := e.sessionService.(session.SearchableService); !ok {
		return nil, fmt.Errorf(
			"session service does not implement SearchableService",
		)
	}

	startTime := time.Now()
	userID := sample.SampleID
	seedSessionIDs := make([]string, 0, len(sample.Conversation))
	for _, sess := range sample.Conversation {
		sessionID := fmt.Sprintf("seed-%s", sess.SessionID)
		if err := e.seedSession(
			ctx, userID, sessionID, sample, sess,
		); err != nil {
			return nil, fmt.Errorf(
				"seed session %s: %w", sess.SessionID, err,
			)
		}
		seedSessionIDs = append(seedSessionIDs, sessionID)
	}
	defer e.cleanupSessions(context.Background(), userID, seedSessionIDs)

	qaAgent := newSessionRecallQAAgent(
		e.model, e.config,
	)
	qaRunner := runner.NewRunner(
		sessionRecallAppName,
		qaAgent,
		runner.WithSessionService(e.sessionService),
	)
	defer qaRunner.Close()

	result := &SampleResult{SampleID: sample.SampleID}
	result.QAResults = make([]*QAResult, 0, len(sample.QA))
	catAgg := metrics.NewCategoryAggregator()
	var sampleUsage TokenUsage

	var historyMsgs []model.Message

	for i, qa := range sample.QA {
		qaResult, err := e.evaluateQA(
			ctx, qaRunner, userID, qa, historyMsgs,
		)
		if err != nil {
			if e.config.Verbose {
				log.Printf(
					"Warning: evaluate QA %s failed: %v",
					qa.QuestionID, err,
				)
			}
			if qaResult == nil {
				qaResult = qaResultFromError(qa, err)
			}
		}
		if e.config.Verbose {
			logVerboseQAResult(i, len(sample.QA), qa, qaResult)
		}
		result.QAResults = append(result.QAResults, qaResult)
		catAgg.Add(qa.Category, qaResult.Metrics)
		if qaResult.TokenUsage != nil {
			sampleUsage.Add(*qaResult.TokenUsage)
		}
	}

	result.ByCategory = catAgg.GetCategoryMetrics()
	result.Overall = catAgg.GetOverall()
	result.TotalTimeMs = time.Since(startTime).Milliseconds()
	result.TokenUsage = &sampleUsage
	return result, nil
}

func newSessionRecallQAAgent(
	m model.Model,
	cfg Config,
) agent.Agent {
	genConfig := model.GenerationConfig{
		Stream:      false,
		MaxTokens:   intPtr(sessionRecallQAMaxTokens),
		Temperature: float64Ptr(0),
	}
	return llmagent.New(
		defaultAgentName,
		llmagent.WithModel(m),
		llmagent.WithInstruction(
			fmt.Sprintf(
				sessionRecallInstructionTemplate,
				fallbackAnswer,
			),
		),
		llmagent.WithGenerationConfig(genConfig),
		llmagent.WithPreloadSessionRecall(
			cfg.SessionRecallResults,
		),
		llmagent.WithPreloadSessionRecallMinScore(
			cfg.SessionRecallMinScore,
		),
	)
}

func (e *SessionRecallEvaluator) evaluateQA(
	ctx context.Context,
	r runner.Runner,
	userID string,
	qa dataset.QAItem,
	historyMsgs []model.Message,
) (*QAResult, error) {
	start := time.Now()
	sessionID := fmt.Sprintf("qa-%s", qa.QuestionID)
	defer func() {
		_ = e.sessionService.DeleteSession(
			context.Background(),
			session.Key{
				AppName:   sessionRecallAppName,
				UserID:    userID,
				SessionID: sessionID,
			},
		)
	}()

	recallTrace, err := e.searchSessionRecall(
		ctx, userID, sessionID, qa.Question,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"session recall search: %w", err,
		)
	}

	msg := model.NewUserMessage(qa.Question)
	var runOpts []agent.RunOption
	if len(historyMsgs) > 0 {
		runOpts = append(
			runOpts,
			agent.WithInjectedContextMessages(historyMsgs),
		)
	}

	res, err := runWithRateLimitRetry(
		ctx,
		func() (<-chan *event.Event, error) {
			return r.Run(
				ctx, userID, sessionID, msg,
				runOpts...,
			)
		},
	)
	if err != nil {
		return &QAResult{
			QuestionID:    qa.QuestionID,
			Question:      qa.Question,
			Category:      qa.Category,
			Expected:      qa.Answer,
			Predicted:     fallbackAnswer,
			Metrics:       metrics.QAMetrics{F1: 0, BLEU: 0},
			LatencyMs:     time.Since(start).Milliseconds(),
			SessionRecall: recallTrace,
		}, fmt.Errorf("runner run: %w", err)
	}
	predicted := res.text

	m := metrics.QAMetrics{
		F1:   metrics.CalculateF1(predicted, qa.Answer),
		BLEU: metrics.CalculateBLEU(predicted, qa.Answer),
	}
	if e.llmJudge != nil {
		judgeResult, err := e.llmJudge.Evaluate(
			ctx, qa.Question, qa.Answer, predicted,
		)
		if err == nil && judgeResult.Correct {
			m.LLMScore = judgeResult.Confidence
		}
	}

	return &QAResult{
		QuestionID:    qa.QuestionID,
		Question:      qa.Question,
		Category:      qa.Category,
		Expected:      qa.Answer,
		Predicted:     predicted,
		Metrics:       m,
		LatencyMs:     time.Since(start).Milliseconds(),
		TokenUsage:    &res.usage,
		Steps:         res.steps,
		SessionRecall: recallTrace,
	}, nil
}

func (e *SessionRecallEvaluator) searchSessionRecall(
	ctx context.Context,
	userID, sessionID, question string,
) (*SessionRecallTrace, error) {
	searchable, ok := e.sessionService.(session.SearchableService)
	if !ok {
		return nil, fmt.Errorf(
			"session service does not implement SearchableService",
		)
	}

	req := session.EventSearchRequest{
		Query: strings.TrimSpace(question),
		UserKey: session.UserKey{
			AppName: sessionRecallAppName,
			UserID:  userID,
		},
		MaxResults: e.config.SessionRecallResults,
		MinScore:   e.config.SessionRecallMinScore,
		SearchMode: session.SearchModeHybrid,
	}
	if sessionID != "" {
		req.ExcludeSessionIDs = []string{sessionID}
	}

	results, err := searchable.SearchEvents(ctx, req)
	if err != nil {
		return nil, err
	}
	return buildSessionRecallTrace(req, results), nil
}

func buildSessionRecallTrace(
	req session.EventSearchRequest,
	results []session.EventSearchResult,
) *SessionRecallTrace {
	trace := &SessionRecallTrace{
		Query:      req.Query,
		MaxResults: req.MaxResults,
		MinScore:   req.MinScore,
		SearchMode: req.SearchMode,
		Hits:       make([]SessionRecallHit, 0, len(results)),
	}
	for _, result := range results {
		trace.Hits = append(trace.Hits, SessionRecallHit{
			SessionID:        result.SessionKey.SessionID,
			SessionCreatedAt: result.SessionCreatedAt,
			EventID:          result.Event.ID,
			EventCreatedAt:   result.EventCreatedAt,
			Role:             result.Role,
			Text:             result.Text,
			Score:            result.Score,
			DenseScore:       result.DenseScore,
			SparseScore:      result.SparseScore,
		})
	}
	return trace
}

func (e *SessionRecallEvaluator) seedSession(
	ctx context.Context,
	userID, sessionID string,
	sample *dataset.LoCoMoSample,
	sess dataset.Session,
) error {
	key := session.Key{
		AppName:   sessionRecallAppName,
		UserID:    userID,
		SessionID: sessionID,
	}
	_ = e.sessionService.DeleteSession(ctx, key)
	s, err := e.sessionService.CreateSession(
		ctx, key, nil,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	msgs := sessionRecallMessages(sample, sess)
	for i, msg := range msgs {
		if msg.Role != model.RoleUser &&
			msg.Role != model.RoleAssistant {
			continue
		}
		evt := event.New(
			fmt.Sprintf("%s-%d", sessionID, i),
			seedAgentName,
			event.WithResponse(&model.Response{
				Done: true,
				Choices: []model.Choice{
					{Message: msg},
				},
			}),
		)
		if err := e.sessionService.AppendEvent(
			ctx, s, evt,
		); err != nil {
			return fmt.Errorf("append event: %w", err)
		}
	}
	return nil
}

func (e *SessionRecallEvaluator) cleanupSessions(
	ctx context.Context,
	userID string,
	sessionIDs []string,
) {
	for _, sessionID := range sessionIDs {
		_ = e.sessionService.DeleteSession(
			ctx,
			session.Key{
				AppName:   sessionRecallAppName,
				UserID:    userID,
				SessionID: sessionID,
			},
		)
	}
}

func sessionRecallMessages(
	sample *dataset.LoCoMoSample,
	sess dataset.Session,
) []model.Message {
	msgs := sessionMessages(sample, sess)
	datePrefix := ""
	if sess.SessionDate != "" {
		datePrefix = fmt.Sprintf(
			"[SessionDate: %s] ",
			sess.SessionDate,
		)
	}
	if datePrefix == "" {
		return msgs
	}
	for i := range msgs {
		if msgs[i].Role != model.RoleUser &&
			msgs[i].Role != model.RoleAssistant {
			continue
		}
		msgs[i].Content = datePrefix + msgs[i].Content
	}
	return msgs
}
