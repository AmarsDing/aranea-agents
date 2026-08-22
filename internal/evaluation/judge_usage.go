package evaluation

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/safego"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

type judgeRunIDKey struct{}

// WithJudgeRunID tags judge calls so usage can be attributed to one eval run.
func WithJudgeRunID(ctx context.Context, runID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if runID == "" {
		return ctx
	}
	return context.WithValue(ctx, judgeRunIDKey{}, runID)
}

func judgeRunIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	s, _ := ctx.Value(judgeRunIDKey{}).(string)
	return s
}

// JudgeCallStats accumulates per-run judge invocation counts.
type JudgeCallStats struct {
	mu    sync.Mutex
	byRun map[string][2]int
}

func (s *JudgeCallStats) add(runID string, prompt, completion int) {
	if s == nil || runID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byRun == nil {
		s.byRun = map[string][2]int{}
	}
	cur := s.byRun[runID]
	cur[0]++
	cur[1] += prompt + completion
	s.byRun[runID] = cur
}

func (s *JudgeCallStats) take(runID string) (calls, tokens int) {
	if s == nil || runID == "" {
		return 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.byRun[runID]
	delete(s.byRun, runID)
	return cur[0], cur[1]
}

type judgeUsageRunner struct {
	inner    runner.Runner
	usage    *biz.UsageUsecase
	stats    *JudgeCallStats
	provider string
	model    string
}

// WrapJudgeUsage records aux_eval_judge usage and per-run counters.
func WrapJudgeUsage(inner runner.Runner, usage *biz.UsageUsecase, provider, model string, stats *JudgeCallStats) runner.Runner {
	if inner == nil {
		return nil
	}
	return &judgeUsageRunner{inner: inner, usage: usage, stats: stats, provider: provider, model: model}
}

func (w *judgeUsageRunner) Close() error {
	if w == nil || w.inner == nil {
		return nil
	}
	return w.inner.Close()
}

func (w *judgeUsageRunner) Run(
	ctx context.Context,
	userID, sessionID string,
	message trpcmodel.Message,
	runOpts ...agent.RunOption,
) (<-chan *event.Event, error) {
	if w == nil || w.inner == nil {
		return nil, nil
	}
	ch, err := w.inner.Run(ctx, userID, sessionID, message, runOpts...)
	if err != nil || ch == nil {
		return ch, err
	}
	out := make(chan *event.Event, 16)
	runID := judgeRunIDFrom(ctx)
	safego.Go(ctx, "eval-judge-usage", func() {
		defer close(out)
		var prompt, completion int
		for ev := range ch {
			if ev != nil && ev.Response != nil && ev.Response.Usage != nil {
				prompt += ev.Response.Usage.PromptTokens
				completion += ev.Response.Usage.CompletionTokens
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				for range ch {
				}
				return
			}
		}
		w.stats.add(runID, prompt, completion)
		if w.usage != nil && (prompt > 0 || completion > 0) {
			_ = w.usage.RecordAuxLLMUsage(ctx, biz.AuxLLMUsageInput{
				Kind:          biz.UsageKindAuxEvalJudge,
				RunID:         runID,
				Provider:      w.provider,
				Model:         w.model,
				PromptTok:     prompt,
				CompletionTok: completion,
				Status:        "ok",
				UsageSource:   biz.UsageSourceResponse,
			})
		}
	})
	return out, nil
}
