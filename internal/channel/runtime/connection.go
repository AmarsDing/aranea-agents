package runtime

import (
	"strings"
	"sync"
	"time"
)

// ConnectionInfo is the in-process WS connector state (F-01b).
type ConnectionInfo struct {
	Connected      bool
	ConnectedSince time.Time
	LastDisconnect time.Time
}

// connectionStore holds channel connection state, scoped to a Manager instance.
type connectionStore struct {
	mu       sync.RWMutex
	byChannel map[string]ConnectionInfo
}

func newConnectionStore() *connectionStore {
	return &connectionStore{byChannel: map[string]ConnectionInfo{}}
}

func (s *connectionStore) set(channelID string, connected bool) {
	channelID = trimSpace(channelID)
	if channelID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.byChannel[channelID]
	now := time.Now()
	if connected {
		if !cur.Connected {
			cur.ConnectedSince = now
		}
		cur.Connected = true
	} else {
		if cur.Connected {
			cur.LastDisconnect = now
		}
		cur.Connected = false
	}
	s.byChannel[channelID] = cur
}

func (s *connectionStore) get(channelID string) ConnectionInfo {
	channelID = trimSpace(channelID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byChannel[channelID]
}

// globalConns is the package-level default store for backward compatibility.
var globalConns = newConnectionStore()

func setChannelConnection(channelID string, connected bool) {
	globalConns.set(channelID, connected)
}

// GetConnectionInfo returns live connector state for a channel instance.
func GetConnectionInfo(channelID string) ConnectionInfo {
	return globalConns.get(channelID)
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != ' ' && c != '\t' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
