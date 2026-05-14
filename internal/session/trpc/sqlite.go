package session

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite/compat"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	trpcsqlite "trpc.group/trpc-go/trpc-agent-go/session/sqlite"
)

func NewInMemorySessionService() trpcsession.Service {
	return trpcinmemory.NewSessionService()
}

func NewSQLiteSessionService(dsn string) (trpcsession.Service, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("session sqlite: dsn is required")
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("session sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("session sqlite ping: %w", err)
	}
	svc, err := trpcsqlite.NewService(db,
		trpcsqlite.WithTablePrefix("trpc_"),
		trpcsqlite.WithEnableAsyncPersist(true),
		trpcsqlite.WithSoftDelete(true),
	)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("session sqlite init: %w", err)
	}
	return svc, nil
}
