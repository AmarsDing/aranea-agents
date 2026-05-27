// migrate-deprecated-prompts consolidates legacy prompt files (SOUL.md, HEARTBEAT.md)
// into the PGO V2 core file set for all agents in the SQLite database.
//
// Commands:
//
//	soul-to-identity  Merge SOUL.md content into IDENTITY.md ## Persona section; delete SOUL.md.
//	prune-heartbeat   Delete HEARTBEAT.md rows (content is obsolete).
//	status            Report how many agents still have deprecated files.
//
// Usage:
//
//	go run ./cmd/migrate-deprecated-prompts status
//	go run ./cmd/migrate-deprecated-prompts soul-to-identity --dry-run
//	go run ./cmd/migrate-deprecated-prompts soul-to-identity --apply
//	go run ./cmd/migrate-deprecated-prompts prune-heartbeat --apply
//
// Safety:
//   - Stop the admin server before --apply to avoid SQLite lock contention.
//   - Override DB path with ARANEA_SQLITE_PATH.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agentpromptfile"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "status":
		runStatus()
	case "soul-to-identity":
		runSoulToIdentity(os.Args[2:])
	case "prune-heartbeat":
		runPruneHeartbeat(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate-deprecated-prompts <status|soul-to-identity|prune-heartbeat> [--dry-run|--apply]")
}

// ── status ───────────────────────────────────────────────────────────────────

func runStatus() {
	client, _, cleanup := mustOpenClient()
	defer cleanup()
	ctx := context.Background()

	souls, err := client.AgentPromptFile.Query().Where(agentpromptfile.FileNameEQ("SOUL.md")).Count(ctx)
	check(err, "count SOUL.md")
	hearts, err := client.AgentPromptFile.Query().Where(agentpromptfile.FileNameEQ("HEARTBEAT.md")).Count(ctx)
	check(err, "count HEARTBEAT.md")

	fmt.Printf("Deprecated prompt files remaining:\n")
	fmt.Printf("  SOUL.md      : %d agents\n", souls)
	fmt.Printf("  HEARTBEAT.md : %d agents\n", hearts)
	if souls == 0 && hearts == 0 {
		fmt.Println("All clear — no deprecated files found.")
	}
}

// ── soul-to-identity ─────────────────────────────────────────────────────────

func runSoulToIdentity(args []string) {
	fs := flag.NewFlagSet("soul-to-identity", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print actions without applying")
	apply := fs.Bool("apply", false, "apply the migration")
	_ = fs.Parse(args)
	if *dryRun == *apply {
		fmt.Fprintln(os.Stderr, "specify exactly one of --dry-run or --apply")
		os.Exit(2)
	}

	client, _, cleanup := mustOpenClient()
	defer cleanup()
	ctx := context.Background()

	souls, err := client.AgentPromptFile.Query().
		Where(agentpromptfile.FileNameEQ("SOUL.md")).
		All(ctx)
	check(err, "query SOUL.md rows")

	if len(souls) == 0 {
		fmt.Println("No SOUL.md rows found — nothing to migrate.")
		return
	}

	merged, skipped, errs := 0, 0, 0
	for _, soul := range souls {
		agentID := soul.AgentID
		soulBody := strings.TrimSpace(soul.Body)

		// Find IDENTITY.md for this agent.
		identity, err := client.AgentPromptFile.Query().
			Where(
				agentpromptfile.AgentIDEQ(agentID),
				agentpromptfile.FileNameEQ("IDENTITY.md"),
			).
			Only(ctx)

		if err != nil {
			// If IDENTITY.md doesn't exist, we still want to at minimum delete SOUL.md
			// to avoid orphan files; log a warning and skip merge.
			fmt.Printf("[WARN ] agent=%s  IDENTITY.md not found — skipping merge (SOUL.md body lost)\n", agentID)
			if !*dryRun {
				_ = client.AgentPromptFile.DeleteOne(soul).Exec(ctx)
			}
			skipped++
			continue
		}

		newBody := injectPersonaSection(identity.Body, soulBody)

		if *dryRun {
			fmt.Printf("[DRY  ] agent=%s  would merge SOUL.md into IDENTITY.md ## Persona (%d chars → %d chars)\n",
				agentID, len(identity.Body), len(newBody))
			merged++
			continue
		}

		// Apply: update IDENTITY.md and delete SOUL.md in a transaction.
		txErr := withTx(ctx, client, func(tx *ent.Tx) error {
			_, err := tx.AgentPromptFile.UpdateOne(identity).
				SetBody(newBody).
				Save(ctx)
			if err != nil {
				return fmt.Errorf("update IDENTITY.md: %w", err)
			}
			return tx.AgentPromptFile.DeleteOne(soul).Exec(ctx)
		})
		if txErr != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] agent=%s  %v\n", agentID, txErr)
			errs++
			continue
		}
		fmt.Printf("[DONE ] agent=%s  merged SOUL.md → IDENTITY.md ## Persona\n", agentID)
		merged++
	}

	fmt.Printf("\nSummary: merged=%d skipped=%d errors=%d\n", merged, skipped, errs)
	if errs > 0 {
		os.Exit(1)
	}
}

