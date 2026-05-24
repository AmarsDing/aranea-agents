package runtime

import (
	"sync"
	"time"
)

// ConnectionInfo is the in-process WS connector state (F-01b).
type ConnectionInfo struct {
	Connected       bool
	ConnectedSince  time.Time
	LastDisconnect  time.Time
}

var connMu sync.RWMutex
var connByChannel = map[string]ConnectionInfo{}

func setChannelConnection(channelID string, connected bool) {
	channelID = trimSpace(channelID)
	if channelID == "" {
		return
	}
	connMu.Lock()
	defer connMu.Unlock()
	cur := connByChannel[channelID]
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
	connByChannel[channelID] = cur
}

// GetConnectionInfo returns live connector state for a channel instance.
func GetConnectionInfo(channelID string) ConnectionInfo {
	channelID = trimSpace(channelID)
	connMu.RLock()
	defer connMu.RUnlock()
	return connByChannel[channelID]
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
