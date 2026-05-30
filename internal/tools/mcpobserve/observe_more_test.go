package mcpobserve

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/event"

	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

type captureBus struct {
	mu   sync.Mutex
	envs []event.Envelope
}

func (b *captureBus) Publish(_ context.Context, env event.Envelope) {
	b.mu.Lock()
	b.envs = append(b.envs, env)
	b.mu.Unlock()
}

func (b *captureBus) Subscribe(_ event.SubscribeOptions) (<-chan event.Envelope, func()) {
	return nil, func() {}
}

func (b *captureBus) DropCount() uint64 { return 0 }

func (b *captureBus) last() event.Envelope {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.envs) == 0 {
		return event.Envelope{}
	}
	return b.envs[len(b.envs)-1]
}

func TestObserverForServer_outcomeCalculation(t *testing.T) {
	tests := []struct {
		name     string
		ev       trpcmcp.ReconnectEvent
		wantOut  string
	}{
		{
			name:    "success",
			ev:      trpcmcp.ReconnectEvent{Success: true, Attempt: 1, MaxAttempts: 3},
			wantOut: "success",
		},
		{
			name:    "failed_still_has_attempts",
			ev:      trpcmcp.ReconnectEvent{Success: false, Attempt: 1, MaxAttempts: 3},
			wantOut: "failed",
		},
		{
			name:    "exhausted_all_attempts",
			ev:      trpcmcp.ReconnectEvent{Success: false, Attempt: 3, MaxAttempts: 3},
			wantOut: "exhausted",
		},
		{
			name:    "exhausted_single_attempt",
			ev:      trpcmcp.ReconnectEvent{Success: false, Attempt: 1, MaxAttempts: 1},
			wantOut: "exhausted",
		},
		{
			name:    "failed_zero_max_attempts",
			ev:      trpcmcp.ReconnectEvent{Success: false, Attempt: 2, MaxAttempts: 0},
			wantOut: "failed",
		},
		{
			name:    "success_zero_max_attempts",
			ev:      trpcmcp.ReconnectEvent{Success: true, Attempt: 1, MaxAttempts: 0},
			wantOut: "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &captureBus{}
			SetBus(bus)
			defer SetBus(nil)

			obs := ObserverForServer("test-server")
			obs(context.Background(), tt.ev)

			env := bus.last()
			got, _ := env.Metadata["outcome"].(string)
			if got != tt.wantOut {
				t.Errorf("outcome = %q, want %q", got, tt.wantOut)
			}
		})
	}
}

func TestObserverForServer_serverNameFallback(t *testing.T) {
	tests := []struct {
		name       string
		serverKey  string
		serverName string
		wantKey    string
	}{
		{
			name:      "uses_server_name_when_present",
			serverKey: "fallback-key",
			serverName: "my-mcp-server",
			wantKey:   "my-mcp-server",
		},
		{
			name:      "falls_back_to_server_key",
			serverKey: "fallback-key",
			serverName: "",
			wantKey:   "fallback-key",
		},
		{
			name:      "falls_back_when_server_name_whitespace",
			serverKey: "fallback-key",
			serverName: "   ",
			wantKey:   "fallback-key",
		},
		{
			name:      "trims_server_key",
			serverKey: "  spaced-key  ",
			serverName: "",
			wantKey:   "spaced-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &captureBus{}
			SetBus(bus)
			defer SetBus(nil)

			obs := ObserverForServer(tt.serverKey)
			obs(context.Background(), trpcmcp.ReconnectEvent{
				Success:    true,
				ServerName: tt.serverName,
			})

			env := bus.last()
			got, _ := env.Metadata["server_key"].(string)
			if got != tt.wantKey {
				t.Errorf("server_key = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestObserverForServer_envelopeMetadata(t *testing.T) {
	bus := &captureBus{}
	SetBus(bus)
	defer SetBus(nil)

	obs := ObserverForServer("srv")
	obs(context.Background(), trpcmcp.ReconnectEvent{
		ServerName:  "my-server",
		Attempt:     2,
		MaxAttempts: 5,
		Success:     false,
		Err:         errors.New("connection reset"),
	})

	env := bus.last()
	if env.Type != event.EnvelopeTypeMCPSessionReconnect {
		t.Errorf("type = %q, want %q", env.Type, event.EnvelopeTypeMCPSessionReconnect)
	}
	if env.Channel != "monitor" {
		t.Errorf("channel = %q, want %q", env.Channel, "monitor")
	}
	if got, _ := env.Metadata["attempt"].(int); got != 2 {
		t.Errorf("attempt = %v, want 2", env.Metadata["attempt"])
	}
	if got, _ := env.Metadata["max_attempts"].(int); got != 5 {
		t.Errorf("max_attempts = %v, want 5", env.Metadata["max_attempts"])
	}
	if got, _ := env.Metadata["success"].(bool); got != false {
		t.Errorf("success = %v, want false", env.Metadata["success"])
	}
	if got, _ := env.Metadata["error"].(string); got != "connection reset" {
		t.Errorf("error = %q, want %q", got, "connection reset")
	}
}

func TestObserverForServer_noErrorFieldWhenNil(t *testing.T) {
	bus := &captureBus{}
	SetBus(bus)
	defer SetBus(nil)

	obs := ObserverForServer("srv")
	obs(context.Background(), trpcmcp.ReconnectEvent{Success: true})

	env := bus.last()
	if _, ok := env.Metadata["error"]; ok {
		t.Error("error key should not be present when Err is nil")
	}
}

func TestObserverForServer_noBusNoPanic(t *testing.T) {
	SetBus(nil)
	obs := ObserverForServer("srv")
	obs(context.Background(), trpcmcp.ReconnectEvent{Success: true})
}

func TestObserverForServer_metadataRecorderCalled(t *testing.T) {
	var recordedKey string
	var recordedAt time.Time
	rec := func(_ context.Context, key string, at time.Time) {
		recordedKey = key
		recordedAt = at
	}
	SetMetadataRecorder(rec)
	defer SetMetadataRecorder(nil)
	SetBus(nil)

	obs := ObserverForServer("rec-srv")
	obs(context.Background(), trpcmcp.ReconnectEvent{Success: true, ServerName: "rec-name"})

	time.Sleep(50 * time.Millisecond)
	if recordedKey != "rec-name" {
		t.Errorf("recordedKey = %q, want %q", recordedKey, "rec-name")
	}
	if recordedAt.IsZero() {
		t.Error("recordedAt should not be zero")
	}
}

func TestSetBus_concurrentSafety(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			SetBus(&captureBus{})
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			SetBus(nil)
		}()
	}
	wg.Wait()
}

func TestSetMetadataRecorder_concurrentSafety(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			SetMetadataRecorder(func(_ context.Context, _ string, _ time.Time) {})
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			SetMetadataRecorder(nil)
		}()
	}
	wg.Wait()
}
