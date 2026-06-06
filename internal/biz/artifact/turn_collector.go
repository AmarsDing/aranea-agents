package artifact

import (
	"context"
	"sync"
)

type turnCollectorKey struct{}

// TurnCollector accumulates artifact refs produced during a single agent turn.
type TurnCollector struct {
	mu   sync.RWMutex
	refs []Ref
}

// Ref is a lightweight artifact reference for message options_json replay.
type Ref struct {
	ID       string
	Name     string
	MimeType string
	Size     int64
}

// WithTurnCollector attaches a collector to ctx for the duration of an agent turn.
func WithTurnCollector(ctx context.Context) (context.Context, *TurnCollector) {
	c := &TurnCollector{}
	return context.WithValue(ctx, turnCollectorKey{}, c), c
}

// CollectorFromContext returns the turn collector when present.
func CollectorFromContext(ctx context.Context) *TurnCollector {
	v := ctx.Value(turnCollectorKey{})
	if v == nil {
		return nil
	}
	c, _ := v.(*TurnCollector)
	return c
}

// Add records a saved artifact ref.
func (c *TurnCollector) Add(a Artifact) {
	if c == nil || a.ID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refs = append(c.refs, Ref{
		ID:       a.ID,
		Name:     a.Name,
		MimeType: a.MimeType,
		Size:     a.Size,
	})
}

// Refs returns a snapshot of collected refs.
func (c *TurnCollector) Refs() []Ref {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Ref, len(c.refs))
	copy(out, c.refs)
	return out
}
