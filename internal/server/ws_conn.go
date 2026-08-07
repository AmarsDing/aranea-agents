package server

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// wsConn represents a single WebSocket connection.
type wsConn struct {
	conn        *websocket.Conn
	sessionID   string
	userID      string
	channels    map[string]bool
	filterKey   string
	unsubscribe func()
	// send is the legacy/system channel (normal priority). eventPump now routes
	// through queues; non-event callers (sendSystemDownstream) still use send.
	// MON-OPT-04: send is kept for backward compat; writePump drains queues first.
	send       chan []byte
	queues     *connQueues // MON-OPT-04 priority lanes
	logEnabled bool
	globalMode bool
	probeMode  bool
	// connCtx is cancelled when this WebSocket connection closes. It governs
	// connection-scoped goroutines (readPump/writePump/eventPump) and short-
	// lived handlers. Turn execution no longer derives from connCtx — turns use
	// appctx.Ctx() and are cancelled via RunRegistry.Cancel (No-Timeout
	// principle, 2026-06-18). Phase 5 Blocker A: the replayDone channel has
	// been removed along with the WS replay path; clients fetch history via
	// ListActivities RPC on reconnect.
	connCtx    context.Context
	connCancel context.CancelFunc
	// stateMu protects channels, logEnabled, filterKey, and capabilities from
	// concurrent read/write between readPump (writes via handleUpstream) and
	// eventPump (reads).
	stateMu sync.RWMutex
	// capabilities is the client-advertised capability set (register_capabilities
	// uplink), e.g. desktop_companion for the Tauri desktop companion that can
	// execute client tools (design 74 §6.2). Nil for plain web clients.
	capabilities map[string]bool
	// closeOnce ensures wc.send is closed at most once, preventing double-close panic
	// when multiple eventPump goroutines might race on high-queue timeout.
	closeOnce sync.Once
}

// close performs all connection cleanup: cancel context, unsubscribe from event bus,
// and close the underlying WebSocket connection. It is safe to call multiple times.
// The caller is still responsible for removing the connection from the store (s.removeConn).
func (wc *wsConn) close() {
	wc.connCancel()
	if wc.unsubscribe != nil {
		wc.unsubscribe()
	}
	wc.conn.Close()
}

// sendSystemDownstream marshals and enqueues a system-level downstream message.
func (wc *wsConn) sendSystemDownstream(msg wsDownstream) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	// H-02: route through queues (normal priority) so writePump has a single drain path.
	wc.queues.enqueueSystem(data)
	wc.wakeWriter()
}

// contextOrBackground returns the connection context or context.Background.
func (wc *wsConn) contextOrBackground() context.Context {
	if wc != nil && wc.connCtx != nil {
		return wc.connCtx
	}
	return context.Background()
}

// sendSystemRaw enqueues raw bytes as a normal-priority system message.
func (wc *wsConn) sendSystemRaw(data []byte) {
	// H-02: route through queues (normal priority).
	wc.queues.enqueueSystem(data)
	wc.wakeWriter()
}

// wakeWriter sends a nil message to the send channel to wake the writePump.
func (wc *wsConn) wakeWriter() {
	if wc == nil || wc.send == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	select {
	case wc.send <- nil:
	default:
	}
}

// hasChannel reports whether the connection is subscribed to the given channel.
func (wc *wsConn) hasChannel(ch string) bool {
	wc.stateMu.RLock()
	defer wc.stateMu.RUnlock()
	return wc.channels[ch]
}

// subscribedChannels returns a snapshot of all subscribed channel names.
func (wc *wsConn) subscribedChannels() []string {
	wc.stateMu.RLock()
	defer wc.stateMu.RUnlock()
	result := make([]string, 0, len(wc.channels))
	for ch := range wc.channels {
		result = append(result, ch)
	}
	return result
}

