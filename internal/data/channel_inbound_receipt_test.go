package data

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite/compat"
)

func TestChannelInboundReceipt_TryClaimDedup(t *testing.T) {
	db, err := sql.Open("sqlite", "file:channel_inbound_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := EnsureChannelInboundSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewChannelInboundReceiptRepo(&Data{rawDB: db})
	claimed, err := repo.TryClaim(ctx, "ch1", "feishu:msg-1", "ou_x", "你好")
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	claimed2, err := repo.TryClaim(ctx, "ch1", "feishu:msg-1", "ou_x", "你好")
	if err != nil || claimed2 {
		t.Fatalf("duplicate claim: claimed=%v err=%v", claimed2, err)
	}
}
