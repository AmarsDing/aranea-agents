package data

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
)

var _ biz.SITriggerCooldownStore = (*siRiskRuleRepo)(nil)

// NewSITriggerCooldownStore wires D8 trigger-cooldown persistence on the
// system_settings singleton (column si_trigger_cooldown_multipliers, DDL
// 20261217). Same unexported repo as SIRiskRuleRepo — no extra table.
func NewSITriggerCooldownStore(d *Data) biz.SITriggerCooldownStore {
	return &siRiskRuleRepo{data: d}
}

func (r *siRiskRuleRepo) LoadTriggerCooldownMultipliers(ctx context.Context) (map[string]float64, error) {
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT si_trigger_cooldown_multipliers FROM system_settings WHERE id = ? LIMIT 1`),
		systemSettingSingletonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return map[string]float64{}, nil
	}
	var raw string
	if err := rows.Scan(&raw); err != nil {
		return nil, err
	}
	return parseTriggerCooldownJSON(raw)
}

func (r *siRiskRuleRepo) SaveTriggerCooldownMultipliers(ctx context.Context, multipliers map[string]float64) error {
	if multipliers == nil {
		multipliers = map[string]float64{}
	}
	payload, err := json.Marshal(multipliers)
	if err != nil {
		return err
	}
	_, err = r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE system_settings SET si_trigger_cooldown_multipliers=? WHERE id=?`),
		string(payload), systemSettingSingletonID)
	return err
}

func parseTriggerCooldownJSON(raw string) (map[string]float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]float64{}, nil
	}
	var m map[string]float64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	if m == nil {
		return map[string]float64{}, nil
	}
	return m, nil
}
