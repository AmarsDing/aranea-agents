package application

import (
	"sync"

	"arenea/backend/internal/domain"
)

type TeamRunEvent struct {
	Type   string              `json:"type"`
	TeamID string              `json:"team_id"`
	RunID  string              `json:"run_id"`
	Run    *domain.TeamRun     `json:"run,omitempty"`
	Step   *domain.TeamRunStep `json:"step,omitempty"`
}

type TeamRunEventBroker struct {
	mu          sync.RWMutex
	subscribers map[chan TeamRunEvent]string
}

func NewTeamRunEventBroker() *TeamRunEventBroker {
	return &TeamRunEventBroker{subscribers: map[chan TeamRunEvent]string{}}
}

func (b *TeamRunEventBroker) Subscribe(teamID string) (chan TeamRunEvent, func()) {
	ch := make(chan TeamRunEvent, 32)
	b.mu.Lock()
	b.subscribers[ch] = teamID
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

func (b *TeamRunEventBroker) Publish(event TeamRunEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch, teamID := range b.subscribers {
		if teamID != "" && teamID != event.TeamID {
			continue
		}
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *ChatService) SubscribeTeamRunEvents(teamID string) (chan TeamRunEvent, func()) {
	return s.teamRunEvents.Subscribe(teamID)
}

func (s *ChatService) publishTeamRunEvent(event TeamRunEvent) {
	if s.teamRunEvents == nil {
		return
	}
	s.teamRunEvents.Publish(event)
}
