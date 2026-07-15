package event

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"aranea-agents/internal/biz"
)

// CriticalJournalEntry is one best-effort durable record of a critical event.
// Replay is for reconnect hydrate / debugging — not a full event outbox.
type CriticalJournalEntry struct {
	SessionID  string    `json:"session_id"`
	Kind       string    `json:"kind"`
	EntityID   string    `json:"entity_id,omitempty"`
	TaskID     string    `json:"task_id,omitempty"`
	NoticeType string    `json:"notice_type,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// CriticalJournal appends critical delivery events to per-session JSONL files
// under a data directory (B-06 minimal durable foundation). Best-effort:
// write failures are ignored by callers (Publish must not fail closed on disk).
//
// Nil *CriticalJournal is a no-op (optional wiring).
type CriticalJournal struct {
	dir string
	mu  sync.Mutex
}

// NewCriticalJournal creates a journal rooted at dir. Empty dir disables writes
// (Append becomes a no-op). The directory is created on first successful write.
func NewCriticalJournal(dir string) *CriticalJournal {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return &CriticalJournal{}
	}
	return &CriticalJournal{dir: dir}
}

// DefaultCriticalJournalDir resolves ARANEA_DATA_DIR/critical_events or
// ./data/critical_events when unset.
func DefaultCriticalJournalDir() string {
	base := strings.TrimSpace(os.Getenv("ARANEA_DATA_DIR"))
	if base == "" {
		base = "data"
	}
	return filepath.Join(base, "critical_events")
}

// Append records e when it is a critical delivery event. Safe for concurrent use.
func (j *CriticalJournal) Append(e biz.Event) error {
	if j == nil || j.dir == "" || e == nil || !biz.IsCriticalDeliveryEvent(e) {
		return nil
	}
	sessionID := strings.TrimSpace(e.SpiritSessionID())
	if sessionID == "" {
		return nil
	}
	entry := CriticalJournalEntry{
		SessionID:  sessionID,
		Kind:       string(e.EventKind()),
		EntityID:   e.EntityID(),
		TaskID:     e.TaskID(),
		OccurredAt: e.OccurredAt(),
	}
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = time.Now().UTC()
	}
	if sn, ok := e.(*biz.SystemNoticeEvent); ok {
		entry.NoticeType = sn.NoticeType
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := os.MkdirAll(j.dir, 0o755); err != nil {
		return err
	}
	path := j.sessionPath(sessionID)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// ReplayCritical returns journal entries for sessionID with OccurredAt strictly
// after afterTime (zero = all). Order is file append order.
func (j *CriticalJournal) ReplayCritical(sessionID string, afterTime time.Time) ([]CriticalJournalEntry, error) {
	if j == nil || j.dir == "" {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	path := j.sessionPath(sessionID)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []CriticalJournalEntry
	sc := bufio.NewScanner(f)
	// Critical events are small; allow larger lines if Meta grows later.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var entry CriticalJournalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // best-effort: skip corrupt lines
		}
		if !afterTime.IsZero() && !entry.OccurredAt.After(afterTime) {
			continue
		}
		out = append(out, entry)
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func (j *CriticalJournal) sessionPath(sessionID string) string {
	return filepath.Join(j.dir, sanitizeSessionFile(sessionID)+".jsonl")
}

func sanitizeSessionFile(sessionID string) string {
	var b strings.Builder
	b.Grow(len(sessionID))
	for _, r := range sessionID {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	s := b.String()
	if s == "" {
		return "_empty"
	}
	return s
}
