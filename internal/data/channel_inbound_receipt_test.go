package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func TestChannelInboundReceipt_TryClaimDedup(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureChannelInboundSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewChannelInboundReceiptRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres})
	claimed, err := repo.TryClaim(ctx, "ch1", "feishu:msg-1", "ou_x", "你好")
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	claimed2, err := repo.TryClaim(ctx, "ch1", "feishu:msg-1", "ou_x", "你好")
	if err != nil || claimed2 {
		t.Fatalf("duplicate claim: claimed=%v err=%v", claimed2, err)
	}
}
