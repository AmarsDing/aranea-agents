package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"
)

type Mode string

const (
	ModeFull Mode = "full"
	ModeSafe Mode = "safe"
)

func parseMode(raw string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "full":
		return ModeFull, nil
	case "safe":
		return ModeSafe, nil
	default:
		return ModeFull, fmt.Errorf("unsupported debug recorder mode: %s", raw)
	}
}

type contextKey struct{}

type TraceStart struct {
	AppName   string `json:"app_name,omitempty"`
	Channel   string `json:"channel,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
}

type TraceEnd struct {
	Status   string        `json:"status,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
	Error    string        `json:"error,omitempty"`
}

type Trace struct {
	root      string
	mode      Mode
	startedAt time.Time
	mu        sync.Mutex
	events    *os.File
	closed    bool
}

func (t *Trace) Dir() string  { return t.root }
func (t *Trace) Mode() Mode   { return t.mode }

func (t *Trace) Record(kind string, payload any) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.events == nil {
		return nil
	}
	entry := map[string]any{
		"kind":      kind,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"payload":   payload,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = t.events.Write(data)
	return err
}

func (t *Trace) Close(end TraceEnd) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	if t.events != nil {
		t.events.Close()
		t.events = nil
	}
	end.Duration = time.Since(t.startedAt)
	resultData, _ := json.Marshal(end)
	return os.WriteFile(filepath.Join(t.root, "result.json"), resultData, 0644)
}

func WithTrace(ctx context.Context, t *Trace) context.Context {
	return context.WithValue(ctx, contextKey{}, t)
}

func TraceFromContext(ctx context.Context) *Trace {
	if t, ok := ctx.Value(contextKey{}).(*Trace); ok {
		return t
	}
	return nil
}

type RecorderFactory struct {
	dir  string
	mode Mode
	on   bool
	lg   loggateway.Logger
}

func NewRecorderFactory(c *conf.DebugRecorder, lg loggateway.Logger) *RecorderFactory {
	if c == nil || !c.Enable {
		return &RecorderFactory{lg: lg}
	}
	dir := strings.TrimSpace(c.Dir)
	if dir == "" {
		dir = defaultDebugDir()
	}
	mode, err := parseMode(c.Mode)
	if err != nil {
		lg.Warn("debug_recorder: invalid mode, falling back to full",
			loggateway.Str("mode", c.Mode), loggateway.Err(err))
		mode = ModeFull
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		lg.Error("debug_recorder: failed to create directory",
			loggateway.Str("dir", dir), loggateway.Err(err))
		return &RecorderFactory{lg: lg}
	}
	lg.Info("debug_recorder: enabled",
		loggateway.Str("dir", dir), loggateway.Str("mode", string(mode)))
	return &RecorderFactory{dir: dir, mode: mode, on: true, lg: lg}
}

func (f *RecorderFactory) Enabled() bool {
	return f != nil && f.on
}

func (f *RecorderFactory) StartTrace(ctx context.Context, start TraceStart) (context.Context, *Trace, error) {
	if !f.Enabled() {
		return ctx, nil, nil
	}
	now := time.Now()
	traceDir := filepath.Join(
		f.dir,
		now.Format("20060102"),
		fmt.Sprintf("%s_%s_%s", now.Format("150405"), start.Channel, start.RequestID),
	)
	if err := os.MkdirAll(traceDir, 0755); err != nil {
		f.lg.Warn("debug_recorder: failed to create trace directory",
			loggateway.Err(err))
		return ctx, nil, nil
	}
	eventsFile, err := os.OpenFile(filepath.Join(traceDir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		f.lg.Warn("debug_recorder: failed to create events.jsonl",
			loggateway.Err(err))
		return ctx, nil, nil
	}
	meta := map[string]any{
		"started_at": now.UTC().Format(time.RFC3339Nano),
		"mode":       string(f.mode),
		"start":      start,
	}
	metaData, _ := json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(traceDir, "meta.json"), metaData, 0644)

	trace := &Trace{
		root:      traceDir,
		mode:      f.mode,
		startedAt: now,
		events:    eventsFile,
	}
	return WithTrace(ctx, trace), trace, nil
}

func (f *RecorderFactory) CloseTrace(trace *Trace, end TraceEnd) {
	if trace == nil {
		return
	}
	if err := trace.Close(end); err != nil {
		f.lg.Warn("debug_recorder: failed to close trace",
			loggateway.Err(err))
	}
}

func defaultDebugDir() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "data", "debug")
}
