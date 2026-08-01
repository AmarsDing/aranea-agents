package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// siRiskRuleRepo implements biz.SIRiskRuleRepo on the system_settings
// singleton via raw SQL (columns managed by DDL migration 20261121, same
// pattern as planner_model_columns — the Ent generator can't cover them).
type siRiskRuleRepo struct {
	data *Data
}

var _ biz.SIRiskRuleRepo = (*siRiskRuleRepo)(nil)

// NewSIRiskRuleRepo wires the P5 risk-rule persistence port.
func NewSIRiskRuleRepo(d *Data) biz.SIRiskRuleRepo {
	return &siRiskRuleRepo{data: d}
}

func (r *siRiskRuleRepo) GetSIRiskRules(ctx context.Context) (biz.SIRiskRules, error) {
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT si_risk_low_max_lines, si_risk_medium_max_lines, si_risk_core_path_globs, si_risk_daily_auto_quota
		 FROM system_settings WHERE id = ? LIMIT 1`), systemSettingSingletonID)
	if err != nil {
		return biz.SIRiskRules{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.SIRiskRules{}, apierror.NotFound(apierror.DomainData, "not found")
	}
	var rules biz.SIRiskRules
	var globs string
	if err := rows.Scan(&rules.LowMaxLines, &rules.MediumMaxLines, &globs, &rules.DailyAutoQuota); err != nil {
		return biz.SIRiskRules{}, err
	}
	rules.CorePathGlobs = splitSIRiskGlobs(globs)
	return rules, nil
}

func (r *siRiskRuleRepo) UpdateSIRiskRules(ctx context.Context, rules biz.SIRiskRules) (biz.SIRiskRules, error) {
	globs := strings.Join(rules.CorePathGlobs, "\n")
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE system_settings SET si_risk_low_max_lines=?, si_risk_medium_max_lines=?, si_risk_core_path_globs=?, si_risk_daily_auto_quota=? WHERE id=?`),
		rules.LowMaxLines, rules.MediumMaxLines, globs, rules.DailyAutoQuota, systemSettingSingletonID)
	if err != nil {
		return biz.SIRiskRules{}, err
	}
	return rules, nil
}

// splitSIRiskGlobs decodes the newline-joined globs column, dropping blank
// lines and surrounding whitespace.
func splitSIRiskGlobs(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if g := strings.TrimSpace(line); g != "" {
			out = append(out, g)
		}
	}
	return out
}
