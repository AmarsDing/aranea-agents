package subagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
)

func normalizeLoadedRuns(runs map[string]*runRecord, now time.Time) bool {
	changed := false
	for _, r := range runs {
		if r == nil {
			continue
		}
		if !r.Status.IsTerminal() {
			r.Status = trpcsubagent.StatusFailed
			r.Error = "interrupted"
			r.Summary = truncateRunes("interrupted", defaultStoredSummaryRunes)
			r.UpdatedAt = now
			r.FinishedAt = cloneTime(now)
			changed = true
		}
		if r.CreatedAt.IsZero() {
			r.CreatedAt = now
			changed = true
		}
		if r.UpdatedAt.IsZero() {
			r.UpdatedAt = now
			changed = true
		}
	}
	return changed
}

func loadRuns(path string, lg loggateway.Logger) (map[string]*runRecord, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	runs := make(map[string]*runRecord)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return runs, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return runs, nil
	}
	var sf storeFile
	if err := json.Unmarshal(data, &sf); err != nil {
		lg.Warn("failed to unmarshal subagent store file",
			loggateway.StepID("tool.subagent.load_runs"),
			loggateway.Err(err),
		)
		return nil, err
	}
	for i := range sf.Runs {
		r := sf.Runs[i]
		runs[r.ID] = &r
	}
	return runs, nil
}

func saveRuns(path string, runs map[string]*runRecord) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	sf := storeFile{Version: 1}
	for _, r := range runs {
		if r == nil {
			continue
		}
		sf.Runs = append(sf.Runs, *r)
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func newChildSessionID(runID string, now time.Time) string {
	return fmt.Sprintf("%s%s:%d", subagentSessionPrefix, strings.TrimSpace(runID), now.UnixNano())
}

func newRequestID(runID string, now time.Time) string {
	return fmt.Sprintf("%s%s:%d", subagentRequestPrefix, strings.TrimSpace(runID), now.UnixNano())
}

func sanitizeStoredResult(text string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = defaultStoredResultRunes
	}
	return truncateRunes(text, maxRunes)
}

func summarizeResult(text string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = defaultStoredSummaryRunes
	}
	return truncateRunes(text, maxRunes)
}

func truncateRunes(text string, limit int) string {
	trimmed := strings.TrimSpace(text)
	if limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit])
}

func cloneTime(t time.Time) *time.Time {
	return &t
}

type replyAccumulator struct {
	text     string
	builder  strings.Builder
	seenFull bool
	err      error
}

func (a *replyAccumulator) consume(evt *trpcevent.Event) {
	if evt == nil {
		return
	}
	if evt.Error != nil {
		a.err = apierror.Internal(apierror.DomainTool, evt.Error.Message)
		return
	}
	if evt.Response == nil {
		return
	}
	switch evt.Object {
	case trpcmodel.ObjectTypeChatCompletion:
		a.consumeFull(evt.Response)
	case trpcmodel.ObjectTypeChatCompletionChunk:
		a.consumeDelta(evt.Response)
	}
}

func (a *replyAccumulator) consumeFull(rsp *trpcmodel.Response) {
	if rsp == nil || len(rsp.Choices) == 0 {
		return
	}
	content := rsp.Choices[0].Message.Content
	if content == "" {
		return
	}
	a.text = content
	a.seenFull = true
}

func (a *replyAccumulator) consumeDelta(rsp *trpcmodel.Response) {
	if rsp == nil || a.seenFull {
		return
	}
	for _, choice := range rsp.Choices {
		if choice.Delta.Content == "" {
			continue
		}
		a.builder.WriteString(choice.Delta.Content)
	}
	a.text = a.builder.String()
}
