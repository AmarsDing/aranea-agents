package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// batchInsertTimeout is the per-batch INSERT timeout. Large enough for
// 500-row batches against Postgres on localhost; tuned for safety not speed.
const batchInsertTimeout = 120 * time.Second

// MigrationReport summarizes the migration outcome.
type MigrationReport struct {
	TotalTables int
	TotalRows   int64
	Migrated    []TableMigration
	Skipped     []TableSkip
	Failed      []TableFailure
}

// TableMigration records a successful table migration.
type TableMigration struct {
	Table string
	Rows  int64
}

// TableSkip records a skipped table.
type TableSkip struct {
	Table  string
	Reason string
}

// TableFailure records a failed table migration.
type TableFailure struct {
	Table string
	Error string
}

// Migrator copies data from SQLite to Postgres.
type Migrator struct {
	srcDB     *sql.DB
	tgtDB     *sql.DB
	batchSize int
	lg        loggateway.Logger
	// tgtConn is a dedicated connection acquired in MigrateAll to ensure
	// SET session_replication_role applies to all subsequent operations.
	// It is nil when MigrateAll is not running (methods fall back to tgtDB).
	tgtConn *sql.Conn
}

// NewMigrator creates a new Migrator.
func NewMigrator(srcDB, tgtDB *sql.DB, batchSize int, lg loggateway.Logger) *Migrator {
	if batchSize <= 0 {
		batchSize = 500
	}
	return &Migrator{srcDB: srcDB, tgtDB: tgtDB, batchSize: batchSize, lg: lg}
}

// pgExecer is the subset of *sql.DB/*sql.Conn methods used by the Migrator.
// Both types satisfy this interface, allowing methods to work with either.
type pgExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// pg returns the active Postgres executor: the dedicated connection during
// MigrateAll (so session_replication_role applies), or the pool otherwise.
func (m *Migrator) pg() pgExecer {
	if m.tgtConn != nil {
		return m.tgtConn
	}
	return m.tgtDB
}

// defaultSkipTables are tables that should never be migrated.
var defaultSkipTables = map[string]bool{
	"schema_migrations":    true, // Postgres DDL registry manages this fresh
	"sqlite_sequence":      true, // SQLite internal
	"sqlite_stat1":         true, // SQLite internal
	"messages_fts":         true, // FTS5 virtual table, rebuilt by DDL migration
	"messages_fts_config":  true, // FTS5 shadow table
	"messages_fts_data":    true, // FTS5 shadow table
	"messages_fts_idx":     true, // FTS5 shadow table
	"messages_fts_content": true, // FTS5 shadow table
	"messages_fts_docsize": true, // FTS5 shadow table
	// Framework-managed tables with incompatible schemas — created by
	// initFrameworkSchema (framework_schema.go) but data cannot be migrated directly.
	// See frameworkSchemaDDL comments for per-table rationale.
	"vector_embeddings":    true, // SQLite: embedding_json TEXT; Postgres: embedding vector — incompatible
	"memory_l2_index_meta": true, // Table dropped in 20260620; skip if present in legacy SQLite
	// trpc_session_events/states: SQLite stores nanosecond timestamps (INTEGER),
	// Postgres uses TIMESTAMP. The formats are incompatible — direct migration
	// would require timestamp conversion. These tables are framework-internal
	// session data that the app recreates on startup; historical data is not
	// business-critical and can be regenerated.
	"trpc_session_events":       true,
	"trpc_session_states":       true,
	"trpc_session_track_events": true, // same timestamp incompatibility as above
	"trpc_session_summaries":    true, // same timestamp incompatibility as above
	"trpc_app_states":           true, // same timestamp incompatibility as above
	"trpc_user_states":          true, // same timestamp incompatibility as above
}

