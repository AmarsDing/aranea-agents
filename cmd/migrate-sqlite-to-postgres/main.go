package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	_ "github.com/glebarez/go-sqlite/compat"
	_ "github.com/lib/pq"
)

// Flags:
//   --source       SQLite DSN or file path (default: file:./data/arenea.sqlite?cache=shared&_fk=1)
//   --target       Postgres DSN (default: from configs/config.yaml data.postgres.source)
//   --mode         migrate | validate | both (default: migrate)
//   --table        Migrate only this table (optional)
//   --batch-size   Rows per INSERT batch (default: 500)
//   --skip-tables  Comma-separated table names to skip
//   --init-schema  Run Ent Schema.Create on Postgres before migration (default: false)
//   --sample-size  Number of rows to sample for validation checksum (default: 100)
func main() {
	source := flag.String("source", "file:./data/arenea.sqlite?cache=shared&_fk=1", "SQLite source DSN or file path")
	target := flag.String("target", "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable", "Postgres target DSN")
	mode := flag.String("mode", "migrate", "migrate | validate | both")
	table := flag.String("table", "", "Migrate only this table (optional)")
	batchSize := flag.Int("batch-size", 500, "Rows per INSERT batch")
	skipTables := flag.String("skip-tables", "", "Comma-separated table names to skip")
	initSchema := flag.Bool("init-schema", false, "Run Ent Schema.Create on Postgres before migration")
	sampleSize := flag.Int("sample-size", 100, "Number of rows to sample for validation checksum")
	flag.Parse()

	lg := loggateway.NewNoop()

	srcDB, err := openSQLite(*source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open sqlite source: %v\n", err)
		os.Exit(1)
	}
	defer srcDB.Close()

	tgtDB, err := openPostgres(*target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open postgres target: %v\n", err)
		os.Exit(1)
	}
	defer tgtDB.Close()

	if *initSchema {
		fmt.Println("=== Initializing Postgres schema via Ent Schema.Create ===")
		if err := initPostgresSchema(context.Background(), tgtDB); err != nil {
			fmt.Fprintf(os.Stderr, "init schema: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Schema initialized.")
	}

	skipSet := parseSkipTables(*skipTables)
	migrator := NewMigrator(srcDB, tgtDB, *batchSize, lg)
	validator := NewValidator(srcDB, tgtDB, *sampleSize, lg)

	wantMigrate := *mode == "migrate" || *mode == "both"
	wantValidate := *mode == "validate" || *mode == "both"

	if wantMigrate {
		fmt.Println("=== Migrating data from SQLite to Postgres ===")
		start := time.Now()
		report, err := migrator.MigrateAll(context.Background(), *table, skipSet)
		if err != nil {
			fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
			os.Exit(1)
		}
		printMigrationReport(report, time.Since(start))
	}

	if wantValidate {
		fmt.Println("\n=== Validating data consistency ===")
		report, err := validator.ValidateAll(context.Background(), *table, skipSet)
		if err != nil {
			fmt.Fprintf(os.Stderr, "validation failed: %v\n", err)
			os.Exit(1)
		}
		printValidationReport(report)
		if !report.AllMatch() {
			os.Exit(1)
		}
	}
}

func openSQLite(dsn string) (*sql.DB, error) {
	if !strings.Contains(dsn, ":") {
		dsn = "file:" + dsn + "?cache=shared&_fk=1"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func openPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

func initPostgresSchema(ctx context.Context, pgDB *sql.DB) error {
	drv := entsql.OpenDB(dialect.Postgres, pgDB)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("ent schema create (postgres): %w", err)
	}
	return nil
}

func parseSkipTables(s string) map[string]bool {
	out := make(map[string]bool)
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			out[t] = true
		}
	}
	return out
}

func printMigrationReport(r *MigrationReport, elapsed time.Duration) {
	fmt.Printf("\nMigration completed in %s\n", elapsed)
	fmt.Printf("  Total tables: %d\n", r.TotalTables)
	fmt.Printf("  Migrated:     %d\n", len(r.Migrated))
	fmt.Printf("  Skipped:      %d\n", len(r.Skipped))
	fmt.Printf("  Failed:       %d\n", len(r.Failed))
	fmt.Printf("  Total rows:   %d\n", r.TotalRows)
	if len(r.Migrated) > 0 {
		fmt.Println("\n  Migrated tables:")
		for _, m := range r.Migrated {
			fmt.Printf("    %-40s %d rows\n", m.Table, m.Rows)
		}
	}
	if len(r.Skipped) > 0 {
		fmt.Println("\n  Skipped tables:")
		for _, s := range r.Skipped {
			fmt.Printf("    %-40s %s\n", s.Table, s.Reason)
		}
	}
	if len(r.Failed) > 0 {
		fmt.Println("\n  Failed tables:")
		for _, f := range r.Failed {
			fmt.Printf("    %-40s %s\n", f.Table, f.Error)
		}
	}
}

func printValidationReport(r *ValidationReport) {
	fmt.Printf("\nValidation results:\n")
	fmt.Printf("  Total tables: %d\n", r.TotalTables)
	fmt.Printf("  Matched:      %d\n", len(r.Matched))
	fmt.Printf("  Mismatched:   %d\n", len(r.Mismatched))
	fmt.Printf("  Skipped:      %d\n", len(r.Skipped))
	if len(r.Mismatched) > 0 {
		fmt.Println("\n  Mismatched tables:")
		for _, m := range r.Mismatched {
			fmt.Printf("    %-40s source=%d target=%d checksum_mismatches=%d\n", m.Table, m.SourceCount, m.TargetCount, m.ChecksumMismatches)
		}
	}
	if len(r.Skipped) > 0 {
		fmt.Println("\n  Skipped tables:")
		for _, s := range r.Skipped {
			fmt.Printf("    %-40s %s\n", s.Table, s.Reason)
		}
	}
}
