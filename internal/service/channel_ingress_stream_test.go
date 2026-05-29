package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/port"
)

func TestProcessInboundStreamingUnsupportedPlatformUnaryFallback(t *testing.T) {
	repo := &streamChannelRepo{}
	uc := biz.NewChannelUsecase(repo, biz.NewCredentialCrypto(nil))
	h := &ChannelIngress{channels: uc}
	ch := biz.Channel{ID: "ch1", ConfigJSON: `{"type":"dingtalk","config":{"streaming_enabled":true}}`}
	ev := port.InboundEvent{PlatformType: "dingtalk", PeerID: "p1", Text: "hi"}
	ltCfg := biz.ParseChannelLongTaskConfig(ch.ConfigJSON)
	var preview string
	var previewMsgID string
	var queued bool
	err := h.processInboundStreaming(context.Background(), ch, ev, "dingtalk", ltCfg, "", &preview, &previewMsgID, &queued)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestFeishuOutboundExtraReceiveIDType(t *testing.T) {
	if lark.ReceiveIDTypeFromMeta(map[string]string{"receive_id_type": lark.ReceiveIDTypeUserID}) != lark.ReceiveIDTypeUserID {
		t.Fatal("expected user_id from extra")
	}
}

type streamChannelRepo struct{}

func (r *streamChannelRepo) List(context.Context) ([]biz.Channel, error) { return nil, nil }
func (r *streamChannelRepo) Get(context.Context, string) (biz.Channel, error) {
	return biz.Channel{}, nil
}
func (r *streamChannelRepo) GetByKey(context.Context, string) (biz.Channel, error) {
	return biz.Channel{}, nil
}
func (r *streamChannelRepo) Create(context.Context, biz.Channel) (biz.Channel, error) {
	return biz.Channel{}, nil
}
func (r *streamChannelRepo) Update(context.Context, biz.Channel) (biz.Channel, error) {
	return biz.Channel{}, nil
}
func (r *streamChannelRepo) Delete(context.Context, string) error { return nil }
func (r *streamChannelRepo) ListCredentials(context.Context, string) ([]biz.ChannelCredential, error) {
	return nil, nil
}
func (r *streamChannelRepo) UpsertCredential(context.Context, biz.ChannelCredential) (biz.ChannelCredential, error) {
	return biz.ChannelCredential{}, nil
}
func (r *streamChannelRepo) DeleteCredential(context.Context, string, string) error { return nil }
func (r *streamChannelRepo) ListDeliveries(context.Context, string, int) ([]biz.ChannelDelivery, error) {
	return nil, nil
}
func (r *streamChannelRepo) AddDelivery(context.Context, biz.ChannelDelivery) (biz.ChannelDelivery, error) {
	return biz.ChannelDelivery{}, nil
}
func (r *streamChannelRepo) ListPendingDeliveries(context.Context, int) ([]biz.ChannelDelivery, error) {
	return nil, nil
}
func (r *streamChannelRepo) UpdateDelivery(context.Context, biz.ChannelDelivery) error { return nil }
