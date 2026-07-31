package server

import (
	"encoding/json"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
)

// writePump drains priority queues and writes messages to the WebSocket connection.
func (s *WSServer) writePump(wc *wsConn) {
	cfg := s.wsConfig()
	ticker := time.NewTicker(cfg.PingPeriod)
	bpTicker := time.NewTicker(cfg.BackpressureInterval) // MON-OPT-04 backpressure reporter
	defer func() {
		ticker.Stop()
		bpTicker.Stop()
		wc.conn.Close()
	}()
	for {
		// H-02 + MON-OPT-04: drain priority queues (high → normal → low) before blocking.
		// wc.queues is the sole write path; wc.send is only used for the close signal.
		// M-02: drainSelect drains high/normal greedily and low at most cfg.LowDrainPerLoop
		// times per outer iteration to prevent high/normal starvation.
		for {
			data, prio, ok := wc.queues.drainSelect()
			if !ok {
				break
			}
			wc.conn.SetWriteDeadline(time.Now().Add(cfg.WriteWait))
			if err := wc.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				s.logWSSendFailed(wc, err)
				return
			}
			// After draining a low-priority message, cap further low drains this
			// loop iteration to allow high/normal items to be checked again.
			if prio == wsPriorityLow {
				lowCount := 1
				for lowCount < int(cfg.LowDrainPerLoop) {
					d, p, o := wc.queues.drainSelect()
					if !o {
						goto endDrain
					}
					wc.conn.SetWriteDeadline(time.Now().Add(cfg.WriteWait))
					if err := wc.conn.WriteMessage(websocket.TextMessage, d); err != nil {
						s.logWSSendFailed(wc, err)
						return
					}
					if p == wsPriorityLow {
						lowCount++
					} else {
						// Non-low message found — continue outer loop for high/normal priority.
						break
					}
				}
				break // yield to outer select (ping/bp tickers) after low batch
			}
		}
	endDrain:

		select {
		case _, ok := <-wc.send:
			// wc.send closed = a pump (monitor/activity) signalled a high-queue timeout → close.
			if !ok {
				wc.conn.SetWriteDeadline(time.Now().Add(cfg.WriteWait))
				wc.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
		case <-ticker.C:
			wc.conn.SetWriteDeadline(time.Now().Add(cfg.WriteWait))
			if err := wc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-bpTicker.C:
			// MON-OPT-04: inject backpressure notification if drops occurred.
			if bp := wc.queues.backpressurePayload(int(cfg.BackpressureInterval.Seconds())); bp != nil {
				wc.conn.SetWriteDeadline(time.Now().Add(cfg.WriteWait))
				_ = wc.conn.WriteMessage(websocket.TextMessage, bp)
			}
		}
	}
}

// logWSSendFailed records a writePump data-write failure: process log Error
// plus a throttled-by-nature (connection dies right after) system.ws.send_failed
// flow log.
func (s *WSServer) logWSSendFailed(wc *wsConn, err error) {
	s.lg.With(loggateway.SessionID(wc.sessionID)).Error("WebSocket 发送失败",
		loggateway.StepID("ws.send_failed"), loggateway.Err(err))
	if flow := wc.queues.flowEmitter(); flow != nil {
		flow.LogError("system.ws.send_failed", "WebSocket 消息写入连接失败",
			event.P("error", err.Error()))
	}
}

// readPump reads messages from the WebSocket connection and dispatches them.
func (s *WSServer) readPump(wc *wsConn) {
	cfg := s.wsConfig()
	defer func() {
		// Cancel connection context to stop connection-scoped goroutines
		// (monitorEventPump). Turns are NOT cancelled here —
		// they use appctx.Ctx() and are cancelled via RunRegistry.Cancel.
		s.removeConn(wc)
		wc.close()
		s.lg.Info("WebSocket 连接关闭",
			loggateway.StepID("ws.closed"),
			loggateway.SessionID(wc.sessionID),
			loggateway.Str("mode", wsConnModeLabel(wc.globalMode, wc.probeMode)),
		)
	}()
	wc.conn.SetReadLimit(cfg.ReadLimit)
	wc.conn.SetReadDeadline(time.Now().Add(cfg.PongWait))
	wc.conn.SetPongHandler(func(string) error {
		wc.conn.SetReadDeadline(time.Now().Add(cfg.PongWait))
		return nil
	})
	for {
		_, message, err := wc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.lg.With(loggateway.SessionID(wc.sessionID)).Warn("WebSocket 读错误", loggateway.StepID("ws.read_error"), loggateway.Err(err))
				if flow := wc.queues.flowEmitter(); flow != nil {
					flow.LogError("system.ws.read_error", "WebSocket 连接意外关闭",
						event.P("error", err.Error()))
				}
			}
			return
		}
		s.handleUpstream(wc, message)
	}
}

// monitorEventPump forwards MonitorEvents from the MonitorBus channel to the
// connection's priority queues.
func (s *WSServer) monitorEventPump(wc *wsConn, monitorCh <-chan contract.MonitorEvent) {
	cfg := s.wsConfig()
	for ev := range monitorCh {
		// Monitor events go to the "monitor" channel.
		if !wc.hasChannel("monitor") {
			continue
		}
		if ev.Type == contract.MonitorEventTypeLog && !wc.isLogEnabled() {
			continue
		}
		msg := wsDownstream{
			Direction:    "server_to_client",
			Channel:      "monitor",
			MonitorEvent: &ev,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			s.lg.With(loggateway.SessionID(wc.sessionID)).Warn("WebSocket MonitorEvent 序列化失败，跳过",
				loggateway.StepID("ws.marshal_fail_monitor"),
				loggateway.Err(err),
				loggateway.Any("event_type", ev.Type))
			continue
		}
		prio := wsMonitorEventPriority(ev.Type)
		if ok := wc.queues.enqueue(cfg, prio, data); !ok {
			s.lg.With(loggateway.SessionID(wc.sessionID)).Warn("WebSocket 高优先级队列超时，关闭连接",
				loggateway.StepID("ws.high_queue_timeout"), loggateway.Any("type", ev.Type))
			wc.closeSend()
			return
		}
		wc.wakeWriter()
	}
}
