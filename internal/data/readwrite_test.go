package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/testhelper"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// newTestEntClient creates a schema-isolated Postgres Ent client for testing.
// No tables are created — the tests only exercise client routing and empty
// transactions, which Postgres allows.
func newTestEntClient(t *testing.T) *ent.Client {
	t.Helper()
	rawDB := testhelper.SetupTestPGRaw(t)
	drv := entsql.OpenDB(dialect.Postgres, rawDB)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })
	return client
}

func TestReadWriteClient_ReadOutsideTx(t *testing.T) {
	write := newTestEntClient(t)
	read := newTestEntClient(t)
	rw := NewReadWriteClient(write, read)

	got := rw.Read(context.Background())
	if got != read {
		t.Fatal("ReadWriteClient.Read: expected read client outside transaction")
	}
}

func TestReadWriteClient_WriteOutsideTx(t *testing.T) {
	write := newTestEntClient(t)
	read := newTestEntClient(t)
	rw := NewReadWriteClient(write, read)

	got := rw.Write(context.Background())
	if got != write {
		t.Fatal("ReadWriteClient.Write: expected write client outside transaction")
	}
}

func TestReadWriteClient_ReadInsideTx(t *testing.T) {
	write := newTestEntClient(t)
	read := newTestEntClient(t)
	rw := NewReadWriteClient(write, read)

	tx, err := write.Tx(context.Background())
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback()

	txCtx := context.WithValue(context.Background(), txClientKey{}, tx)
	got := rw.Read(txCtx)
	if got != tx.Client() {
		t.Fatal("ReadWriteClient.Read: expected tx client inside transaction")
	}
}

func TestReadWriteClient_WriteInsideTx(t *testing.T) {
	write := newTestEntClient(t)
	read := newTestEntClient(t)
	rw := NewReadWriteClient(write, read)

	tx, err := write.Tx(context.Background())
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback()

	txCtx := context.WithValue(context.Background(), txClientKey{}, tx)
	got := rw.Write(txCtx)
	if got != tx.Client() {
		t.Fatal("ReadWriteClient.Write: expected tx client inside transaction")
	}
}
