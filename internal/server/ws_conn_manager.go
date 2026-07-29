package server

import (
	"net/http"
)

// canAcceptConnection checks whether a new connection is allowed for the given session.
// It returns an HTTP status code (0 means accepted) and a message.
func (s *WSServer) canAcceptConnection(sessionID string, globalMode, probeMode bool) (int, string) {
	if globalMode && !probeMode {
		if s.store.countGlobalMonitorConns() >= s.maxGlobalMonitorConns {
			return http.StatusTooManyRequests, "too many global monitor connections"
		}
	} else if !globalMode {
		if s.store.count(sessionID) >= s.maxSessionConns {
			return http.StatusTooManyRequests, "too many connections for this session"
		}
	}
	return 0, ""
}

// countGlobalMonitorConns returns the number of non-probe global monitor connections.
func (s *WSServer) countGlobalMonitorConns() int {
	return s.store.countGlobalMonitorConns()
}

// CountGlobalMonitorConns exports the global monitor connection count for the
// monitor self-check (WebSocketChecker via monitor.WSConnectionCounter).
func (s *WSServer) CountGlobalMonitorConns() int {
	return s.countGlobalMonitorConns()
}

// removeConn removes a connection from the store.
func (s *WSServer) removeConn(wc *wsConn) {
	s.store.remove(wc)
}
