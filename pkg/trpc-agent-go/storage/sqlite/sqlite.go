//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package sqlite provides SQLite instance info management via database/sql.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // register sqlite3 driver
)

func init() {
	sqliteRegistry = make(map[string][]ClientBuilderOpt)
}

var sqliteRegistry map[string][]ClientBuilderOpt

type clientBuilder func(ctx context.Context, builderOpts ...ClientBuilderOpt) (Client, error)

var globalBuilder clientBuilder = defaultClientBuilder

// SetClientBuilder sets the sqlite client builder.
func SetClientBuilder(builder clientBuilder) {
	globalBuilder = builder
}

// GetClientBuilder gets the sqlite client builder.
func GetClientBuilder() clientBuilder {
	return globalBuilder
}

// defaultClientBuilder opens a database/sql pool using the mattn/go-sqlite3 driver.
func defaultClientBuilder(ctx context.Context, builderOpts ...ClientBuilderOpt) (Client, error) {
	o := &ClientBuilderOpts{}
	for _, opt := range builderOpts {
		opt(o)
	}

	if o.ConnString == "" {
		return nil, errors.New("sqlite: connection string is empty")
	}

	db, err := sql.Open("sqlite3", o.ConnString)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open connection: %w", err)
	}

	// Typical for SQLite file DBs; safe for :memory: as well.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: ping database: %w", err)
	}

	return &sqlClient{db: db}, nil
}

// ClientBuilderOpt is the option for the sqlite client.
type ClientBuilderOpt func(*ClientBuilderOpts)

// ClientBuilderOpts is the options for the sqlite client.
type ClientBuilderOpts struct {
	// ConnString is the SQLite DSN for database/sql.
	// Examples: path "app.db", in-memory ":memory:", or URI "file:app.db?_foreign_keys=on".
	ConnString string

	// ExtraOptions is the extra options for the sqlite client.
	// This is mainly used for customized sqlite client builders.
	ExtraOptions []any
}

// WithClientConnString sets the SQLite connection string for clientBuilder.
func WithClientConnString(connString string) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.ConnString = connString
	}
}

// WithExtraOptions sets the sqlite client extra options for clientBuilder.
// This option is mainly used for customized sqlite client builders.
func WithExtraOptions(extraOptions ...any) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.ExtraOptions = append(opts.ExtraOptions, extraOptions...)
	}
}

// RegisterSqliteInstance registers a sqlite instance with the given options.
func RegisterSqliteInstance(name string, opts ...ClientBuilderOpt) {
	sqliteRegistry[name] = append(sqliteRegistry[name], opts...)
}

// GetSqliteInstance gets the sqlite instance options by name.
func GetSqliteInstance(name string) ([]ClientBuilderOpt, bool) {
	instance, ok := sqliteRegistry[name]
	return instance, ok
}

// Client defines the interface for SQLite operations.
// It mirrors the database/sql standard library interface.
type Client interface {
	// ExecContext executes a query that doesn't return rows.
	// For example: INSERT, UPDATE, DELETE.
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)

	// Query executes a query that returns rows and passes them to the handler.
	// The rows are automatically closed after the handler returns.
	// This ensures proper resource cleanup and prevents resource leaks.
	Query(ctx context.Context, fn HandlerFunc, query string, args ...any) error

	// Transaction executes a function within a transaction.
	// The transaction is automatically committed if the function returns nil,
	// or rolled back if the function returns an error or panics.
	Transaction(ctx context.Context, fn TxFunc) error

	// Close closes the database connection pool and releases all resources.
	// After calling Close, the client should not be used anymore.
	Close() error
}

// HandlerFunc is a function that processes query results.
// The rows are automatically closed after this function returns.
type HandlerFunc func(*sql.Rows) error

// TxFunc is a function that executes within a transaction.
type TxFunc func(*sql.Tx) error

// sqlClient implements the Client interface using database/sql.
type sqlClient struct {
	db *sql.DB
}

// ExecContext executes a query that doesn't return rows.
func (c *sqlClient) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

// Query executes a query that returns rows and passes them to the handler.
// It automatically closes the rows after the handler completes or panics.
func (c *sqlClient) Query(ctx context.Context, handler HandlerFunc, query string, args ...any) error {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	defer rows.Close()

	if err := handler(rows); err != nil {
		return err
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows iteration: %w", err)
	}

	return nil
}

// Transaction executes a function within a transaction.
// It automatically handles commit on success and rollback on error or panic.
func (c *sqlClient) Transaction(ctx context.Context, fn TxFunc) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			_ = tx.Rollback()
		}
	}()

	err = fn(tx)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// Close closes the database connection pool and releases all resources.
// It's safe to call Close multiple times.
func (c *sqlClient) Close() error {
	return c.db.Close()
}
