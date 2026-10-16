package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/pkg/loggateway"

	_ "modernc.org/sqlite"
)

func TestChatTurnSubmitClaimer_TryClaimAndRelease(t *testing.T) {
	db, err := sql.Open("sqlite", "file:chat_turn_claim_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := EnsureChatTurnSubmitClaimSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	c := NewChatTurnSubmitClaimer(&Data{
		rawDB:   db,
		readDB:  db,
		rwDB:    NewReadWriteDB(db, db),
		lg:      loggateway.NewNoop(),
		dialect: DialectSQLite,
	})

	ok, err := c.TryClaim(ctx, "sess-1", "pending-user-abc")
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	ok, err = c.TryClaim(ctx, "sess-1", "pending-user-abc")
	if err != nil || ok {
		t.Fatalf("duplicate claim: ok=%v err=%v (want false,nil)", ok, err)
	}
	ok, err = c.TryClaim(ctx, "sess-2", "pending-user-abc")
	if err != nil || !ok {
		t.Fatalf("other session claim: ok=%v err=%v", ok, err)
	}
	if err := c.Release(ctx, "sess-1", "pending-user-abc"); err != nil {
		t.Fatalf("release: %v", err)
	}
	ok, err = c.TryClaim(ctx, "sess-1", "pending-user-abc")
	if err != nil || !ok {
		t.Fatalf("reclaim after release: ok=%v err=%v", ok, err)
	}
}
