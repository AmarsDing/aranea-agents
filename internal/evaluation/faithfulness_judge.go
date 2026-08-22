package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

const faithfulnessJudgePrompt = `You are scoring RAG faithfulness.

Retrieved context:
%s

Assistant answer:
%s

Decide how much of the answer is supported by the retrieved context.
Ignore fluency. Penalize claims that are not grounded in the context.

Reply with EXACTLY two lines and no extra commentary:
score: <number between 0 and 1>
reason: <one sentence>
`

var faithfulnessScoreLine = regexp.MustCompile(`(?i)score\s*[:=]\s*([0-9]*\.?[0-9]+)`)

func judgeFaithfulness(ctx context.Context, judge runner.Runner, output, retrieved string) (float32, error) {
	if judge == nil {
		return 0, fmt.Errorf("judge runner is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ch, err := judge.Run(ctx, "eval", "faithfulness", trpcmodel.NewUserMessage(
		fmt.Sprintf(faithfulnessJudgePrompt, retrieved, output),
	))
	if err != nil {
		return 0, err
	}
	return parseFaithfulnessScore(collectAssistantText(ch))
}

func collectAssistantText(ch <-chan *event.Event) string {
	if ch == nil {
		return ""
	}
	var b strings.Builder
	for ev := range ch {
		if ev == nil || ev.Response == nil {
			continue
		}
		for _, c := range ev.Response.Choices {
			if s := strings.TrimSpace(c.Message.Content); s != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(s)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func parseFaithfulnessScore(raw string) (float32, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty judge response")
	}
	var obj struct {
		Score float64 `json:"score"`
	}
	if json.Unmarshal([]byte(raw), &obj) == nil && obj.Score > 0 {
		return clampFaithfulness(float32(obj.Score)), nil
	}
	if m := faithfulnessScoreLine.FindStringSubmatch(raw); len(m) == 2 {
		v, err := strconv.ParseFloat(m[1], 32)
		if err != nil {
			return 0, err
		}
		return clampFaithfulness(float32(v)), nil
	}
	v, err := strconv.ParseFloat(raw, 32)
	if err != nil {
		return 0, fmt.Errorf("unparseable faithfulness score")
	}
	return clampFaithfulness(float32(v)), nil
}

func clampFaithfulness(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
