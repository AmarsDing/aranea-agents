package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func TestChannelRuntimeLeaseRepoAcquireRenewRelease(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureChannelRuntimeLeaseSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewChannelRuntimeLeaseRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres})
	now := time.Now().UTC()
	lease := biz.NewChannelRuntimeLease("ch-1", "lark", "node-a", time.Minute, now)
	claimed, err := repo.TryAcquireRuntimeLease(ctx, lease)
	if err != nil || !claimed {
		t.Fatalf("first acquire claimed=%v err=%v", claimed, err)
	}
	other := biz.NewChannelRuntimeLease("ch-1", "lark", "node-b", time.Minute, now)
	claimed, err = repo.TryAcquireRuntimeLease(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("second owner should not claim unexpired lease")
	}
	renewed, err := repo.RenewRuntimeLease(ctx, lease.Key, "node-a", now.Add(2*time.Minute))
	if err != nil || !renewed {
		t.Fatalf("renewed=%v err=%v", renewed, err)
	}
	if err := repo.ReleaseRuntimeLease(ctx, lease.Key, "node-a"); err != nil {
		t.Fatal(err)
	}
	claimed, err = repo.TryAcquireRuntimeLease(ctx, other)
	if err != nil || !claimed {
		t.Fatalf("acquire after release claimed=%v err=%v", claimed, err)
	}
}
