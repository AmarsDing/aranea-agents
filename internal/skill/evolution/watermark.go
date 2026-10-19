package evolution

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

// sessionStateKeyLastReviewAt mirrors pkg/trpc-agent-go/evolution (unexported
// there). Must stay identical so restore/persist round-trips with the worker.
const sessionStateKeyLastReviewAt = "evolution:last_review_at"

// WatermarkStore persists lastReviewAt across turns. The framework only writes
// the key into in-memory session state after AppendEvent has already flushed,
// so PG rebuilds drop it every turn without this local bypass.
type WatermarkStore interface {
	Get(sessionID string) (time.Time, bool)
	Set(sessionID string, ts time.Time)
}

// MemoryWatermarkStore is process-local (tests / no-disk).
type MemoryWatermarkStore struct {
	mu   sync.RWMutex
	byID map[string]time.Time
}

// NewMemoryWatermarkStore returns an empty in-memory store.
func NewMemoryWatermarkStore() *MemoryWatermarkStore {
	return &MemoryWatermarkStore{byID: make(map[string]time.Time)}
}

// Get implements WatermarkStore.
func (s *MemoryWatermarkStore) Get(sessionID string) (time.Time, bool) {
	if s == nil || sessionID == "" {
		return time.Time{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ts, ok := s.byID[sessionID]
	return ts, ok
}

// Set implements WatermarkStore.
func (s *MemoryWatermarkStore) Set(sessionID string, ts time.Time) {
	if s == nil || sessionID == "" || ts.IsZero() {
		return
	}
	s.mu.Lock()
	s.byID[sessionID] = ts.UTC()
	s.mu.Unlock()
}

// FileWatermarkStore persists one RFC3339Nano timestamp per session file under Dir.
type FileWatermarkStore struct {
	Dir string
	mem *MemoryWatermarkStore
	lg  loggateway.Logger
}

// NewFileWatermarkStore creates a file-backed store (Dir/watermarks/<id>.ts).
func NewFileWatermarkStore(dir string, lg loggateway.Logger) *FileWatermarkStore {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &FileWatermarkStore{
		Dir: filepath.Join(dir, "watermarks"),
		mem: NewMemoryWatermarkStore(),
		lg:  lg,
	}
}

func (s *FileWatermarkStore) path(sessionID string) string {
	// Keep filenames filesystem-safe; session IDs are usually UUIDs.
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, sessionID)
	return filepath.Join(s.Dir, safe+".ts")
}

// Get implements WatermarkStore.
func (s *FileWatermarkStore) Get(sessionID string) (time.Time, bool) {
	if s == nil || sessionID == "" {
		return time.Time{}, false
	}
	if ts, ok := s.mem.Get(sessionID); ok {
		return ts, true
	}
	raw, err := os.ReadFile(s.path(sessionID))
	if err != nil || len(raw) == 0 {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(raw)))
	if err != nil {
		return time.Time{}, false
	}
	s.mem.Set(sessionID, ts)
	return ts, true
}

// Set implements WatermarkStore.
func (s *FileWatermarkStore) Set(sessionID string, ts time.Time) {
	if s == nil || sessionID == "" || ts.IsZero() {
		return
	}
	s.mem.Set(sessionID, ts)
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		s.lg.Warn("evolution watermark mkdir failed",
			loggateway.StepID("skill.evolution_wm_mkdir"),
			loggateway.Err(err))
		return
	}
	body := []byte(ts.UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(s.path(sessionID), body, 0o644); err != nil {
		s.lg.Warn("evolution watermark write failed",
			loggateway.StepID("skill.evolution_wm_write"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
	}
}

// watermarkPersistEnabled defaults on; set EVOLUTION_WATERMARK_PERSIST=0 to disable.
func watermarkPersistEnabled() bool {
	v := strings.TrimSpace(os.Getenv("EVOLUTION_WATERMARK_PERSIST"))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}
