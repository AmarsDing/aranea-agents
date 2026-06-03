package event

import (
	"strings"
	"sync"
	"time"
)

// UsageContext holds usage metadata with its own mutex.
type UsageContext struct {
	mu          sync.Mutex
	otelTraceID string
	otelRootID  string
	turnStart   time.Time
}

// NewUsageContext creates a UsageContext with turnStart set to now.
func NewUsageContext() *UsageContext {
	return &UsageContext{
		turnStart: time.Now(),
	}
}

// SetOtelRefs stores OTel trace/span ids for usage metadata correlation.
func (uc *UsageContext) SetOtelRefs(traceID, rootSpanID string) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.otelTraceID = strings.TrimSpace(traceID)
	uc.otelRootID = strings.TrimSpace(rootSpanID)
}

// OtelTraceID returns the stored OTel trace id.
func (uc *UsageContext) OtelTraceID() string {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	return uc.otelTraceID
}

// OtelRootID returns the stored OTel root span id.
func (uc *UsageContext) OtelRootID() string {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	return uc.otelRootID
}

// TurnStart returns the turn start time.
func (uc *UsageContext) TurnStart() time.Time {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	return uc.turnStart
}
