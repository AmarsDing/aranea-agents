package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/memory"
	"aranea-agents/pkg/loggateway"
)

const (
	memoryEbbinghausDecayDefaultInterval = 24 * time.Hour
)

// MemoryEbbinghausDecayWorker periodically computes Ebbinghaus exponential
// decay statistics (R_t = exp(-n_t / S_t)) for stored memories.
//
// This worker is a statistics-only companion to MemoryL3DecayWorker. It does
// NOT mutate the database — the existing L3 decay worker already owns
// importance updates. Instead, this worker:
//   - periodically scans memories (when a reader is wired),
//   - computes Ebbinghaus reachability R_t for each,
//   - logs how many memories have decayed below a threshold,
//   - provides the decay data foundation for future active-recall.
//
// When no memory reader is wired (current skeleton state), the worker only
// logs startup and ticks at the configured interval, ready for future
// enhancement once a memory-listing Repo port is introduced.
type MemoryEbbinghausDecayWorker struct {
	interval   time.Duration
	calculator *memory.EbbinghausDecayCalculator
	lg         loggateway.Logger
}

// NewMemoryEbbinghausDecayWorker creates a MemoryEbbinghausDecayWorker.
//
// Parameters:
//   - interval:   tick interval. Falls back to 24h when <= 0.
//   - calculator: the Ebbinghaus decay calculator. When nil, a default one is created.
//   - lg:         logger. Falls back to a no-op logger when nil.
func NewMemoryEbbinghausDecayWorker(interval time.Duration, calculator *memory.EbbinghausDecayCalculator, lg loggateway.Logger) *MemoryEbbinghausDecayWorker {
	if interval <= 0 {
		interval = memoryEbbinghausDecayDefaultInterval
	}
	if calculator == nil {
		calculator = memory.NewEbbinghausDecayCalculator()
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MemoryEbbinghausDecayWorker{
		interval:   interval,
		calculator: calculator,
		lg:         lg,
	}
}

// Start blocks until ctx is cancelled. It logs startup, then ticks at the
// configured interval. Each tick is a no-op in the current skeleton — future
// enhancements will scan memories and compute decay statistics here.
func (w *MemoryEbbinghausDecayWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.lg.Info("ebbinghaus decay worker started",
		loggateway.Any("interval", w.interval.String()))

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.lg.Info("ebbinghaus decay worker stopped")
			return
		case <-ticker.C:
			// Skeleton tick: no database scan yet. The Ebbinghaus decay
			// calculator is available via w.calculator for future use once
			// a memory-listing Repo port is wired.
			w.lg.Debug("ebbinghaus decay tick (skeleton, no-op)")
		}
	}
}

// MemoryEbbinghausDecayDisabled reports whether the Ebbinghaus decay worker
// is disabled via the MEMORY_EBBINGHAUS_DECAY_DISABLED environment variable.
func MemoryEbbinghausDecayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_EBBINGHAUS_DECAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
