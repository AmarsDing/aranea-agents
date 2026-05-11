//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestSetGetClientBuilder(t *testing.T) {
	oldRegistry := sqliteRegistry
	sqliteRegistry = make(map[string][]ClientBuilderOpt)
	defer func() { sqliteRegistry = oldRegistry }()

	oldBuilder := GetClientBuilder()
	defer func() { SetClientBuilder(oldBuilder) }()

	invoked := false
	custom := func(ctx context.Context, opts ...ClientBuilderOpt) (Client, error) {
		invoked = true
		return nil, nil
	}

	SetClientBuilder(custom)
	b := GetClientBuilder()
	_, err := b(context.Background(), WithClientConnString(":memory:"))
	require.NoError(t, err)
	require.True(t, invoked, "custom builder was not invoked")
}

func TestDefaultClientBuilder_EmptyConnString(t *testing.T) {
	const expected = "sqlite: connection string is empty"
	_, err := defaultClientBuilder(context.Background())
	require.Error(t, err)
	require.Equal(t, expected, err.Error())
}

func TestRegisterAndGetSqliteInstance(t *testing.T) {
	oldRegistry := sqliteRegistry
	sqliteRegistry = make(map[string][]ClientBuilderOpt)
	defer func() { sqliteRegistry = oldRegistry }()

	const (
		name       = "test-instance"
		connString = "file:test_register.db?_foreign_keys=on"
	)

	RegisterSqliteInstance(name, WithClientConnString(connString))
	opts, ok := GetSqliteInstance(name)
	require.True(t, ok, "expected instance to exist")
	require.NotEmpty(t, opts, "expected at least one option")

	cfg := &ClientBuilderOpts{}
	for _, opt := range opts {
		opt(cfg)
	}
	require.Equal(t, connString, cfg.ConnString)
}

func TestGetSqliteInstance_NotFound(t *testing.T) {
	oldRegistry := sqliteRegistry
	sqliteRegistry = make(map[string][]ClientBuilderOpt)
	defer func() { sqliteRegistry = oldRegistry }()

	opts, ok := GetSqliteInstance("not-exist")
	require.False(t, ok)
	require.Nil(t, opts)
}

func TestWithExtraOptions_Accumulation(t *testing.T) {
	oldBuilder := GetClientBuilder()
	defer func() { SetClientBuilder(oldBuilder) }()

	observed := make([]any, 0)
	custom := func(ctx context.Context, builderOpts ...ClientBuilderOpt) (Client, error) {
		cfg := &ClientBuilderOpts{}
		for _, opt := range builderOpts {
			opt(cfg)
		}
		observed = append(observed, cfg.ExtraOptions...)
		return nil, nil
	}
	SetClientBuilder(custom)

	const (
		first  = "alpha"
		second = "beta"
		third  = "gamma"
	)
	b := GetClientBuilder()
	_, err := b(
		context.Background(),
		WithClientConnString(":memory:"),
		WithExtraOptions(first),
		WithExtraOptions(second, third),
	)
	require.NoError(t, err)
	require.Equal(t, []any{first, second, third}, observed)
}

func TestRegisterSqliteInstance_AppendsOptions(t *testing.T) {
	oldRegistry := sqliteRegistry
	sqliteRegistry = make(map[string][]ClientBuilderOpt)
	defer func() { sqliteRegistry = oldRegistry }()

	const name = "append-instance"
	RegisterSqliteInstance(name, WithClientConnString(":memory:"))
	RegisterSqliteInstance(name, WithExtraOptions("x"), WithExtraOptions("y"))

	opts, ok := GetSqliteInstance(name)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(opts), 3)

	cfg := &ClientBuilderOpts{}
	for _, opt := range opts {
		opt(cfg)
	}
	require.Equal(t, []any{"x", "y"}, cfg.ExtraOptions)
}

func TestSQLClient_Close(t *testing.T) {
	oldBuilder := GetClientBuilder()
	defer func() { SetClientBuilder(oldBuilder) }()

	closeCalled := false
	mockClient := &mockClient{
		closeFn: func() error {
			closeCalled = true
			return nil
		},
	}

	SetClientBuilder(func(ctx context.Context, opts ...ClientBuilderOpt) (Client, error) {
		return mockClient, nil
	})

	client, err := GetClientBuilder()(context.Background(), WithClientConnString("test"))
	require.NoError(t, err)
	require.NotNil(t, client)

	err = client.Close()
	require.NoError(t, err)
	require.True(t, closeCalled)
}

func TestSQLClient_ExecContext(t *testing.T) {
	oldBuilder := GetClientBuilder()
	defer func() { SetClientBuilder(oldBuilder) }()

	execCalled := false
	mockClient := &mockClient{
		execFn: func(ctx context.Context, query string, args ...any) (sql.Result, error) {
			execCalled = true
			require.Equal(t, "INSERT INTO test VALUES (?)", query)
			require.Equal(t, []any{"value"}, args)
			return &mockResult{rowsAffected: 1}, nil
		},
	}

	SetClientBuilder(func(ctx context.Context, opts ...ClientBuilderOpt) (Client, error) {
		return mockClient, nil
	})

	client, err := GetClientBuilder()(context.Background(), WithClientConnString("test"))
	require.NoError(t, err)

	result, err := client.ExecContext(context.Background(), "INSERT INTO test VALUES (?)", "value")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, execCalled)

	rows, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
}

