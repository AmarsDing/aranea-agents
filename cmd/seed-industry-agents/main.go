package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/scenario/loader"
	"aranea-agents/pkg/loggateway"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "print plan only")
	industry := flag.String("industry", "", "seed specific industry only (softwaredev|selfmedia|finance)")
	teamsOnly := flag.Bool("teams-only", false, "seed teams only (agents must exist)")
	flag.Parse()

	dbPath := resolveSQLitePath()
	fmt.Printf("sqlite: %s\n", dbPath)

	ctx := context.Background()
	entClient, rawDB, cleanup, err := data.OpenSQLiteEntClient(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	store := data.NewCLIData(entClient, rawDB)

	agentRepo := data.NewAgentRepo(store)
	teamRepo := data.NewTeamRepo(store)
	positionRepo := data.NewPositionRepo(store)
	catRepo := data.NewAgentCategoryRepo(store)
	catUC := biz.NewAgentCategoryUsecase(catRepo)
	agentUC := biz.NewAgentUsecase(agentRepo, nil, nil, loggateway.NewNoop())
	teamUC := biz.NewTeamUsecase(teamRepo, nil)
	positionUC := biz.NewPositionUsecase(positionRepo, catUC)

	scenarioDir := resolveScenarioDir()
	industries := []string{"softwaredev", "selfmedia", "finance"}
	if *industry != "" {
		industries = []string{*industry}
	}

	deps := loader.Deps{
		AgentUC:     agentUC,
		TeamUC:      teamUC,
		PositionUC:  positionUC,
		ScenarioDir: scenarioDir,
	}

	totalAgents, totalTeams := 0, 0
	for _, ind := range industries {
		fmt.Printf("\n=== %s ===\n", ind)
		spec, err := loader.LoadIndustrySpec(scenarioDir, ind)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load spec %s: %v\n", ind, err)
			continue
		}
		fmt.Printf("agents: %d, teams: %d\n", len(spec.Agents), len(spec.Teams))

		if *dryRun {
			for _, a := range spec.Agents {
				fmt.Printf("  [agent] %s | %s/%s | pos=%s variant=%s tier=%s\n",
					a.Key, spec.Defaults.Provider, resolveModelName(spec.Defaults, a.ModelTier),
					a.PositionKey, a.Variant, a.ModelTier)
			}
			for _, t := range spec.Teams {
				fmt.Printf("  [team] %s | %s | %d members\n", t.Key, t.Mode, len(t.Members))
			}
			totalAgents += len(spec.Agents)
			totalTeams += len(spec.Teams)
			continue
		}

		if !*teamsOnly {
			ac, tc, err := loader.SeedFromYAML(ctx, deps, ind, false)
			if err != nil {
				fmt.Fprintf(os.Stderr, "seed %s: %v\n", ind, err)
			}
			totalAgents += ac
			totalTeams += tc
		}

		tc, err := loader.SeedTeamsFromYAML(ctx, deps, spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed %s teams: %v\n", ind, err)
		}
		totalTeams += tc
	}

	fmt.Printf("\ndone: %d agents, %d teams\n", totalAgents, totalTeams)
}

func resolveModelName(d loader.AgentDefaults, tier string) string {
	if tier == "strong" {
		return d.StrongModel
	}
	return d.FastModel
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

func resolveScenarioDir() string {
	if v := strings.TrimSpace(os.Getenv("ARANEA_SCENARIO_DIR")); v != "" {
		return v
	}
	wd, _ := os.Getwd()
	return filepath.Join(wd, "internal", "scenario")
}
