package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// ValidationReport summarizes the validation outcome.
type ValidationReport struct {
	TotalTables int
	Matched     []TableMatch
	Mismatched  []TableMismatch
	Skipped     []TableSkip
}

// TableMatch records a table that passed validation.
type TableMatch struct {
	Table      string
	SourceRows int64
	TargetRows int64
}

// TableMismatch records a table that failed validation.
type TableMismatch struct {
	Table              string
	SourceCount        int64
	TargetCount        int64
	ChecksumMismatches int
}

// AllMatch reports whether all validated tables matched.
func (r *ValidationReport) AllMatch() bool {
	return len(r.Mismatched) == 0
}

// Validator compares data between SQLite source and Postgres target.
type Validator struct {
	srcDB      *sql.DB
	tgtDB      *sql.DB
	sampleSize int
	lg         loggateway.Logger
}

// NewValidator creates a new Validator.
func NewValidator(srcDB, tgtDB *sql.DB, sampleSize int, lg loggateway.Logger) *Validator {
	if sampleSize <= 0 {
		sampleSize = 100
	}
	return &Validator{srcDB: srcDB, tgtDB: tgtDB, sampleSize: sampleSize, lg: lg}
}

// ValidateAll validates all tables (or a single table if tableFilter is non-empty).
func (v *Validator) ValidateAll(ctx context.Context, tableFilter string, skipSet map[string]bool) (*ValidationReport, error) {
	tables, err := v.discoverTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover tables: %w", err)
	}

	report := &ValidationReport{TotalTables: len(tables)}

	for _, table := range tables {
		if tableFilter != "" && table != tableFilter {
			report.Skipped = append(report.Skipped, TableSkip{Table: table, Reason: "filtered out (--table)"})
			continue
		}
		if defaultSkipTables[table] || skipSet[table] {
			report.Skipped = append(report.Skipped, TableSkip{Table: table, Reason: "skip list"})
			continue
		}

		match, err := v.validateTable(ctx, table)
		if err != nil {
			report.Mismatched = append(report.Mismatched, TableMismatch{
				Table:       table,
				SourceCount: -1,
				TargetCount: -1,
			})
			fmt.Printf("  [ERR]  %-40s %v\n", table, err)
			continue
		}

		if match.SourceRows == match.TargetRows && match.ChecksumMismatches == 0 {
			report.Matched = append(report.Matched, TableMatch{
				Table:      table,
				SourceRows: match.SourceRows,
				TargetRows: match.TargetRows,
			})
			fmt.Printf("  [OK]   %-40s rows=%d\n", table, match.SourceRows)
		} else {
			report.Mismatched = append(report.Mismatched, TableMismatch{
				Table:              table,
				SourceCount:        match.SourceRows,
				TargetCount:        match.TargetRows,
				ChecksumMismatches: match.ChecksumMismatches,
			})
			fmt.Printf("  [FAIL] %-40s source=%d target=%d checksum_mismatches=%d\n",
				table, match.SourceRows, match.TargetRows, match.ChecksumMismatches)
		}
	}

	return report, nil
}

