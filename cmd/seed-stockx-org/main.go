// seed-stockx-org seeds Daily Stock Analysis org tree + fully configured agents into SQLite.
//
// Usage:
//
//	go run ./cmd/seed-stockx-org
//	go run ./cmd/seed-stockx-org --dry-run
//	go run ./cmd/seed-stockx-org --update
//	go run ./cmd/seed-stockx-org --teams-only
//
// Stop `go run ./cmd/admin` before apply (SQLite lock on Windows).
// Override DB: ARANEA_SQLITE_PATH=data/arenea.sqlite
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agentcategory"
	"aranea-agents/internal/data/ent/llmprovidermodel"
	"aranea-agents/pkg/loggateway"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "print plan only")
	update := flag.Bool("update", false, "update existing agents/teams and position role configs")
	teamsOnly := flag.Bool("teams-only", false, "seed teams only (agents must exist)")
	flag.Parse()

	dbPath := resolveSQLitePath()
	fmt.Printf("sqlite: %s\n", dbPath)
	if *dryRun {
		fmt.Println("mode: dry-run")
	}

	ctx := context.Background()
	entClient, rawDB, cleanup, err := data.OpenSQLiteEntClient(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	store := data.NewCLIData(entClient, rawDB)

	if *teamsOnly || *dryRun {
		if *dryRun && !*teamsOnly {
			fastP, fastM, strongP, strongM := resolveModels(ctx, entClient)
			fmt.Printf("models: fast=%s/%s strong=%s/%s\n", fastP, fastM, strongP, strongM)
			printPlan(buildPlan(fastP, fastM, strongP, strongM))
		}
		seedTeamsOrExit(ctx, entClient, store, *update, *dryRun)
		if *dryRun || *teamsOnly {
			if !*dryRun {
				fmt.Println("done")
			}
			return
		}
	}

	fastP, fastM, strongP, strongM := resolveModels(ctx, entClient)
	fmt.Printf("models: fast=%s/%s strong=%s/%s\n", fastP, fastM, strongP, strongM)

	plan := buildPlan(fastP, fastM, strongP, strongM)
	catRepo := data.NewAgentCategoryRepo(store)
	agentRepo := data.NewAgentRepo(store)
	catUC := biz.NewAgentCategoryUsecase(catRepo)
	agentUC := biz.NewAgentUsecase(agentRepo, nil, nil, loggateway.NewNoop())

	idByKey := map[string]string{}
	now := time.Now().UTC().Format(time.RFC3339)

	for _, node := range plan.categories {
		existing, err := findCategoryByKey(ctx, entClient, node.key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lookup %s: %v\n", node.key, err)
			os.Exit(1)
		}
		if existing != "" {
			idByKey[node.key] = existing
			if *update {
				if node.level == "position" && len(node.roleConfig) > 0 {
					if err := patchCategoryRoleConfig(ctx, entClient, existing, node.roleConfig, now); err != nil {
						fmt.Fprintf(os.Stderr, "update category %s: %v\n", node.key, err)
						os.Exit(1)
					}
					fmt.Printf("updated position role config: %s\n", node.key)
				}
				if err := patchCategoryEnabled(ctx, entClient, existing, true, now); err != nil {
					fmt.Fprintf(os.Stderr, "enable category %s: %v\n", node.key, err)
					os.Exit(1)
				}
				fmt.Printf("ensured category enabled: %s\n", node.key)
			} else {
				fmt.Printf("skip category (exists): %s\n", node.key)
			}
			continue
		}
		parentID := ""
		if node.parentKey != "" {
			parentID = idByKey[node.parentKey]
			if parentID == "" {
				fmt.Fprintf(os.Stderr, "missing parent %s for %s\n", node.parentKey, node.key)
				os.Exit(1)
			}
		}
		cfgJSON := roleConfigJSON(node.roleConfig)
		created, err := catUC.Create(ctx, biz.AgentCategory{
			ID:           newID(),
			Key:          node.key,
			Name:         node.name,
			Description:  node.description,
			Enabled:      true,
			SortOrder:    node.sortOrder,
			ParentID:    parentID,
			Level:       node.level,
			IsSystem:    true,
			ConfigJSON:  cfgJSON,
			MetadataJSON: cfgJSON,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "create category %s: %v\n", node.key, err)
			os.Exit(1)
		}
		idByKey[node.key] = created.ID
		fmt.Printf("created category: %s (%s)\n", node.key, created.ID)
	}

	for _, spec := range plan.agents {
		posID := idByKey[spec.positionKey]
		if posID == "" {
			// position may exist from prior run but wasn't in idByKey if parent lookup failed — resolve again
			if id, err := findCategoryByKey(ctx, entClient, spec.positionKey); err == nil && id != "" {
				posID = id
				idByKey[spec.positionKey] = id
			}
		}
		if posID == "" {
			fmt.Fprintf(os.Stderr, "missing position %s for agent %s\n", spec.positionKey, spec.agentKey)
			os.Exit(1)
		}

		existing, err := agentRepo.GetAgentByAgentKey(ctx, spec.agentKey)
		if err == nil {
			if !*update {
				fmt.Printf("skip agent (exists): %s\n", spec.agentKey)
				continue
			}
			patch := spec.toBizAgent(posID, "seed-stockx-org")
			patch.ID = existing.ID
			patch.CreatedAt = existing.CreatedAt
			patch.UpdatedAt = now
			updated, err := agentUC.Update(ctx, existing.ID, patch)
			if err != nil {
				fmt.Fprintf(os.Stderr, "update agent %s: %v\n", spec.agentKey, err)
				os.Exit(1)
			}
			fmt.Printf("updated agent: %s (%s) files=%d\n", spec.agentKey, updated.ID, len(updated.Files))
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			fmt.Fprintf(os.Stderr, "lookup agent %s: %v\n", spec.agentKey, err)
			os.Exit(1)
		}

		payload := spec.toBizAgent(posID, "seed-stockx-org")
		payload.ID = newID()
		payload.CreatedAt = now
		payload.UpdatedAt = now
		created, err := agentUC.Create(ctx, payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create agent %s: %v\n", spec.agentKey, err)
			os.Exit(1)
		}
		fmt.Printf("created agent: %s (%s) -> %s files=%d\n", spec.agentKey, created.ID, spec.positionKey, len(created.Files))
	}

	seedTeamsOrExit(ctx, entClient, store, *update, false)
	fmt.Println("done")
}