// MigrateAll discovers tables from SQLite and migrates each to Postgres.
// If tableFilter is non-empty, only that table is migrated.
// skipSet is merged with defaultSkipTables.
//
// FK constraints are disabled during migration (SET session_replication_role = 'replica')
// to allow tables to be migrated in alphabetical order without FK violations.
// They are re-enabled after migration. The caller should run the validator
// afterwards to verify data integrity.
//
// A dedicated *sql.Conn is acquired from the pool for the entire migration so
// that SET session_replication_role (a session-level setting) applies to all
// subsequent operations. Using the pool directly would be incorrect because
// different statements might land on different connections where the setting
// is not applied.
func (m *Migrator) MigrateAll(ctx context.Context, tableFilter string, skipSet map[string]bool) (*MigrationReport, error) {
	tables, err := m.discoverTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover tables: %w", err)
	}

	// Acquire a dedicated Postgres connection for the duration of the migration.
	// SET session_replication_role is session-scoped: it only affects the
	// connection that executes it. With a pool, subsequent ExecContext calls
	// might use a different connection where FK checks are still enabled,
	// causing FK violations during bulk load. Pinning a single connection
	// ensures the setting applies to every operation in this migration.
	conn, err := m.tgtDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire postgres connection: %w", err)
	}
	m.tgtConn = conn
	defer func() {
		m.tgtConn = nil
		if err := conn.Close(); err != nil {
			m.lg.Warn("close dedicated pg conn", loggateway.StepID("migrate.conn_close"), loggateway.Err(err))
		}
	}()

	// Disable FK checks for the duration of the migration.
	// session_replication_role = 'replica' tells Postgres to skip FK triggers,
	// which is the standard technique for bulk data loading.
	if err := m.setFKChecks(ctx, false); err != nil {
		return nil, fmt.Errorf("disable fk checks: %w", err)
	}
	defer func() {
		// Use a detached context with timeout for re-enabling FK checks.
		// If the caller's ctx is cancelled (e.g. migration aborted), we still
		// need to restore session_replication_role = 'origin' before returning
		// the connection to the pool, otherwise the pool gets a polluted conn.
		reEnableCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := m.setFKChecks(reEnableCtx, true); err != nil {
			m.lg.Warn("re-enable fk checks failed after migration",
				loggateway.StepID("migrate.fk_reenable"),
				loggateway.Err(err))
		}
	}()

	report := &MigrationReport{TotalTables: len(tables)}

	for _, table := range tables {
		if tableFilter != "" && table != tableFilter {
			report.Skipped = append(report.Skipped, TableSkip{Table: table, Reason: "filtered out (--table)"})
			continue
		}
		if defaultSkipTables[table] || skipSet[table] {
			report.Skipped = append(report.Skipped, TableSkip{Table: table, Reason: "skip list"})
			continue
		}

		rows, err := m.migrateTable(ctx, table)
		if err != nil {
			report.Failed = append(report.Failed, TableFailure{Table: table, Error: err.Error()})
			fmt.Printf("  [FAIL] %s: %v\n", table, err)
			continue
		}
		report.Migrated = append(report.Migrated, TableMigration{Table: table, Rows: rows})
		report.TotalRows += rows
		fmt.Printf("  [OK]   %-40s %d rows\n", table, rows)
	}

	return report, nil
}

// setFKChecks enables or disables FK constraint checks on the Postgres connection.
// Uses SET session_replication_role = 'replica' (disable) / 'origin' (enable).
// Must be called on the dedicated connection (m.tgtConn) to take effect.
func (m *Migrator) setFKChecks(ctx context.Context, enable bool) error {
	role := "origin"
	if !enable {
		role = "replica"
	}
	_, err := m.pg().ExecContext(ctx, fmt.Sprintf("SET session_replication_role = '%s'", role))
	return err
}

