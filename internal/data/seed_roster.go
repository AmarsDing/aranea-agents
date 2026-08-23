package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/agentruntimesetting"
	"aranea-agents/internal/data/ent/organization"
	"aranea-agents/pkg/loggateway"
)

// SeedRosterIdentity backfills domain_path / mission_statement / tools_profile
// for catalog agents that were imported before roster fields were wired.
// Only fills empty (or illegal "general") values — never overwrites a real specialty.
func SeedRosterIdentity(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	depts := map[string]string{} // positionID → department key
	deptNodes, err := tx.Organization.Query().
		Where(organization.DeletedAtEQ(""), organization.LevelEQ("department")).
		All(ctx)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	deptByID := map[string]string{}
	for _, d := range deptNodes {
		deptByID[d.ID] = d.OrgKey
	}
	posNodes, err := tx.Organization.Query().
		Where(organization.DeletedAtEQ(""), organization.LevelEQ("position")).
		All(ctx)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	for _, p := range posNodes {
		depts[p.ID] = deptByID[p.ParentID]
	}

	rows, err := tx.Agent.Query().
		Where(agent.DeletedAtEQ(""), agent.StatusEQ("active")).
		All(ctx)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated := 0
	failed := 0
	intended := 0
	for _, row := range rows {
		if biz.IsSystemAgentKey(row.AgentKey) || biz.IsOrgGovernanceAgent(biz.Agent{AgentKey: row.AgentKey, AgentVariant: string(row.AgentVariant)}) {
			continue
		}
		deptKey := depts[row.PositionID]
		needPath := strings.TrimSpace(row.DomainPath) == ""
		needMission := strings.TrimSpace(row.MissionStatement) == ""
		path := row.DomainPath
		if needPath {
			path = biz.InferDomainPath(row.PositionKey, deptKey, row.DisplayName)
		}
		mission := row.MissionStatement
		if needMission {
			mission = biz.InferMissionStatement(row.DisplayName, row.AgentDescription)
		}
		if (needPath && path != "") || (needMission && mission != "") {
			intended++
			upd := tx.Agent.UpdateOneID(row.ID).SetUpdatedAt(now)
			if needPath && path != "" {
				upd.SetDomainPath(path)
			}
			if needMission && mission != "" {
				upd.SetMissionStatement(mission)
			}
			if err := upd.Exec(ctx); err != nil {
				failed++
				lg.Error("roster backfill agent failed",
					loggateway.StepID("data.seed.roster_identity"),
					loggateway.Str("agent_key", row.AgentKey),
					loggateway.Err(err),
				)
				continue
			}
			updated++
		}
		if path == "" {
			continue
		}
		st, err := tx.AgentRuntimeSetting.Query().
			Where(agentruntimesetting.IDEQ(row.ID)).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				continue
			}
			failed++
			lg.Error("roster backfill settings read failed",
				loggateway.StepID("data.seed.roster_identity"),
				loggateway.Str("agent_key", row.AgentKey),
				loggateway.Err(err),
			)
			continue
		}
		prof := strings.ToLower(strings.TrimSpace(st.ToolsProfile))
		if prof != "" && prof != "general" {
			continue
		}
		want := biz.InferToolsProfile(path)
		if want == "" || want == prof {
			continue
		}
		intended++
		if err := tx.AgentRuntimeSetting.UpdateOneID(row.ID).
			SetToolsProfile(want).
			SetUpdatedAt(now).
			Exec(ctx); err != nil {
			failed++
			lg.Error("roster backfill profile failed",
				loggateway.StepID("data.seed.roster_identity"),
				loggateway.Str("agent_key", row.AgentKey),
				loggateway.Err(err),
			)
		}
	}
	if failed > 0 {
		lg.Error("roster identity backfill had failures",
			loggateway.StepID("data.seed.roster_identity"),
			loggateway.Int("intended", intended),
			loggateway.Int("updated", updated),
			loggateway.Int("failed", failed),
		)
	}
	if intended > 0 && updated == 0 && failed == intended {
		return fmt.Errorf("roster identity backfill failed for all %d updates", failed)
	}
	if err := tx.Commit(); err != nil {
		return entErrToBizErr(err, "SEED")
	}
	committed = true
	lg.Info("roster identity backfill done",
		loggateway.StepID("data.seed.roster_identity"),
		loggateway.Int("agent_rows", len(rows)),
		loggateway.Int("updated", updated),
		loggateway.Int("failed", failed),
	)
	return nil
}
