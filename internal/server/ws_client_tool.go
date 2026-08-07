package server

// ws_client_tool.go — client tool bridge WS integration (design 74 §6.2).
//
// Downstream: client_tool.invoke frames are fanned out to session connections
// that advertised the desktop_companion capability (register_capabilities).
// Upstream: client_tool.result resolves the pending bridge invocation.
// The bridge itself (pending registry / timeout / audit / flow logs) lives in
// internal/tools/clientbridge; this file is only the transport adapter.

import (
	"encoding/json"

	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/pkg/loggateway"
)

// CapabilityDesktopCompanion marks a WS connection backed by the Tauri desktop
// companion that can execute client tools (open_app/open_url).
const CapabilityDesktopCompanion = "desktop_companion"

// SetClientToolBridge wires the client tool bridge and installs this server as
// its downstream router. Nil bridge is ignored (client tools stay disabled).
func (s *WSServer) SetClientToolBridge(b *clientbridge.Bridge) {
	if s == nil || b == nil {
		return
	}
	s.clientBridge = b
	b.SetRouter(s)
}

// RouteClientToolInvoke implements clientbridge.Router: fan the invoke frame
// out to every session connection that advertised the desktop_companion
// capability. Returns true when at least one connection accepted the frame.
// Session-scoped connections are already ownership-checked at subscribe time
// (SessionAuthorizer), so session fan-out implies same-user delivery.
func (s *WSServer) RouteClientToolInvoke(sessionID string, msg clientbridge.InvokeMessage) bool {
	if s == nil || sessionID == "" {
		return false
	}
	payload := map[string]any{
		"invocation_id": msg.InvocationID,
		"session_id":    msg.SessionID,
		"tool":          msg.Tool,
	}
	if len(msg.Args) > 0 {
		// Forward the tool arguments verbatim (raw JSON object).
		payload["args"] = json.RawMessage(msg.Args)
	}
	frame := wsDownstream{
		Direction: "server_to_client",
		Channel:   "system",
		Type:      "client_tool.invoke",
		Payload:   payload,
	}
	delivered := false
	s.store.forEachConnForSession(sessionID, func(wc *wsConn) {
		if !wc.hasCapability(CapabilityDesktopCompanion) {
			return
		}
		wc.sendSystemDownstream(frame)
		delivered = true
	})
	return delivered
}

// handleRegisterCapabilities processes a register_capabilities uplink: the
// desktop companion announces which client capabilities this connection
// provides (currently: desktop_companion).
func (s *WSServer) handleRegisterCapabilities(wc *wsConn, up wsUpstream) {
	payload, ok := up.Payload.(map[string]any)
	if !ok {
		return
	}
	rawCaps, ok := payload["capabilities"].([]any)
	if !ok {
		return
	}
	caps := make([]string, 0, len(rawCaps))
	for _, c := range rawCaps {
		if name, ok := c.(string); ok && name != "" {
			caps = append(caps, name)
		}
	}
	wc.setCapabilities(caps)
	s.lg.Info("WS client capabilities registered",
		loggateway.StepID("ws.client_capabilities"),
		loggateway.SessionID(wc.sessionID),
		loggateway.Any("capabilities", caps))
}

// handleClientToolResult processes a client_tool.result uplink and resolves
// the pending bridge invocation. Unknown invocation IDs (late/duplicate
// frames after timeout) are dropped with a Warn.
func (s *WSServer) handleClientToolResult(wc *wsConn, up wsUpstream) {
	if s.clientBridge == nil {
		return
	}
	payload, ok := up.Payload.(map[string]any)
	if !ok {
		return
	}
	invocationID, _ := payload["invocation_id"].(string)
	if invocationID == "" {
		return
	}
	okFlag, _ := payload["ok"].(bool)
	output, _ := payload["output"].(string)
	errText, _ := payload["error"].(string)
	if !s.clientBridge.ResolveResult(invocationID, okFlag, output, errText) {
		s.lg.Warn("client_tool.result dropped: unknown or stale invocation",
			loggateway.StepID("ws.client_tool_result_drop"),
			loggateway.SessionID(wc.sessionID),
			loggateway.Str("invocation_id", invocationID))
	}
}