func resolveModels(ctx context.Context, client *ent.Client) (fastP, fastM, strongP, strongM string) {
	fastP = envOr("STOCKX_SEED_PROVIDER", "")
	fastM = envOr("STOCKX_SEED_MODEL", "")
	strongP = envOr("STOCKX_SEED_STRONG_PROVIDER", fastP)
	strongM = envOr("STOCKX_SEED_STRONG_MODEL", "")

	if fastP != "" && fastM != "" {
		if strongM == "" {
			strongM = fastM
		}
		if strongP == "" {
			strongP = fastP
		}
		return fastP, fastM, strongP, strongM
	}

	rows, err := client.LlmProviderModel.Query().
		Where(
			llmprovidermodel.DeletedAtEQ(""),
			llmprovidermodel.EnabledEQ(true),
		).
		Order(llmprovidermodel.BySortOrder()).
		All(ctx)
	if err != nil || len(rows) == 0 {
		return "openai", "gpt-4o-mini", "openai", "gpt-4o"
	}

	fastP, fastM = rows[0].Provider, rows[0].Model
	strongP, strongM = fastP, fastM
	for _, row := range rows {
		name := strings.ToLower(row.Model + " " + row.Name)
		if strings.Contains(name, "gpt-4o") && !strings.Contains(name, "mini") {
			strongP, strongM = row.Provider, row.Model
			break
		}
		if strings.Contains(name, "claude") && strings.Contains(name, "opus") {
			strongP, strongM = row.Provider, row.Model
			break
		}
	}
	return fastP, fastM, strongP, strongM
}

func patchCategoryRoleConfig(ctx context.Context, client *ent.Client, id string, role map[string]any, now string) error {
	raw := roleConfigJSON(role)
	_, err := client.AgentCategory.UpdateOneID(id).
		SetConfigJSON(raw).
		SetMetadataJSON(raw).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func patchCategoryEnabled(ctx context.Context, client *ent.Client, id string, enabled bool, now string) error {
	row, err := client.AgentCategory.Get(ctx, id)
	if err != nil {
		return err
	}
	if row.Enabled == enabled {
		return nil
	}
	_, err = client.AgentCategory.UpdateOneID(id).
		SetEnabled(enabled).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func resolveSQLitePath() string {
	if v := strings.TrimSpace(os.Getenv("ARANEA_SQLITE_PATH")); v != "" {
		if strings.HasPrefix(v, "file:") {
			return strings.TrimPrefix(v, "file:")
		}
		return v
	}
	wd, _ := os.Getwd()
	candidates := []string{
		filepath.Join(wd, "data", "arenea.sqlite"),
		filepath.Join(wd, "cmd", "data", "arenea.sqlite"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return candidates[0]
}

func findCategoryByKey(ctx context.Context, client *ent.Client, key string) (string, error) {
	row, err := client.AgentCategory.Query().
		Where(
			agentcategory.CategoryKeyEQ(key),
			agentcategory.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return row.ID, nil
}

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func printPlan(plan seedPlan) {
	fmt.Println("--- categories ---")
	for _, c := range plan.categories {
		parent := c.parentKey
		if parent == "" {
			parent = "(root)"
		}
		fmt.Printf("[%s] %s (%s) parent=%s order=%d\n", c.level, c.name, c.key, parent, c.sortOrder)
	}
	fmt.Println("--- agents ---")
	for _, a := range plan.agents {
		fmt.Printf("%s | %s | pos=%s | %s/%s | tools=%d skills=%d\n",
			a.agentKey, a.displayName, a.positionKey, a.provider, a.model,
			len(a.toolsAllow), len(a.skillsAllow))
	}
}
