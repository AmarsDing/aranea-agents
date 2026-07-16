package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/loggateway"
)

func TestGateInboundBeforeTurnDeniesAccess(t *testing.T) {
	repo := &ingressChannelRepo{}
	uc := testIngressChannelUsecase(repo)
	dedupe := biz.NewIngressMessageDedupe(biz.DefaultMessageDedupeTTL)
	h := &ChannelIngress{
		channels:     uc,
		lg:           loggateway.NewNoop(),
		deduplicator: dedupe,
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

func TestGraphExecutionSummaryFailedFromNotice(t *testing.T) {
	notice := biz.NewSystemNoticeEvent("sess", "execution_done", "", map[string]any{
		"execution_summary": map[string]any{
			"nodes": []any{
				map[string]any{"status": "success"},
				map[string]any{"status": "error", "error": "node boom"},
			},
		},
	})
	failed, msg := graphExecutionSummaryFailedFromNotice(notice, loggateway.NewNoop())
	if !failed || msg != "node boom" {
		t.Fatalf("failed=%v msg=%q", failed, msg)
	}
}
