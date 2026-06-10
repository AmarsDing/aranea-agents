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

type ingressChannelRepo struct {
	streamChannelRepo
	deliveries int
}

func (r *ingressChannelRepo) AddDelivery(_ context.Context, _ biz.ChannelDelivery) (biz.ChannelDelivery, error) {
	r.deliveries++
	return biz.ChannelDelivery{}, nil
}

func TestAcceptInboundReturnsExecuteSync(t *testing.T) {
	repo := &ingressChannelRepo{}
	uc := biz.NewChannelUsecase(repo, repo, repo, repo, nil, nil, nil, nil, biz.NewCredentialCrypto(nil, nil), nil)
	h := &ChannelIngress{
		channels: uc,
		lg:       loggateway.NewNoop(),
	}
	ch := biz.Channel{
		ID:         "ch-1",
		Key:        "feishu_main",
		ConfigJSON: `{"type":"feishu","config":{"ack_message":"收到"},"routing":{"default_agent_id":"agent-1"}}`,
	}
	ev := port.InboundEvent{
		PlatformType:   "feishu",
		PeerID:         "ou_x",
		Text:           "hello",
		IdempotencyKey: "feishu:msg-1",
	}
	outcome, err := h.acceptInbound(context.Background(), ch, ev, true)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.ExecuteSync || outcome.DispatchAsync {
		t.Fatalf("outcome = %+v", outcome)
	}
	if repo.deliveries == 0 {
		t.Fatal("expected ACK delivery enqueue")
	}
}

func TestAcceptInboundDefersAckWhenStreaming(t *testing.T) {
	repo := &ingressChannelRepo{}
	uc := biz.NewChannelUsecase(repo, repo, repo, repo, nil, nil, nil, nil, biz.NewCredentialCrypto(nil, nil), nil)
	h := &ChannelIngress{
		channels: uc,
		lg:       loggateway.NewNoop(),
	}
	ch := biz.Channel{
		ID:         "ch-1",
		ConfigJSON: `{"type":"feishu","config":{"streaming_enabled":true,"ack_message":"收到"},"routing":{"default_agent_id":"agent-1"}}`,
	}
	ev := port.InboundEvent{
		PlatformType:   "feishu",
		PeerID:         "ou_x",
		Text:           "hello",
		IdempotencyKey: "feishu:msg-stream",
	}
	outcome, err := h.acceptInbound(context.Background(), ch, ev, true)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.ExecuteSync {
		t.Fatalf("outcome = %+v", outcome)
	}
	if repo.deliveries != 0 {
		t.Fatalf("streaming feishu should defer ACK to preview, deliveries=%d", repo.deliveries)
	}
}

func TestAcceptInboundReturnsDispatchAsync(t *testing.T) {
	repo := &ingressChannelRepo{}
	uc := biz.NewChannelUsecase(repo, repo, repo, repo, nil, nil, nil, nil, biz.NewCredentialCrypto(nil, nil), nil)
	h := &ChannelIngress{
		channels: uc,
		lg:       loggateway.NewNoop(),
	}
	ch := biz.Channel{
		ID:         "ch-1",
		Key:        "feishu_main",
		ConfigJSON: `{"type":"feishu","config":{"execution_mode":"async","async_graph_id":"g1","ack_message":""},"routing":{"default_agent_id":"agent-1"}}`,
	}
	ev := port.InboundEvent{
		PlatformType:   "feishu",
		PeerID:         "ou_x",
		Text:           "run graph",
		IdempotencyKey: "feishu:msg-2",
	}
	outcome, err := h.acceptInbound(context.Background(), ch, ev, true)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.DispatchAsync || outcome.ExecuteSync {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestAcceptInboundSkipsDuplicateInbound(t *testing.T) {
	repo := &ingressChannelRepo{}
	uc := biz.NewChannelUsecase(repo, repo, repo, repo, nil, nil, nil, nil, biz.NewCredentialCrypto(nil, nil), nil)
	h := &ChannelIngress{
		channels: uc,
		lg:       loggateway.NewNoop(),
	}
	ch := biz.Channel{ID: "ch-1", ConfigJSON: `{"type":"feishu","config":{}}`}
	ev := port.InboundEvent{
		PlatformType:   "feishu",
		PeerID:         "ou_x",
		Text:           "hello",
		IdempotencyKey: "feishu:dup",
	}
	if _, err := h.acceptInbound(context.Background(), ch, ev, true); err != nil {
		t.Fatal(err)
	}
	outcome, err := h.acceptInbound(context.Background(), ch, ev, true)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.needsBackgroundWork() {
		t.Fatalf("duplicate should not schedule work: %+v", outcome)
	}
}

func TestProcessInboundHTTPResponds200(t *testing.T) {
	repo := &ingressChannelRepo{}
	uc := biz.NewChannelUsecase(repo, repo, repo, repo, nil, nil, nil, nil, biz.NewCredentialCrypto(nil, nil), nil)
	h := &ChannelIngress{
		channels: uc,
		lg:       loggateway.NewNoop(),
	}
	ch := biz.Channel{
		ID:         "ch-1",
		ConfigJSON: `{"type":"feishu","config":{"ack_message":""},"routing":{"default_agent_id":"agent-1"}}`,
	}
	ev := port.InboundEvent{
		PlatformType:   "feishu",
		PeerID:         "ou_x",
		Text:           "hello",
		IdempotencyKey: "feishu:http-1",
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/feishu_main", nil)
	writeInboundHTTPResponse(rec, h.processInboundHTTP(req, ch, ev))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}