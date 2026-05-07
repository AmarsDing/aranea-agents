package adksvc

import (
	"encoding/json"
	"iter"
	"maps"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/session"
)

// persistedBundle is JSON-serializable ADK session state for SQLite (sessions.adk_snapshot_json).
type persistedBundle struct {
	AppName       string           `json:"app_name"`
	UserID        string           `json:"user_id"`
	RootAgentName string           `json:"root_agent_name,omitempty"`
	State         map[string]any   `json:"state"`
	Events        []*session.Event `json:"events"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

func bundleFromSession(s *mutableSession) *persistedBundle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev := make([]*session.Event, len(s.events))
	copy(ev, s.events)
	return &persistedBundle{
		AppName:       s.appName,
		UserID:        s.userID,
		RootAgentName: s.RootAgentName,
		State:         maps.Clone(s.state),
		Events:        ev,
		UpdatedAt:     s.updatedAt,
	}
}

func (b *persistedBundle) toMutableSession(sessionID string) *mutableSession {
	st := map[string]any{}
	if b.State != nil {
		st = maps.Clone(b.State)
	}
	ev := b.Events
	if ev == nil {
		ev = []*session.Event{}
	}
	app := strings.TrimSpace(b.AppName)
	if app == "" {
		app = DefaultAppName
	}
	user := strings.TrimSpace(b.UserID)
	if user == "" {
		user = DefaultUserID
	}
	ms := &mutableSession{
		sessionID: sessionID,
		appName:   app,
		userID:    user,
		state:     st,
		events:    ev,
		updatedAt: b.UpdatedAt,
	}
	ms.RootAgentName = strings.TrimSpace(b.RootAgentName)
	return ms
}

func marshalBundle(b *persistedBundle) (string, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalBundle(data string) (*persistedBundle, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return &persistedBundle{State: map[string]any{}, Events: []*session.Event{}}, nil
	}
	var b persistedBundle
	if err := json.Unmarshal([]byte(data), &b); err != nil {
		return nil, err
	}
	if b.State == nil {
		b.State = map[string]any{}
	}
	if b.Events == nil {
		b.Events = []*session.Event{}
	}
	return &b, nil
}

// mutableSession implements session.Session for persistence round-trips.
type mutableSession struct {
	mu sync.RWMutex

	RootAgentName string
	sessionID     string
	appName       string
	userID        string
	state         map[string]any
	events        []*session.Event
	updatedAt     time.Time
}

func newMutableSession(appName, userID, sessionID string) *mutableSession {
	if appName == "" {
		appName = DefaultAppName
	}
	if userID == "" {
		userID = DefaultUserID
	}
	return &mutableSession{
		sessionID: sessionID,
		appName:   appName,
		userID:    userID,
		state:     map[string]any{},
		events:    nil,
		updatedAt: time.Now(),
	}
}

func (s *mutableSession) ID() string                   { return s.sessionID }
func (s *mutableSession) AppName() string              { return s.appName }
func (s *mutableSession) UserID() string               { return s.userID }
func (s *mutableSession) LastUpdateTime() time.Time    { return s.updatedAt }
func (s *mutableSession) State() session.State         { return &mutableState{s: s} }
func (s *mutableSession) Events() session.Events {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := append([]*session.Event(nil), s.events...)
	return eventSlice(cp)
}

type eventSlice []*session.Event

func (e eventSlice) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, ev := range e {
			if !yield(ev) {
				return
			}
		}
	}
}

func (e eventSlice) Len() int { return len(e) }

func (e eventSlice) At(i int) *session.Event {
	if i >= 0 && i < len(e) {
		return e[i]
	}
	return nil
}

type mutableState struct {
	s *mutableSession
}

func (st *mutableState) Get(key string) (any, error) {
	st.s.mu.RLock()
	defer st.s.mu.RUnlock()
	v, ok := st.s.state[key]
	if !ok {
		return nil, session.ErrStateKeyNotExist
	}
	return v, nil
}

func (st *mutableState) Set(key string, val any) error {
	st.s.mu.Lock()
	defer st.s.mu.Unlock()
	if st.s.state == nil {
		st.s.state = map[string]any{}
	}
	st.s.state[key] = val
	return nil
}

func (st *mutableState) All() iter.Seq2[string, any] {
	st.s.mu.RLock()
	cp := maps.Clone(st.s.state)
	st.s.mu.RUnlock()
	return func(yield func(string, any) bool) {
		for k, v := range cp {
			if !yield(k, v) {
				return
			}
		}
	}
}
