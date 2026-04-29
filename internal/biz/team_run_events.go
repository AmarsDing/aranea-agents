package biz

import "sync"

// TeamRunEvent matches legacy conversation/application.TeamRunEvent JSON for SSE clients.
type TeamRunEvent struct {
	Type   string       `json:"type"`
	TeamID string       `json:"team_id"`
	RunID  string       `json:"run_id"`
	Run    *TeamRun      `json:"run,omitempty"`
	Step   *TeamRunStep `json:"step,omitempty"`
}

// TeamRunEventBroker fans out team run notifications to SSE subscribers (filtered by team_id).
type TeamRunEventBroker struct {
	mu          sync.RWMutex
	subscribers map[chan TeamRunEvent]string
}

// NewTeamRunEventBroker constructs an empty broker (singleton per process via wire).
func NewTeamRunEventBroker() *TeamRunEventBroker {
	return &TeamRunEventBroker{subscribers: map[chan TeamRunEvent]string{}}
}

// Subscribe registers a subscriber. filterTeamID empty means receive all teams.
func (b *TeamRunEventBroker) Subscribe(filterTeamID string) (chan TeamRunEvent, func()) {
	ch := make(chan TeamRunEvent, 32)
	b.mu.Lock()
	b.subscribers[ch] = filterTeamID
	b.mu.Unlock()
	unsubscribe := func() {
		b.mu.Lock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
	return ch, unsubscribe
}

// Publish sends an event to matching subscribers (non-blocking per channel).
func (b *TeamRunEventBroker) Publish(event TeamRunEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch, filter := range b.subscribers {
		if filter != "" && filter != event.TeamID {
			continue
		}
		select {
		case ch <- event:
		default:
		}
	}
}
