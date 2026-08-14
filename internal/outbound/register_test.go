package outbound

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestRegisterFromInboundEvent(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	RegisterFromInboundEvent(r, "telegram", &stubOutboundText{id: "telegram"})
	ch := r.Channels()
	if len(ch) != 1 || ch[0] != "telegram" {
		t.Fatalf("expected [telegram], got %v", ch)
	}
}

func TestRegisterFromInboundEvent_NilIgnored(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	RegisterFromInboundEvent(nil, "telegram", &stubOutboundText{id: "telegram"})
	RegisterFromInboundEvent(r, "telegram", nil)
	if len(r.Channels()) != 0 {
		t.Fatal("nil router/sender must be ignored")
	}
}
