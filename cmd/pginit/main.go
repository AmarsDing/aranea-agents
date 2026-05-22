// pginit ensures the aranea Postgres database and pgvector schemas from configs.
// Usage: go run ./cmd/pginit -conf ./configs/config1.yaml
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/pgvector"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	_ "github.com/lib/pq"
)

var flagconf string

func init() {
	flag.StringVar(&flagconf, "conf", "./configs/config1.yaml", "config file path")
}

func main() {
	flag.Parse()
	cfg, err := loadBootstrap(flagconf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	d := cfg.GetData()
	if d == nil || d.GetPostgres() == nil {
		fmt.Fprintln(os.Stderr, "data.postgres.source is empty in config")
		os.Exit(1)
	}
	dsn := strings.TrimSpace(d.GetPostgres().GetSource())
	dim := int(d.GetPostgres().GetVectorDim())
	if dim <= 0 {
		dim = 1536
	}

	if err := ensureDatabase(dsn); err != nil {
		fmt.Fprintf(os.Stderr, "ensure database: %v\n", err)
		os.Exit(1)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open postgres: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	if err = db.PingContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "ping postgres: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err = pgvector.EnsureSchema(ctx, db, dim); err != nil {
		fmt.Fprintf(os.Stderr, "agent memory schema: %v\n", err)
		os.Exit(1)
	}
	if err = data.EnsureKnowledgeSchema(ctx, db, dim); err != nil {
		fmt.Fprintf(os.Stderr, "knowledge schema: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("ok: postgres ready (vector_dim=%d, table=%s)\n", dim, pgvector.TableNameForDimension(dim))
}

func loadBootstrap(path string) (*conf.Bootstrap, error) {
	c := config.New(config.WithSource(file.NewSource(path)))
	defer c.Close()
	if err := c.Load(); err != nil {
		return nil, err
	}
	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		return nil, err
	}
	return &bc, nil
}

func ensureDatabase(targetDSN string) error {
	u, err := url.Parse(targetDSN)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}
	dbName := strings.TrimPrefix(strings.TrimSpace(u.Path), "/")
	if dbName == "" {
		return fmt.Errorf("dsn missing database name in path")
	}
	u.Path = "/postgres"
	adminDSN := u.String()

	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		return err
	}
	defer admin.Close()
	if err = admin.PingContext(context.Background()); err != nil {
		return fmt.Errorf("ping admin db: %w", err)
	}

	var exists int
	q := `SELECT 1 FROM pg_database WHERE datname = $1`
	if err = admin.QueryRowContext(context.Background(), q, dbName).Scan(&exists); err == sql.ErrNoRows {
		// CREATE DATABASE cannot run in a transaction; identifier is from config only.
		ddl := fmt.Sprintf(`CREATE DATABASE %s`, quoteIdent(dbName))
		if _, err = admin.ExecContext(context.Background(), ddl); err != nil {
			return fmt.Errorf("create database %q: %w", dbName, err)
		}
		fmt.Printf("created database %q\n", dbName)
		return nil
	}
	if err != nil {
		return fmt.Errorf("lookup database: %w", err)
	}
	fmt.Printf("database %q already exists\n", dbName)
	return nil
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
