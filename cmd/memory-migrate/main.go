// memory-migrate runs offline Memory data migrations against the project SQLite DB.
//
// Usage:
//
//	go run ./cmd/memory-migrate legacy-trpc-facts --dry-run
//	go run ./cmd/memory-migrate legacy-trpc-facts --apply
//
// Safety:
//   - Stop `go run ./cmd/admin` (or any process holding the same SQLite file) before --apply.
//   - Parallel opens of the same DSN cause SQLite lock contention on Windows.
//   - Override DB path with ARANEA_SQLITE_PATH (file path or file: DSN).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/data"
	"aranea-agents/internal/data/sessionmemory"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "legacy-trpc-facts":
		runLegacyTRPCFacts(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: go run ./cmd/memory-migrate legacy-trpc-facts [--dry-run|--apply]\n")
}

func runLegacyTRPCFacts(args []string) {
	fs := flag.NewFlagSet("legacy-trpc-facts", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "report pending rows only")
	apply := fs.Bool("apply", false, "run migration when not yet applied")
	_ = fs.Parse(args)
	if *dryRun == *apply {
		fmt.Fprintln(os.Stderr, "specify exactly one of --dry-run or --apply")
		os.Exit(2)
	}

	store, cleanup, err := openProjectStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	ctx := context.Background()
	status, err := data.GetLegacyTRPCMigrationStatus(ctx, store)
	if err != nil {
		fmt.Fprintf(os.Stderr, "status: %v\n", err)
		os.Exit(1)
	}
	if status.Applied {
		fmt.Printf("migration %d already applied; pending=%d\n", data.MigrationLegacyTRPCMemoryFacts, status.Pending)
		return
	}
	fmt.Printf("pending legacy trpc_memory entities: %d\n", status.Pending)
	if *dryRun {
		return
	}
	migrated, skipped, err := data.RunLegacyTRPCMemoryMigration(ctx, store)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apply: %v\n", err)
		os.Exit(1)
	}
	if skipped {
		fmt.Println("skipped (already applied)")
		return
	}
	fmt.Printf("migrated=%d\n", migrated)
}

func openProjectStore() (*sessionmemory.Store, func(), error) {
	dsn := defaultSQLiteDSN()
	client, _, cleanup, err := data.OpenSQLiteEntClient(dsn)
	if err != nil {
		return nil, nil, err
	}
	return sessionmemory.NewStore(client), cleanup, nil
}

func defaultSQLiteDSN() string {
	if v := strings.TrimSpace(os.Getenv("ARANEA_SQLITE_PATH")); v != "" {
		return v
	}
	root, _ := os.Getwd()
	return filepath.Join(root, "cmd", "data", "arenea.sqlite")
}
