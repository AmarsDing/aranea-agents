package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func TestGateInboundBeforeTurnDeniesAccess(t *testing.T) {
	repo := &ingressChannelRepo{}
	uc := biz.NewChannelUsecase(repo, nil, nil, nil, nil, biz.NewCredentialCrypto(nil, nil), nil)
	h := &ChannelIngress{
		channels: uc,
		lg:       loggateway.NewNoop(),
	}
	ch := biz.Channel{
		ID:         "ch-1",
		ConfigJSON: `{"type":"wechat","config":{"allowed_user_ids":["other"]},"routing":{"default_agent_id":"a1"}}`,
	}
	ev := port.InboundEvent{
		PlatformType:   "wechat",
		PeerID:         "user-x",
		Text:           "hi",
		IdempotencyKey: "wechat:1",
	}
	proceed, deny, err := h.gateInboundBeforeTurn(context.Background(), ch, ev, true)
	if err != nil || proceed || deny == "" {
		t.Fatalf("proceed=%v deny=%q err=%v", proceed, deny, err)
	}
}

func TestProcessInboundHTTPResultQQDispatchACKOnError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeInboundHTTPResponse(rec, inboundHTTPResult{Err: context.Canceled})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestGraphExecutionSummaryFailed(t *testing.T) {
	failed, msg := graphExecutionSummaryFailed(eventEnvelopeWithSummary(map[string]any{
		"nodes": []any{
			map[string]any{"status": "success"},
			map[string]any{"status": "error", "error": "node boom"},
		},
	}), loggateway.NewNoop())
	if !failed || msg != "node boom" {
		t.Fatalf("failed=%v msg=%q", failed, msg)
	}
}

func eventEnvelopeWithSummary(summary map[string]any) event.Envelope {
	return event.Envelope{Metadata: map[string]any{"execution_summary": summary}}
}
