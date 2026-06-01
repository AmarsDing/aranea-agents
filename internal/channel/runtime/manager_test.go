package runtime_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"
)

type stubHandler struct {
	called int
}

func (s *stubHandler) ProcessInbound(ctx context.Context, ch biz.Channel, ev port.InboundEvent) error {
	s.called++
	return nil
}

func TestNeedsRuntimeConnector(t *testing.T) {
	ch := biz.Channel{Enabled: true, ConfigJSON: `{"type":"feishu","receive_mode":"websocket"}`}
	if !runtime.NeedsRuntimeConnector(ch, loggateway.NewNoop()) {
		t.Fatal("feishu websocket should need runtime")
	}
	ch.ConfigJSON = `{"type":"feishu","receive_mode":"webhook"}`
	if runtime.NeedsRuntimeConnector(ch, loggateway.NewNoop()) {
		t.Fatal("feishu webhook should not need runtime")
	}
}

func TestRegisterStarter(t *testing.T) {
	runtime.RegisterStarter("testplat", "polling", func(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup runtime.CredentialLookup, handler port.InboundHandler) error {
		return ctx.Err()
	})
	ch := biz.Channel{
		ID:         "ch-1",
		Enabled:    true,
		ConfigJSON: `{"type":"testplat","receive_mode":"polling"}`,
	}
	handler := &stubHandler{}
	mgr := runtime.NewManager(nil, handler, func(ctx context.Context, creds []biz.ChannelCredential, key string) (string, error) {
		return "x", nil
	}, loggateway.NewNoop())
	// Manager with nil channels skips Reload safely
	if err := mgr.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = ch
}