// ── prune-heartbeat ──────────────────────────────────────────────────────────

func runPruneHeartbeat(args []string) {
	fs := flag.NewFlagSet("prune-heartbeat", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print actions without applying")
	apply := fs.Bool("apply", false, "apply the migration")
	_ = fs.Parse(args)
	if *dryRun == *apply {
		fmt.Fprintln(os.Stderr, "specify exactly one of --dry-run or --apply")
		os.Exit(2)
	}

	client, _, cleanup := mustOpenClient()
	defer cleanup()
	ctx := context.Background()

	rows, err := client.AgentPromptFile.Query().
		Where(agentpromptfile.FileNameEQ("HEARTBEAT.md")).
		All(ctx)
	check(err, "query HEARTBEAT.md rows")

	if len(rows) == 0 {
		fmt.Println("No HEARTBEAT.md rows found — nothing to prune.")
		return
	}

	if *dryRun {
		for _, r := range rows {
			fmt.Printf("[DRY  ] agent=%s  would delete HEARTBEAT.md (%d chars)\n", r.AgentID, len(r.Body))
		}
		fmt.Printf("\nWould delete %d HEARTBEAT.md rows.\n", len(rows))
		return
	}

	n, err := client.AgentPromptFile.Delete().
		Where(agentpromptfile.FileNameEQ("HEARTBEAT.md")).
		Exec(ctx)
	check(err, "delete HEARTBEAT.md rows")
	fmt.Printf("Deleted %d HEARTBEAT.md rows.\n", n)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// personaRe matches a ## Persona section up to the next ## heading or EOF.
var personaRe = regexp.MustCompile(`(?ms)^##\s+Persona\b.*?(?:^##|\z)`)

// injectPersonaSection merges soulContent into the ## Persona section of identityBody.
// If a ## Persona section already exists, its body is replaced.
// If it doesn't exist, the section is appended.
func injectPersonaSection(identityBody, soulContent string) string {
	if strings.TrimSpace(soulContent) == "" {
		return identityBody
	}

	personaBlock := "## Persona\n\n" + strings.TrimSpace(soulContent) + "\n"

	if personaRe.MatchString(identityBody) {
		// Replace existing section, preserving the next ## heading.
		return personaRe.ReplaceAllStringFunc(identityBody, func(match string) string {
			// Check if the match ends with a ## heading (separator for next section).
			if strings.HasSuffix(strings.TrimSpace(match), "##") || strings.Contains(match[len(match)-5:], "##") {
				nextHeading := match[strings.LastIndex(match, "##"):]
				return personaBlock + "\n" + nextHeading
			}
			return personaBlock
		})
	}

	// Append.
	body := strings.TrimRight(identityBody, "\n")
	return body + "\n\n" + personaBlock
}

// withTx runs fn inside an ent transaction, rolling back on error.
func withTx(ctx context.Context, client *ent.Client, fn func(*ent.Tx) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func mustOpenClient() (*ent.Client, interface{}, func()) {
	dsn := defaultSQLiteDSN()
	client, _, cleanup, err := data.OpenSQLiteEntClient(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	return client, nil, cleanup
}

func defaultSQLiteDSN() string {
	if v := strings.TrimSpace(os.Getenv("ARANEA_SQLITE_PATH")); v != "" {
		return v
	}
	root, _ := os.Getwd()
	return filepath.Join(root, "cmd", "data", "arenea.sqlite")
}

func check(err error, msg string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
		os.Exit(1)
	}
}
