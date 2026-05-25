package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/team"
)

func loadAgentIDMap(ctx context.Context, client *ent.Client) (agentIDMap, error) {
	rows, err := client.Agent.Query().
		Where(agent.DeletedAtEQ("")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := agentIDMap{}
	for _, row := range rows {
		out[row.AgentKey] = row.ID
	}
	return out, nil
}

func seedTeams(ctx context.Context, entClient *ent.Client, store *data.Data, ids agentIDMap, update bool, dryRun bool) error {
	specs, err := buildTeamSpecs(ids)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Println("--- teams ---")
		for _, t := range specs {
			fmt.Printf("%s | %s | mode=%s members=%d\n", t.teamKey, t.displayName, t.spec.Mode, len(t.spec.Members))
		}
		return nil
	}

	teamRepo := data.NewTeamRepo(store)
	teamUC := biz.NewTeamUsecase(teamRepo)
	now := time.Now().UTC().Format(time.RFC3339)

	for _, ts := range specs {
		defJSON, err := biz.OrchestrationSpecToDefinitionJSON(ts.spec)
		if err != nil {
			return fmt.Errorf("team %s definition: %w", ts.teamKey, err)
		}
		defJSON = biz.EnsureGraphRuntimeDefault(defJSON)

		existingID, err := findTeamByKey(ctx, entClient, ts.teamKey)
		if err != nil {
			return err
		}
		if existingID != "" {
			if !update {
				fmt.Printf("skip team (exists): %s\n", ts.teamKey)
				continue
			}
			updated, err := teamUC.Update(ctx, existingID, biz.Team{
				DisplayName:    ts.displayName,
				DefinitionJSON: defJSON,
				ADKAppName:     ts.teamKey,
			})
			if err != nil {
				return fmt.Errorf("update team %s: %w", ts.teamKey, err)
			}
			fmt.Printf("updated team: %s (%s) mode=%s members=%d\n",
				ts.teamKey, updated.ID, ts.spec.Mode, len(ts.spec.Members))
			continue
		}

		created, err := teamUC.Create(ctx, biz.Team{
			ID:             newID(),
			TeamKey:        ts.teamKey,
			DisplayName:    ts.displayName,
			Status:         "active",
			DefinitionJSON: defJSON,
			ADKAppName:     ts.teamKey,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		if err != nil {
			return fmt.Errorf("create team %s: %w", ts.teamKey, err)
		}
		fmt.Printf("created team: %s (%s) mode=%s members=%d\n",
			ts.teamKey, created.ID, ts.spec.Mode, len(ts.spec.Members))
	}
	return nil
}

func findTeamByKey(ctx context.Context, client *ent.Client, key string) (string, error) {
	row, err := client.Team.Query().
		Where(team.TeamKeyEQ(key), team.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return row.ID, nil
}

func seedTeamsOrExit(ctx context.Context, entClient *ent.Client, store *data.Data, update, dryRun bool) {
	ids, err := loadAgentIDMap(ctx, entClient)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load agents: %v\n", err)
		os.Exit(1)
	}
	if err := seedTeams(ctx, entClient, store, ids, update, dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "seed teams: %v\n", err)
		os.Exit(1)
	}
}
