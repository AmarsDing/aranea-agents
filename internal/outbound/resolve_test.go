package outbound

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func resetSessionResolvers() {
	sessionResolversMu.Lock()
	sessionResolvers = nil
	sessionResolversMu.Unlock()
}

type captureLogger struct {
	msgs *[]string
}

func newCaptureLogger() *captureLogger {
	msgs := make([]string, 0)
	return &captureLogger{msgs: &msgs}
}

func (c *captureLogger) Debug(msg string, _ ...loggateway.Field) { c.record(msg) }
func (c *captureLogger) Info(msg string, _ ...loggateway.Field)  { c.record(msg) }
func (c *captureLogger) Warn(msg string, _ ...loggateway.Field)  { c.record(msg) }
func (c *captureLogger) Error(msg string, _ ...loggateway.Field) { c.record(msg) }
func (c *captureLogger) With(_ ...loggateway.Field) loggateway.Logger {
	return c
}
func (c *captureLogger) record(msg string) { *c.msgs = append(*c.msgs, msg) }

func TestResolveTarget_ExplicitHit(t *testing.T) {
	got, err := ResolveTarget(context.Background(), DeliveryTarget{Channel: " telegram ", Target: " chat-1 "})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if got.Channel != "telegram" || got.Target != "chat-1" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveTarget_MissIsObservableError(t *testing.T) {
	t.Cleanup(resetSessionResolvers)
	resetSessionResolvers()
	_, err := ResolveTarget(context.Background(), DeliveryTarget{})
	if err == nil {
		t.Fatal("unresolved target must return an error, not a silent empty value")
	}
}

func TestResolveTarget_PartialExplicitStillErrors(t *testing.T) {
	t.Cleanup(resetSessionResolvers)
	resetSessionResolvers()
	_, err := ResolveTarget(context.Background(), DeliveryTarget{Channel: "telegram"})
	if err == nil {
		t.Fatal("channel without target must error")
	}
	_, err = ResolveTarget(context.Background(), DeliveryTarget{Target: "chat-1"})
	if err == nil {
		t.Fatal("target without channel must error")
	}
}

func TestRuntimeStateForTarget(t *testing.T) {
	got := RuntimeStateForTarget(DeliveryTarget{Channel: " slack ", Target: " C1 "})
	if got[runtimeStateDeliveryChannel] != "slack" || got[runtimeStateDeliveryTarget] != "C1" {
		t.Fatalf("got %#v", got)
	}
	if RuntimeStateForTarget(DeliveryTarget{Channel: "slack"}) != nil {
		t.Fatal("incomplete target must yield nil runtime state")
	}
}

func TestResolveTargetFromSessionID_NoResolvers(t *testing.T) {
	t.Cleanup(resetSessionResolvers)
	resetSessionResolvers()
	_, ok := ResolveTargetFromSessionID("sess-1")
	if ok {
		t.Fatal("no resolvers must return ok=false (resolver-chain miss)")
	}
}

func TestRegisterSessionResolver_NilIgnored(t *testing.T) {
	t.Cleanup(resetSessionResolvers)
	resetSessionResolvers()
	RegisterSessionResolver(nil)
	_, ok := ResolveTargetFromSessionID("sess-1")
	if ok {
		t.Fatal("nil resolver must not be registered")
	}
}

func TestResolveTargetFromSessionID_Hit(t *testing.T) {
	t.Cleanup(resetSessionResolvers)
	resetSessionResolvers()
	RegisterSessionResolver(func(sessionID string) (DeliveryTarget, bool) {
		if sessionID != "sess-hit" {
			return DeliveryTarget{}, false
		}
		return DeliveryTarget{Channel: "feishu", Target: "ou_1"}, true
	})
	got, ok := ResolveTargetFromSessionID("sess-hit")
	if !ok || got.Channel != "feishu" || got.Target != "ou_1" {
		t.Fatalf("hit: ok=%v got=%+v", ok, got)
	}
	_, ok = ResolveTargetFromSessionID("sess-miss")
	if ok {
		t.Fatal("unrelated session must miss")
	}
}

func TestResolveTarget_FillsFromSessionResolver(t *testing.T) {
	t.Cleanup(resetSessionResolvers)
	resetSessionResolvers()
	RegisterSessionResolver(func(sessionID string) (DeliveryTarget, bool) {
		return DeliveryTarget{Channel: "telegram", Target: "chat-9"}, true
	})
	// Without an invocation in ctx, session fallback is skipped; explicit still works.
	got, err := ResolveTarget(context.Background(), DeliveryTarget{Channel: "telegram", Target: "chat-9"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Channel != "telegram" {
		t.Fatalf("got %+v", got)
	}
}

func TestNewLoggingSessionResolver_LookupErrorIsLoggedAndFalse(t *testing.T) {
	lg := newCaptureLogger()
	fn := NewLoggingSessionResolver(lg, func(sessionID string) (DeliveryTarget, error) {
		return DeliveryTarget{}, errors.New("session store down")
	})
	got, ok := fn("sess-err")
	if ok {
		t.Fatalf("lookup error must be ok=false (chain continues), got %+v", got)
	}
	found := false
	for _, msg := range *lg.msgs {
		if strings.Contains(msg, "outbound session resolver lookup failed") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("lookup failure must be logged, msgs=%v", *lg.msgs)
	}
}

func TestNewLoggingSessionResolver_NoChannelMetaIsSilentFalse(t *testing.T) {
	lg := newCaptureLogger()
	fn := NewLoggingSessionResolver(lg, func(string) (DeliveryTarget, error) {
		return DeliveryTarget{}, nil // Web Chat / no channel metadata
	})
	_, ok := fn("sess-web")
	if ok {
		t.Fatal("no channel meta must be ok=false")
	}
	if len(*lg.msgs) != 0 {
		t.Fatalf("expected no channel-meta is not an error, logged %v", *lg.msgs)
	}
}

func TestNewLoggingSessionResolver_Hit(t *testing.T) {
	fn := NewLoggingSessionResolver(loggateway.NewNoop(), func(string) (DeliveryTarget, error) {
		return DeliveryTarget{Channel: " slack ", Target: " C9 "}, nil
	})
	got, ok := fn("sess")
	if !ok || got.Channel != "slack" || got.Target != "C9" {
		t.Fatalf("ok=%v got=%+v", ok, got)
	}
}

func TestNewLoggingSessionResolver_NilLookup(t *testing.T) {
	fn := NewLoggingSessionResolver(loggateway.NewNoop(), nil)
	_, ok := fn("x")
	if ok {
		t.Fatal("nil lookup must return false")
	}
}

func TestNewLoggingSessionResolver_NilLogger(t *testing.T) {
	fn := NewLoggingSessionResolver(nil, func(string) (DeliveryTarget, error) {
		return DeliveryTarget{}, errors.New("boom")
	})
	_, ok := fn("x")
	if ok {
		t.Fatal("expected false")
	}
}