// discoverTables returns all user tables from SQLite (excluding sqlite_* internal tables).
func (v *Validator) discoverTables(ctx context.Context) ([]string, error) {
	rows, err := v.srcDB.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
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

// tableValidationResult holds the result of validating a single table.
type tableValidationResult struct {
	SourceRows         int64
	TargetRows         int64
	ChecksumMismatches int
}

// validateTable validates a single table by comparing row counts and sample checksums.
func (v *Validator) validateTable(ctx context.Context, table string) (*tableValidationResult, error) {
	// Resolve target table name (some SQLite tables map to differently-named
	// Postgres tables, e.g. checkpoints -> graph_checkpoints).
	targetTable := resolveTargetTableName(table)

	// Check if table exists in Postgres.
	exists, err := v.tableExistsInPostgres(ctx, targetTable)
	if err != nil {
		return nil, fmt.Errorf("check postgres table: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("table %s does not exist in Postgres", targetTable)
	}

	// Compare row counts. SQLite queries use the source table name;
	// Postgres queries use the resolved target table name.
	srcCount, err := v.countRows(ctx, v.srcDB, table, true)
	if err != nil {
		return nil, fmt.Errorf("count sqlite rows: %w", err)
	}
	tgtCount, err := v.countRows(ctx, v.tgtDB, targetTable, false)
	if err != nil {
		return nil, fmt.Errorf("count postgres rows: %w", err)
	}

	result := &tableValidationResult{SourceRows: srcCount, TargetRows: tgtCount}

	// If row counts differ, no need to check checksums.
	if srcCount != tgtCount {
		return result, nil
	}

	// If no rows, skip checksum validation.
	if srcCount == 0 {
		return result, nil
	}

	// Get common columns for checksum comparison.
	migrator := &Migrator{srcDB: v.srcDB, tgtDB: v.tgtDB, batchSize: 1, lg: v.lg}
	columns, err := migrator.getCommonColumns(ctx, table, targetTable)
	if err != nil {
		return nil, fmt.Errorf("get common columns: %w", err)
	}
	if len(columns) == 0 {
		return result, nil
	}

	// Sample rows and compare checksums.
	mismatches, err := v.compareSampleChecksums(ctx, table, targetTable, columns)
	if err != nil {
		return nil, fmt.Errorf("compare checksums: %w", err)
	}
	result.ChecksumMismatches = mismatches

	return result, nil
}

// tableExistsInPostgres checks if a table exists in the Postgres public schema.
func (v *Validator) tableExistsInPostgres(ctx context.Context, table string) (bool, error) {
	var exists bool
	err := v.tgtDB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
		table).Scan(&exists)
	return exists, err
}

// countRows returns the row count for a table.
// isSQLite selects the SQL dialect (SQLite uses "table" quoting, Postgres uses "table" too).
func (v *Validator) countRows(ctx context.Context, db *sql.DB, table string, isSQLite bool) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)).Scan(&count)
	return count, err
}

// compareSampleChecksums compares checksums of sample rows between source and target.
// It reads up to sampleSize rows from each side (ordered by the first column) and
// compares a SHA-256 checksum of the concatenated row values.
func (v *Validator) compareSampleChecksums(ctx context.Context, sqliteTable, pgTable string, columns []string) (int, error) {
	colList := strings.Join(quoteIdentifiers(columns), ", ")
	// Order by all columns to ensure deterministic ordering.
	orderBy := strings.Join(quoteIdentifiers(columns), ", ")

	// Read sample from SQLite.
	srcSQL := fmt.Sprintf(`SELECT %s FROM "%s" ORDER BY %s LIMIT %d`, colList, sqliteTable, orderBy, v.sampleSize)
	srcChecksums, err := v.computeRowChecksums(ctx, v.srcDB, srcSQL, len(columns))
	if err != nil {
		return 0, fmt.Errorf("source checksums: %w", err)
	}

	// Read sample from Postgres.
	tgtSQL := fmt.Sprintf(`SELECT %s FROM "%s" ORDER BY %s LIMIT %d`, colList, pgTable, orderBy, v.sampleSize)
	tgtChecksums, err := v.computeRowChecksums(ctx, v.tgtDB, tgtSQL, len(columns))
	if err != nil {
		return 0, fmt.Errorf("target checksums: %w", err)
	}

	// Compare checksums.
	mismatches := 0
	maxLen := len(srcChecksums)
	if len(tgtChecksums) > maxLen {
		maxLen = len(tgtChecksums)
	}
	for i := 0; i < maxLen; i++ {
		var srcHash, tgtHash string
		if i < len(srcChecksums) {
			srcHash = srcChecksums[i]
		}
		if i < len(tgtChecksums) {
			tgtHash = tgtChecksums[i]
		}
		if srcHash != tgtHash {
			mismatches++
		}
	}
	return mismatches, nil
}

// computeRowChecksums computes a SHA-256 checksum for each row.
// Each row's checksum is based on the concatenation of all column values (as strings).
// NULL values are represented as "\x00" to distinguish from empty string.
func (v *Validator) computeRowChecksums(ctx context.Context, db *sql.DB, query string, nCols int) ([]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scanVals := make([]any, nCols)
	for i := range scanVals {
		scanVals[i] = new([]byte)
	}

	var checksums []string
	for rows.Next() {
		if err := rows.Scan(scanVals...); err != nil {
			return nil, err
		}
		h := sha256.New()
		for i, v := range scanVals {
			if i > 0 {
				h.Write([]byte{0x1f}) // field separator
			}
			bp := v.(*[]byte)
			if *bp == nil {
				h.Write([]byte{0x00}) // NULL marker
			} else {
				h.Write(*bp)
			}
		}
		checksums = append(checksums, hex.EncodeToString(h.Sum(nil)))
	}
	return checksums, rows.Err()
}