func TestSQLClient_Query(t *testing.T) {
	oldBuilder := GetClientBuilder()
	defer func() { SetClientBuilder(oldBuilder) }()

	queryCalled := false
	mockClient := &mockClient{
		queryFn: func(ctx context.Context, handler HandlerFunc, query string, args ...any) error {
			queryCalled = true
			require.Equal(t, "SELECT * FROM test WHERE id = ?", query)
			require.Equal(t, []any{1}, args)
			return nil
		},
	}

	SetClientBuilder(func(ctx context.Context, opts ...ClientBuilderOpt) (Client, error) {
		return mockClient, nil
	})

	client, err := GetClientBuilder()(context.Background(), WithClientConnString("test"))
	require.NoError(t, err)

	err = client.Query(context.Background(), func(rows *sql.Rows) error {
		return nil
	}, "SELECT * FROM test WHERE id = ?", 1)
	require.NoError(t, err)
	require.True(t, queryCalled)
}

func TestSQLClient_Transaction(t *testing.T) {
	oldBuilder := GetClientBuilder()
	defer func() { SetClientBuilder(oldBuilder) }()

	txCalled := false
	mockClient := &mockClient{
		txFn: func(ctx context.Context, fn TxFunc) error {
			txCalled = true
			return fn(nil)
		},
	}

	SetClientBuilder(func(ctx context.Context, opts ...ClientBuilderOpt) (Client, error) {
		return mockClient, nil
	})

	client, err := GetClientBuilder()(context.Background(), WithClientConnString("test"))
	require.NoError(t, err)

	txFnCalled := false
	err = client.Transaction(context.Background(), func(tx *sql.Tx) error {
		txFnCalled = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, txCalled)
	require.True(t, txFnCalled)
}

type mockClient struct {
	execFn  func(ctx context.Context, query string, args ...any) (sql.Result, error)
	queryFn func(ctx context.Context, handler HandlerFunc, query string, args ...any) error
	txFn    func(ctx context.Context, fn TxFunc) error
	closeFn func() error
}

func (m *mockClient) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.execFn != nil {
		return m.execFn(ctx, query, args...)
	}
	return nil, nil
}

func (m *mockClient) Query(ctx context.Context, handler HandlerFunc, query string, args ...any) error {
	if m.queryFn != nil {
		return m.queryFn(ctx, handler, query, args...)
	}
	return nil
}

func (m *mockClient) Transaction(ctx context.Context, fn TxFunc) error {
	if m.txFn != nil {
		return m.txFn(ctx, fn)
	}
	return nil
}

func (m *mockClient) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockResult struct {
	rowsAffected int64
	lastInsertID int64
}

func (m *mockResult) LastInsertId() (int64, error) {
	return m.lastInsertID, nil
}

func (m *mockResult) RowsAffected() (int64, error) {
	return m.rowsAffected, nil
}

func TestRealSQLClient_ExecContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	mock.ExpectExec("INSERT INTO test").
		WithArgs("value").
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := client.ExecContext(context.Background(), "INSERT INTO test VALUES (?)", "value")
	require.NoError(t, err)
	require.NotNil(t, result)

	rows, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Query(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(1, "test1").
		AddRow(2, "test2")

	mock.ExpectQuery("SELECT .* FROM test").
		WithArgs(1).
		WillReturnRows(rows)

	var results []struct {
		ID   int
		Name string
	}

	err = client.Query(context.Background(), func(rows *sql.Rows) error {
		for rows.Next() {
			var id int
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			results = append(results, struct {
				ID   int
				Name string
			}{ID: id, Name: name})
		}
		return nil
	}, "SELECT * FROM test WHERE id = ?", 1)

	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, 1, results[0].ID)
	require.Equal(t, "test1", results[0].Name)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Query_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	mock.ExpectQuery("SELECT .* FROM test").
		WillReturnError(errors.New("query error"))

	err = client.Query(context.Background(), func(rows *sql.Rows) error {
		return nil
	}, "SELECT * FROM test")

	require.Error(t, err)
	require.Contains(t, err.Error(), "query")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Transaction_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO test").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = client.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test VALUES (?)", "value")
		return err
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Transaction_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO test").WillReturnError(errors.New("insert error"))
	mock.ExpectRollback()

	err = client.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test VALUES (?)", "value")
		return err
	})

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Transaction_BeginError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	mock.ExpectBegin().WillReturnError(errors.New("begin error"))

	err = client.Transaction(context.Background(), func(tx *sql.Tx) error {
		return nil
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "begin transaction")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Transaction_CommitError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("commit error"))

	err = client.Transaction(context.Background(), func(tx *sql.Tx) error {
		return nil
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "commit transaction")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Close(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	client := &sqlClient{db: db}

	mock.ExpectClose()

	err = client.Close()
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Query_HandlerError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "test1")
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	handlerErr := errors.New("handler error")
	err = client.Query(context.Background(), func(rows *sql.Rows) error {
		return handlerErr
	}, "SELECT * FROM test")

	require.Error(t, err)
	require.Equal(t, handlerErr, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Query_RowsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(1, "test1").
		AddRow(2, "test2").
		RowError(1, errors.New("rows iteration error"))

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	err = client.Query(context.Background(), func(rows *sql.Rows) error {
		for rows.Next() {
			var id int
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
		}
		return nil
	}, "SELECT * FROM test")

	require.Error(t, err)
	require.Contains(t, err.Error(), "rows iteration")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealSQLClient_Transaction_Panic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	client := &sqlClient{db: db}

	mock.ExpectBegin()
	mock.ExpectRollback()

	defer func() {
		if r := recover(); r != nil {
			require.Equal(t, "panic in transaction", r)
		} else {
			t.Fatal("expected panic but didn't get one")
		}
	}()

	_ = client.Transaction(context.Background(), func(tx *sql.Tx) error {
		panic("panic in transaction")
	})

	require.NoError(t, mock.ExpectationsWereMet())
}
