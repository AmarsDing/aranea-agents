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

func newTestSQLDB(t *testing.T) *sql.DB {
	t.Helper()
	rawDB, err := sql.Open(dialect.SQLite, ":memory:")
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	t.Cleanup(func() { rawDB.Close() })
	return rawDB
}

func newTestEntClientWithDB(t *testing.T) (*ent.Client, *sql.DB) {
	t.Helper()
	rawDB := newTestSQLDB(t)
	drv := entsql.OpenDB(dialect.SQLite, rawDB)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })
	return client, rawDB
}

func TestReadWriteDB_ReadDBOutsideTx(t *testing.T) {
	writeDB := newTestSQLDB(t)
	readDB := newTestSQLDB(t)
	rwDB := NewReadWriteDB(writeDB, readDB)

	got := rwDB.ReadDB(context.Background())
	if got != readDB {
		t.Fatal("ReadWriteDB.ReadDB: expected read DB outside transaction")
	}
}

func TestReadWriteDB_WriteDBOutsideTx(t *testing.T) {
	writeDB := newTestSQLDB(t)
	readDB := newTestSQLDB(t)
	rwDB := NewReadWriteDB(writeDB, readDB)

	got := rwDB.WriteDB(context.Background())
	if got != writeDB {
		t.Fatal("ReadWriteDB.WriteDB: expected write DB outside transaction")
	}
}

func TestReadWriteDB_ReadDBInsideTx(t *testing.T) {
	entClient, _ := newTestEntClientWithDB(t)
	readDB := newTestSQLDB(t)
	writeDB := newTestSQLDB(t)
	rwDB := NewReadWriteDB(writeDB, readDB)

	tx, err := entClient.Tx(context.Background())
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback()

	txCtx := context.WithValue(context.Background(), rawTxKey{}, tx)
	got := rwDB.ReadDB(txCtx)
	if got != tx.Client() {
		t.Fatal("ReadWriteDB.ReadDB: expected tx client inside transaction")
	}
}

func TestReadWriteDB_WriteDBInsideTx(t *testing.T) {
	entClient, _ := newTestEntClientWithDB(t)
	readDB := newTestSQLDB(t)
	writeDB := newTestSQLDB(t)
	rwDB := NewReadWriteDB(writeDB, readDB)

	tx, err := entClient.Tx(context.Background())
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback()

	txCtx := context.WithValue(context.Background(), rawTxKey{}, tx)
	got := rwDB.WriteDB(txCtx)
	if got != tx.Client() {
		t.Fatal("ReadWriteDB.WriteDB: expected tx client inside transaction")
	}
}
