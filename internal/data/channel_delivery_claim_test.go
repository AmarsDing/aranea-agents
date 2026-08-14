package data

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

func TestClaimPendingDeliveries_ConcurrentOnlyOneWins(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewChannelRepo(d)
	ctx := context.Background()

	row, err := repo.AddDelivery(ctx, biz.ChannelDelivery{
		ID:          uuid.NewString(),
		ChannelID:   "ch-claim-1",
		Status:      biz.ChannelDeliveryStatusPending,
		PayloadJSON: `{"platform":"feishu","recipient":"u1","text":"hi"}`,
	})
	if err != nil {
		t.Fatalf("AddDelivery: %v", err)
	}

	const n = 8
	var mu sync.Mutex
	var winners []string
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			claimed, err := repo.ClaimPendingDeliveries(ctx, 10)
			if err != nil {
				t.Errorf("ClaimPendingDeliveries: %v", err)
				return
			}
			for _, c := range claimed {
				if c.ID == row.ID {
					mu.Lock()
					winners = append(winners, c.ID)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if len(winners) != 1 {
		t.Fatalf("concurrent claim winners = %d, want 1", len(winners))
	}

	got, err := repo.ClaimPendingDeliveries(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.ID == row.ID {
			t.Fatal("already-claimed row must not be reclaimed within lease")
		}
	}
}

func TestClaimPendingDeliveries_ExpiredSendingLeaseReclaimed(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewChannelRepo(d)
	ctx := context.Background()

	row, err := repo.AddDelivery(ctx, biz.ChannelDelivery{
		ID:          uuid.NewString(),
		ChannelID:   "ch-claim-lease",
		Status:      biz.ChannelDeliveryStatusSending,
		PayloadJSON: `{"platform":"feishu","recipient":"u1","text":"hi"}`,
	})
	if err != nil {
		t.Fatalf("AddDelivery: %v", err)
	}
	old := time.Now().UTC().Add(-biz.OutboundDeliveryLease - time.Minute).Format(time.RFC3339)
	if _, err := d.RWDB().WriteHandle().ExecContext(ctx,
		`UPDATE channel_delivery SET status = $1, updated_at = $2 WHERE id = $3`,
		biz.ChannelDeliveryStatusSending, old, row.ID,
	); err != nil {
		t.Fatalf("age sending row: %v", err)
	}

	claimed, err := repo.ClaimPendingDeliveries(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range claimed {
		if c.ID == row.ID {
			found = true
			if c.Status != biz.ChannelDeliveryStatusSending {
				t.Fatalf("status = %q, want sending", c.Status)
			}
		}
	}
	if !found {
		t.Fatal("expired sending lease should be reclaimable")
	}
}

func TestClaimPendingDeliveries_SkipsFutureRetry(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewChannelRepo(d)
	ctx := context.Background()

	future := time.Now().UTC().Add(2 * time.Minute).Format(time.RFC3339)
	row, err := repo.AddDelivery(ctx, biz.ChannelDelivery{
		ID:          uuid.NewString(),
		ChannelID:   "ch-claim-retry",
		Status:      biz.ChannelDeliveryStatusRetry,
		PayloadJSON: `{"platform":"feishu","recipient":"u1","text":"hi","next_retry_at":"` + future + `"}`,
	})
	if err != nil {
		t.Fatalf("AddDelivery: %v", err)
	}

	claimed, err := repo.ClaimPendingDeliveries(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range claimed {
		if c.ID == row.ID {
			t.Fatal("future retry must not be claimed")
		}
	}
}