// setChannel adds or removes a channel subscription.
func (wc *wsConn) setChannel(ch string, subscribed bool) {
	wc.stateMu.Lock()
	defer wc.stateMu.Unlock()
	if subscribed {
		wc.channels[ch] = true
	} else {
		delete(wc.channels, ch)
	}
}

// isLogEnabled reports whether log events should be delivered.
func (wc *wsConn) isLogEnabled() bool {
	wc.stateMu.RLock()
	defer wc.stateMu.RUnlock()
	return wc.logEnabled
}

// setLogEnabled updates the logEnabled flag.
func (wc *wsConn) setLogEnabled(enabled bool) {
	wc.stateMu.Lock()
	defer wc.stateMu.Unlock()
	wc.logEnabled = enabled
}

// setCapabilities replaces the connection's advertised capability set.
func (wc *wsConn) setCapabilities(caps []string) {
	wc.stateMu.Lock()
	defer wc.stateMu.Unlock()
	wc.capabilities = make(map[string]bool, len(caps))
	for _, c := range caps {
		wc.capabilities[c] = true
	}
}

// hasCapability reports whether the connection advertised the given capability.
func (wc *wsConn) hasCapability(cap string) bool {
	wc.stateMu.RLock()
	defer wc.stateMu.RUnlock()
	return wc.capabilities[cap]
}

// getFilterKey returns the current filter key.
func (wc *wsConn) getFilterKey() string {
	wc.stateMu.RLock()
	defer wc.stateMu.RUnlock()
	return wc.filterKey
}

// setFilterKey updates the filter key.
func (wc *wsConn) setFilterKey(key string) {
	wc.stateMu.Lock()
	defer wc.stateMu.Unlock()
	wc.filterKey = key
}

// closeSend safely closes the send channel at most once.
func (wc *wsConn) closeSend() {
	wc.closeOnce.Do(func() {
		close(wc.send)
	})
}

// wsBuildChannels constructs the initial channel subscription map based on connection mode.
func wsBuildChannels(globalMode, probeMode bool) map[string]bool {
	channels := map[string]bool{
		"chat":   true,
		"system": true,
	}
	if globalMode && !probeMode {
		channels["monitor"] = true
		channels["team"] = true
		channels["graph"] = true
		channels["knowledge"] = true
	}
	return channels
}

// connStore holds the connection map with its mutex.
// Extracted from WSServer to isolate connection lifecycle management.
type connStore struct {
	mu    sync.RWMutex
	conns map[string][]*wsConn
}

func newConnStore() *connStore {
	return &connStore{
		conns: make(map[string][]*wsConn),
	}
}

func (cs *connStore) add(sessionID string, wc *wsConn) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.conns[sessionID] = append(cs.conns[sessionID], wc)
}

func (cs *connStore) remove(wc *wsConn) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	conns := cs.conns[wc.sessionID]
	for i, c := range conns {
		if c == wc {
			cs.conns[wc.sessionID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(cs.conns[wc.sessionID]) == 0 {
		delete(cs.conns, wc.sessionID)
	}
}

func (cs *connStore) count(sessionID string) int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.conns[sessionID])
}

func (cs *connStore) countGlobalMonitorConns() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	conns := cs.conns["*"]
	n := 0
	for _, wc := range conns {
		if wc != nil && !wc.probeMode {
			n++
		}
	}
	return n
}

func (cs *connStore) forEachConn(fn func(wc *wsConn)) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, conns := range cs.conns {
		for _, wc := range conns {
			fn(wc)
		}
	}
}

// forEachConnForSession iterates over connections subscribed to a specific
// session ID, and also fans out to global (session_id=*) subscribers.
// Used by WSV2Subscriber so admin/session-list clients receive background
// turn completions (E2E-P1-06).
func (cs *connStore) forEachConnForSession(sessionID string, fn func(wc *wsConn)) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, wc := range cs.conns[sessionID] {
		fn(wc)
	}
	if sessionID == "*" {
		return
	}
	for _, wc := range cs.conns["*"] {
		if wc != nil && !wc.probeMode {
			fn(wc)
		}
	}
}
