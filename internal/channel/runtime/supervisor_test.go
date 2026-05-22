package runtime_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
)

type reconnectHandler struct{}

func (reconnectHandler) ProcessInbound(ctx context.Context, ch biz.Channel, ev port.InboundEvent) error {
	return nil
}

type countingRepo struct {
	channels []biz.Channel
}

func (r *countingRepo) List(_ context.Context) ([]biz.Channel, error) {
	return append([]biz.Channel(nil), r.channels...), nil
}
func (r *countingRepo) Get(_ context.Context, id string) (biz.Channel, error) { return biz.Channel{}, nil }
func (r *countingRepo) GetByKey(_ context.Context, key string) (biz.Channel, error) {
	return biz.Channel{}, nil
}
func (r *countingRepo) Create(_ context.Context, row biz.Channel) (biz.Channel, error) { return row, nil }
func (r *countingRepo) Update(_ context.Context, row biz.Channel) (biz.Channel, error) { return row, nil }
func (r *countingRepo) Delete(_ context.Context, id string) error                     { return nil }
func (r *countingRepo) ListCredentials(_ context.Context, channelID string) ([]biz.ChannelCredential, error) {
	return nil, nil
}
func (r *countingRepo) UpsertCredential(_ context.Context, cred biz.ChannelCredential) (biz.ChannelCredential, error) {
	return cred, nil
}
func (r *countingRepo) DeleteCredential(_ context.Context, channelID, credentialKey string) error { return nil }
func (r *countingRepo) ListDeliveries(_ context.Context, channelID string, limit int) ([]biz.ChannelDelivery, error) {
	return nil, nil
}
func (r *countingRepo) AddDelivery(_ context.Context, d biz.ChannelDelivery) (biz.ChannelDelivery, error) {
	return d, nil
}
func (r *countingRepo) ListPendingDeliveries(_ context.Context, limit int) ([]biz.ChannelDelivery, error) {
	return nil, nil
}
func (r *countingRepo) UpdateDelivery(_ context.Context, d biz.ChannelDelivery) error { return nil }
func (r *countingRepo) ListCredentialsRaw(_ context.Context, channelID string) ([]biz.ChannelCredential, error) {
	return nil, nil
}

func TestSupervisorReconnectsAfterDisconnect(t *testing.T) {
	var calls atomic.Int32
	runtime.RegisterStarter("reconnplat", "polling", func(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup runtime.CredentialLookup, handler runtime.InboundHandler) error {
		n := calls.Add(1)
		if n == 1 {
			return errors.New("simulated disconnect")
		}
		<-ctx.Done()
		return ctx.Err()
	})

	repo := &countingRepo{channels: []biz.Channel{{
		ID:         "ch-reconn",
		Enabled:    true,
		ConfigJSON: `{"type":"reconnplat","receive_mode":"polling"}`,
	}}}
	uc := biz.NewChannelUsecase(repo)
	mgr := runtime.NewManager(uc, reconnectHandler{}, func(ctx context.Context, creds []biz.ChannelCredential, key string) (string, error) {
		return "token", nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Reload(ctx); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatalf("expected reconnect restart, calls=%d", calls.Load())
	}
	cancel()
}
