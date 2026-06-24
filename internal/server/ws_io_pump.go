package server

import (
	"encoding/json"
	"time"

	"aranea-agents/internal/event"
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
			// wc.send closed = eventPump signalled a high-queue timeout → close.
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

// readPump reads messages from the WebSocket connection and dispatches them.
func (s *WSServer) readPump(wc *wsConn) {
	cfg := s.wsConfig()
	defer func() {
		// Cancel connection context to stop connection-scoped goroutines
		// (eventPump, replay, sync_request). Turns are NOT cancelled here —
		// they use appctx.Ctx() and are cancelled via RunRegistry.Cancel.
		s.removeConn(wc)
		wc.close()
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
			}
			return
		}
		s.handleUpstream(wc, message)
	}
}

// eventPump forwards events from the event bus channel to the connection's priority queues.
func (s *WSServer) eventPump(wc *wsConn, eventCh <-chan event.Envelope) {
	cfg := s.wsConfig()
	if wc.replayDone != nil {
		<-wc.replayDone
	}
	for env := range eventCh {
		if env.Type == event.EnvelopeTypeError {
			s.lg.With(loggateway.SessionID(wc.sessionID)).Info("eventPump received error envelope",
				loggateway.StepID("ws.eventPump_error"),
				loggateway.Any("channel", env.Channel),
				loggateway.Any("envelope_id", env.ID),
				loggateway.Any("hasChannel", wc.hasChannel(env.Channel)))
		}
		if !wc.hasChannel(env.Channel) {
			continue
		}
		if env.Type == event.EnvelopeTypeLog && !wc.isLogEnabled() {
			continue
		}
		// flow_log is always delivered on monitor channel (no enable_log gate).
		if fk := wc.getFilterKey(); fk != "" && !event.MatchFilterKey(fk, env.FilterKey) {
			continue
		}
		msg := wsDownstream{
			Direction: "server_to_client",
			Channel:   env.Channel,
			Envelope:  &env,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			s.lg.With(loggateway.SessionID(wc.sessionID)).Warn("WebSocket 下行消息序列化失败，跳过",
				loggateway.StepID("ws.marshal_fail"), loggateway.Err(err), loggateway.Any("envelope_type", env.Type))
			continue
		}
		// MON-OPT-04: route to priority queue; close connection on high-queue timeout.
		prio := wsEnvelopePriority(env.Type)
		if ok := wc.queues.enqueue(cfg, prio, data); !ok {
			s.lg.With(loggateway.SessionID(wc.sessionID)).Warn("WebSocket 高优先级队列超时，关闭连接",
				loggateway.StepID("ws.high_queue_timeout"), loggateway.Any("type", env.Type))
			wc.closeSend() // signal writePump to exit (safe: sync.Once protects)
			return
		}
		wc.wakeWriter()
	}
}