// discoverTables returns all user tables from SQLite (excluding sqlite_* internal tables).
func (m *Migrator) discoverTables(ctx context.Context) ([]string, error) {
	rows, err := m.srcDB.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("query sqlite_master: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// migrateTable migrates a single table from SQLite to Postgres.
func (m *Migrator) migrateTable(ctx context.Context, table string) (int64, error) {
	// Resolve target table name (some SQLite tables map to differently-named
	// Postgres tables, e.g. checkpoints -> graph_checkpoints).
	targetTable := resolveTargetTableName(table)

	// Check if table exists in Postgres.
	exists, err := m.tableExistsInPostgres(ctx, targetTable)
	if err != nil {
		return 0, fmt.Errorf("check postgres table: %w", err)
	}
	if !exists {
		return 0, fmt.Errorf("table %s does not exist in Postgres (run with --init-schema --init-framework-schema to create schema)", targetTable)
	}

	// Get common columns (intersection of SQLite and Postgres, excluding GENERATED).
	columns, err := m.getCommonColumns(ctx, table, targetTable)
	if err != nil {
		return 0, fmt.Errorf("get columns: %w", err)
	}
	if len(columns) == 0 {
		return 0, fmt.Errorf("no common columns between SQLite %s and Postgres %s", table, targetTable)
	}

	// Stream rows from SQLite and batch insert into Postgres.
	return m.streamAndInsert(ctx, table, targetTable, columns)
}

// tableExistsInPostgres checks if a table exists in the Postgres public schema.
func (m *Migrator) tableExistsInPostgres(ctx context.Context, table string) (bool, error) {
	var exists bool
	err := m.pg().QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
		table).Scan(&exists)
	return exists, err
}

// getCommonColumns returns the intersection of column names from SQLite and Postgres,
// excluding Postgres GENERATED ALWAYS columns (which cannot be inserted into).
func (m *Migrator) getCommonColumns(ctx context.Context, sqliteTable, pgTable string) ([]string, error) {
	sqliteCols, err := m.getSQLiteColumns(ctx, sqliteTable)
	if err != nil {
		return nil, fmt.Errorf("sqlite columns: %w", err)
	}

	pgCols, err := m.getPostgresInsertableColumns(ctx, pgTable)
	if err != nil {
		return nil, fmt.Errorf("postgres columns: %w", err)
	}

	// Intersection (preserve SQLite order for deterministic column ordering).
	pgSet := make(map[string]bool, len(pgCols))
	for _, c := range pgCols {
		pgSet[c] = true
	}
	var common []string
	for _, c := range sqliteCols {
		if pgSet[c] {
			common = append(common, c)
		}
	}
	return common, nil
}

// getSQLiteColumns returns column names from SQLite's pragma_table_info.
func (m *Migrator) getSQLiteColumns(ctx context.Context, table string) ([]string, error) {
	// Escape single quotes in table name for SQLite string literal safety.
	escapedTable := strings.ReplaceAll(table, `'`, `''`)
	rows, err := m.srcDB.QueryContext(ctx, fmt.Sprintf("SELECT name FROM pragma_table_info('%s') ORDER BY cid", escapedTable))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// getByteaColumns returns the set of bytea column names for the given Postgres
// table. Used to skip UTF-8 sanitization for binary columns during migration.
func (m *Migrator) getByteaColumns(ctx context.Context, table string) (map[string]bool, error) {
	rows, err := m.pg().QueryContext(ctx,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1 AND data_type = 'bytea'`,
		table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// getPostgresInsertableColumns returns column names from Postgres that can be inserted into
// (excludes GENERATED ALWAYS columns).
func (m *Migrator) getPostgresInsertableColumns(ctx context.Context, table string) ([]string, error) {
	rows, err := m.pg().QueryContext(ctx,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1 AND is_generated = 'NEVER'
		 ORDER BY ordinal_position`,
		table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// streamAndInsert streams rows from SQLite and batch-inserts into Postgres.
// Uses []byte scanning to avoid type coercion issues between SQLite (dynamic typing)
// and Postgres (strong typing). The lib/pq driver accepts []byte for most types
// via the text protocol (e.g., "0"/"1" for boolean, ISO timestamps for timestamp).
//
// bytea columns: Postgres bytea columns receive raw []byte without UTF-8
// sanitization to preserve binary data integrity. Text-like columns are
// sanitized via strings.ToValidUTF8 to avoid "invalid UTF8 byte sequence"
// errors on the Postgres side (SQLite may store arbitrary bytes in TEXT).
func (m *Migrator) streamAndInsert(ctx context.Context, sqliteTable, pgTable string, columns []string) (int64, error) {
	// Identify bytea columns on the Postgres target so we can skip UTF-8
	// sanitization for them (sanitizing would corrupt binary payloads).
	byteaSet, err := m.getByteaColumns(ctx, pgTable)
	if err != nil {
		return 0, fmt.Errorf("detect bytea columns: %w", err)
	}

	// Build the SELECT query from SQLite.
	colList := strings.Join(quoteIdentifiers(columns), ", ")
	selectSQL := fmt.Sprintf(`SELECT %s FROM %s`, colList, quoteIdent(sqliteTable))

	// Build the INSERT prefix for Postgres.
	// The VALUES clause and ON CONFLICT are appended per batch.
	pgColList := strings.Join(quoteIdentifiers(columns), ", ")
	insertPrefix := fmt.Sprintf(`INSERT INTO %s (%s) VALUES `, quoteIdent(pgTable), pgColList)

	// Stream rows from SQLite.
	rows, err := m.srcDB.QueryContext(ctx, selectSQL)
	if err != nil {
		return 0, fmt.Errorf("query sqlite: %w", err)
	}
	defer rows.Close()

	// Allocate scan destinations using *[]byte.
	// NULL → nil, non-NULL → []byte (may be empty for empty string).
	scanVals := make([]any, len(columns))
	for i := range scanVals {
		scanVals[i] = new([]byte)
	}

	var totalRows int64
	var batch [][]any

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		n, err := m.execBatch(ctx, insertPrefix, len(columns), batch)
		if err != nil {
			return err
		}
		totalRows += n
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		if err := rows.Scan(scanVals...); err != nil {
			return totalRows, fmt.Errorf("scan row: %w", err)
		}
		// Convert *[]byte to any (nil for NULL, value for data).
		// - bytea columns: pass raw []byte to preserve binary data integrity.
		// - text-like columns: convert to string and sanitize UTF8 because:
		//   1. Postgres text columns require valid UTF8 (SQLite may store invalid bytes).
		//   2. lib/pq sends []byte as bytea and string as text; for text columns,
		//      string avoids "invalid UTF8 byte sequence" errors.
		row := make([]any, len(columns))
		for i, v := range scanVals {
			bp := v.(*[]byte)
			if *bp == nil {
				row[i] = nil // NULL
				continue
			}
			if byteaSet[columns[i]] {
				// Binary column: preserve raw bytes (do not sanitize).
				row[i] = *bp
				continue
			}
			// Text-like column: sanitize invalid UTF8 sequences (replace with \uFFFD).
			row[i] = strings.ToValidUTF8(string(*bp), "\uFFFD")
		}
		batch = append(batch, row)
		if len(batch) >= m.batchSize {
			if err := flush(); err != nil {
				return totalRows, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return totalRows, fmt.Errorf("iterate rows: %w", err)
	}

	if err := flush(); err != nil {
		return totalRows, err
	}

	return totalRows, nil
}

// execBatch executes a multi-row INSERT within a single transaction.
// Returns the number of rows attempted (not necessarily all inserted due to ON CONFLICT DO NOTHING).
func (m *Migrator) execBatch(ctx context.Context, insertPrefix string, nCols int, batch [][]any) (int64, error) {
	nRows := len(batch)

	// Build the multi-row VALUES clause: ($1,$2,...,$N), ($N+1,...,$2N), ...
	var b strings.Builder
	b.Grow(nRows * nCols * 6)
	for i := 0; i < nRows; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('(')
		for j := 0; j < nCols; j++ {
			if j > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "$%d", i*nCols+j+1)
		}
		b.WriteByte(')')
	}
	b.WriteString(" ON CONFLICT DO NOTHING")

	stmt := insertPrefix + b.String()

	// Flatten batch args.
	args := make([]any, 0, nRows*nCols)
	for _, row := range batch {
		args = append(args, row...)
	}

	ctxExec, cancel := context.WithTimeout(ctx, batchInsertTimeout)
	defer cancel()

	tx, err := m.pg().BeginTx(ctxExec, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctxExec, stmt, args...)
	if err != nil {
		return 0, fmt.Errorf("exec batch insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return int64(nRows), nil
}

// quoteIdent wraps a single identifier in double quotes (Postgres-compatible),
// escaping any internal double quotes by doubling them (per SQL standard).
// Use this for table names; use quoteIdentifiers for column lists.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// quoteIdentifiers wraps each identifier in double quotes (Postgres-compatible),
// escaping any internal double quotes by doubling them (per SQL standard).
func quoteIdentifiers(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = quoteIdent(c)
	}
	return out
}
