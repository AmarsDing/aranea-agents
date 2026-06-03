package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	_ "github.com/glebarez/go-sqlite/compat"
)

// newTestEntClient creates an in-memory SQLite Ent client for testing.
func newTestEntClient(t *testing.T) *ent.Client {
	t.Helper()
	rawDB, err := sql.Open(dialect.SQLite, "file:enttest?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	t.Cleanup(func() { rawDB.Close() })
	// Set PRAGMAs required by Ent Schema.Create.
	for _, pragma := range []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA journal_mode=WAL",
	} {
		if _, e := rawDB.ExecContext(context.Background(), pragma); e != nil {
			t.Fatalf("pragma %s: %v", pragma, e)
		}
	}
	drv := entsql.OpenDB(dialect.SQLite, rawDB)
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
