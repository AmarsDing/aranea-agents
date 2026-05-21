package server

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
)

func TestWSUpstreamPingProducesPong(t *testing.T) {
	srv := NewWSServer(&conf.Server{Ws: &conf.Server_WS{Enable: true}}, event.NewBus(), event.NewBuffer(), nil, nil)
	wc := &wsConn{
		channels: map[string]bool{"system": true},
		send:     make(chan []byte, 4),
	}

	raw, err := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "ping",
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.handleUpstream(wc, raw)

	select {
	case out := <-wc.send:
		var down wsDownstream
		if err := json.Unmarshal(out, &down); err != nil {
			t.Fatal(err)
		}
		if down.Type != "pong" || down.Direction != "server_to_client" {
			t.Fatalf("unexpected downstream: %+v", down)
		}
	default:
		t.Fatal("expected pong on send channel")
	}
}

func TestWSUpstreamSubscribeAddsChannel(t *testing.T) {
	srv := NewWSServer(&conf.Server{Ws: &conf.Server_WS{Enable: true}}, event.NewBus(), event.NewBuffer(), nil, nil)
	wc := &wsConn{
		channels: map[string]bool{"chat": true, "system": true},
		send:     make(chan []byte, 1),
	}

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Type:      "subscribe",
		Payload:   map[string]any{"channel": "monitor"},
	})
	srv.handleUpstream(wc, raw)
	if !wc.channels["monitor"] {
		t.Fatal("expected monitor channel subscribed")
	}
}

func TestWSUpstreamCancelPublishesEnvelope(t *testing.T) {
	bus := event.NewBus()
	srv := NewWSServer(&conf.Server{Ws: &conf.Server_WS{Enable: true}}, bus, event.NewBuffer(), &stubRunCanceller{}, nil)
	wc := &wsConn{
		sessionID: "sess-1",
		channels:  map[string]bool{"system": true},
		send:      make(chan []byte, 1),
	}
	ch, unsub := bus.Subscribe(event.SubscribeOptions{SessionID: "sess-1", BufferSize: 4})
	defer unsub()

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Type:      "cancel",
	})
	srv.handleUpstream(wc, raw)

	select {
	case env := <-ch:
		if env.Type != event.EnvelopeTypeError {
			t.Fatalf("expected error envelope, got %s", env.Type)
		}
		if env.Error == nil || env.Error.Type != "cancelled" {
			t.Fatalf("unexpected error payload: %+v", env.Error)
		}
	default:
		t.Fatal("expected cancel envelope on bus")
	}
}

func TestWSUpstreamUnsubscribeRemovesChannel(t *testing.T) {
	srv := NewWSServer(&conf.Server{Ws: &conf.Server_WS{Enable: true}}, event.NewBus(), event.NewBuffer(), nil, nil)
	wc := &wsConn{
		channels: map[string]bool{"chat": true, "monitor": true, "system": true},
		send:     make(chan []byte, 1),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Type:      "unsubscribe",
		Payload:   map[string]any{"channel": "monitor"},
	})
	srv.handleUpstream(wc, raw)
	if wc.channels["monitor"] {
		t.Fatal("expected monitor channel removed")
	}
	if !wc.channels["chat"] || !wc.channels["system"] {
		t.Fatal("expected other channels kept")
	}
}

func TestWSUpstreamBadDirectionIgnored(t *testing.T) {
	srv := NewWSServer(&conf.Server{Ws: &conf.Server_WS{Enable: true}}, event.NewBus(), event.NewBuffer(), nil, nil)
	wc := &wsConn{
		channels: map[string]bool{"system": true},
		send:     make(chan []byte, 1),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "server_to_client",
		Type:      "ping",
	})
	srv.handleUpstream(wc, raw)
	select {
	case <-wc.send:
		t.Fatal("unexpected downstream for bad direction")
	default:
	}
}

func TestWSUpstreamEnqueueMessageAccepted(t *testing.T) {
	srv := NewWSServer(&conf.Server{Ws: &conf.Server_WS{Enable: true}}, event.NewBus(), event.NewBuffer(), nil, nil)
	wc := &wsConn{
		sessionID: "sess-enq",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 2),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "chat",
		Type:      "enqueue_message",
		Payload: map[string]any{
			"session_id": "sess-enq",
			"content":    "follow up",
		},
	})
	srv.handleUpstream(wc, raw)
	// Should not panic; may or may not send ack depending on gateway wiring.
}

type stubRunCanceller struct{}

func (stubRunCanceller) CancelRun(_ context.Context, _ string) bool {
	return true
}
