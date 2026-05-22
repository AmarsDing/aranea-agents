package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type deliveryRepoStub struct {
	last ChannelDelivery
}

func (s *deliveryRepoStub) List(_ context.Context) ([]Channel, error) { return nil, nil }
func (s *deliveryRepoStub) Get(_ context.Context, id string) (Channel, error) {
	return Channel{}, nil
}
func (s *deliveryRepoStub) GetByKey(_ context.Context, channelKey string) (Channel, error) {
	return Channel{}, nil
}
func (s *deliveryRepoStub) Create(_ context.Context, row Channel) (Channel, error) { return row, nil }
func (s *deliveryRepoStub) Update(_ context.Context, row Channel) (Channel, error) { return row, nil }
func (s *deliveryRepoStub) Delete(_ context.Context, id string) error                { return nil }
func (s *deliveryRepoStub) ListCredentials(_ context.Context, channelID string) ([]ChannelCredential, error) {
	return nil, nil
}
func (s *deliveryRepoStub) UpsertCredential(_ context.Context, cred ChannelCredential) (ChannelCredential, error) {
	return cred, nil
}
func (s *deliveryRepoStub) DeleteCredential(_ context.Context, channelID, credentialKey string) error {
	return nil
}
func (s *deliveryRepoStub) ListDeliveries(_ context.Context, channelID string, limit int) ([]ChannelDelivery, error) {
	return nil, nil
}
func (s *deliveryRepoStub) AddDelivery(_ context.Context, d ChannelDelivery) (ChannelDelivery, error) {
	return d, nil
}
func (s *deliveryRepoStub) ListPendingDeliveries(_ context.Context, limit int) ([]ChannelDelivery, error) {
	return nil, nil
}
func (s *deliveryRepoStub) UpdateDelivery(_ context.Context, d ChannelDelivery) error {
	s.last = d
	return nil
}

func TestOutboundRetryDelay(t *testing.T) {
	if outboundRetryDelay(1) != 5*time.Second {
		t.Fatalf("attempt 1: %v", outboundRetryDelay(1))
	}
	if outboundRetryDelay(2) != 10*time.Second {
		t.Fatalf("attempt 2: %v", outboundRetryDelay(2))
	}
	if outboundRetryDelay(3) != 20*time.Second {
		t.Fatalf("attempt 3: %v", outboundRetryDelay(3))
	}
	if outboundRetryDelay(10) != outboundRetryMaxDelay {
		t.Fatalf("attempt 10 should cap: %v", outboundRetryDelay(10))
	}
}

func TestIsOutboundDeliveryReady(t *testing.T) {
	uc := &ChannelUsecase{}
	future := time.Now().UTC().Add(2 * time.Minute).Format(time.RFC3339)
	past := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)

	if !uc.IsOutboundDeliveryReady(ChannelDelivery{Status: ChannelDeliveryStatusPending}) {
		t.Fatal("pending should be ready")
	}
	if !uc.IsOutboundDeliveryReady(ChannelDelivery{
		Status:      ChannelDeliveryStatusRetry,
		PayloadJSON: `{"next_retry_at":"` + past + `"}`,
	}) {
		t.Fatal("past retry should be ready")
	}
	if uc.IsOutboundDeliveryReady(ChannelDelivery{
		Status:      ChannelDeliveryStatusRetry,
		PayloadJSON: `{"next_retry_at":"` + future + `"}`,
	}) {
		t.Fatal("future retry should wait")
	}
}

func TestMarkOutboundAttemptBackoffAndDeadLetter(t *testing.T) {
	repo := &deliveryRepoStub{}
	uc := &ChannelUsecase{repo: repo}
	row := ChannelDelivery{
		ID:          "d1",
		ChannelID:   "c1",
		Status:      ChannelDeliveryStatusPending,
		PayloadJSON: `{"platform":"feishu","recipient":"u1","text":"hi"}`,
	}

	dead, err := uc.MarkOutboundAttempt(t.Context(), row, errors.New("send failed"))
	if err != nil || dead {
		t.Fatalf("first failure: dead=%v err=%v status=%s", dead, err, repo.last.Status)
	}
	if repo.last.Status != ChannelDeliveryStatusRetry {
		t.Fatalf("status=%s", repo.last.Status)
	}
	var payload ChannelOutboundPayload
	if err := json.Unmarshal([]byte(repo.last.PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Attempts != 1 || payload.NextRetryAt == "" {
		t.Fatalf("payload=%+v", payload)
	}

	row = repo.last
	dead, err = uc.MarkOutboundAttempt(t.Context(), row, errors.New("again"))
	if err != nil || dead {
		t.Fatalf("second failure: dead=%v", dead)
	}
	row = repo.last
	dead, err = uc.MarkOutboundAttempt(t.Context(), row, errors.New("final"))
	if err != nil || !dead {
		t.Fatalf("third failure should dead-letter: dead=%v err=%v status=%s", dead, err, repo.last.Status)
	}
	if repo.last.Status != ChannelDeliveryStatusError {
		t.Fatalf("status=%s", repo.last.Status)
	}
}
