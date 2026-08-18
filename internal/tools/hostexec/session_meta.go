package hostexec

import (
	"sync"
	"time"
)

const defaultHungAfter = 15 * time.Second

type sessionMeta struct {
	startedAt    time.Time
	lastChangeAt time.Time
	lastOutput   string
}

type sessionMetaStore struct {
	mu    sync.Mutex
	items map[string]*sessionMeta
}

func newSessionMetaStore() *sessionMetaStore {
	return &sessionMetaStore{items: map[string]*sessionMeta{}}
}

func (s *sessionMetaStore) observe(sessionID, output, status string, now time.Time, hungAfter time.Duration) (runningForMS int64, hung bool) {
	if s == nil || sessionID == "" {
		return 0, false
	}
	if hungAfter <= 0 {
		hungAfter = defaultHungAfter
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]*sessionMeta{}
	}
	meta := s.items[sessionID]
	if meta == nil {
		meta = &sessionMeta{startedAt: now, lastChangeAt: now, lastOutput: output}
		s.items[sessionID] = meta
	}
	if output != meta.lastOutput {
		meta.lastOutput = output
		meta.lastChangeAt = now
	}
	runningForMS = now.Sub(meta.startedAt).Milliseconds()
	if runningForMS < 0 {
		runningForMS = 0
	}
	if status == "running" && now.Sub(meta.lastChangeAt) >= hungAfter {
		hung = true
	}
	if status == "exited" {
		delete(s.items, sessionID)
	}
	return runningForMS, hung
}
