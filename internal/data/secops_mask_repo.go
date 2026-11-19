package data

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
)

// ============================================================================
// secops 脱敏规则只读台账（E 阶段 W2b D6 L-2，DDL 迁移 20261327 +
// 种子 20261328）。查询走 RWDB().ReadDB 返回的 execer（事务感知），
// wordlist_json 列反序列化为 []string（wb 的 TEXT[] 列在本仓收敛为
// JSON 文本，双方言可移植）。布尔列统一 INTEGER 0/1（迁移同口径）。
// ============================================================================

type secopsMaskRepo struct {
	data *Data
}

var _ biz.SecopsMaskRuleRepo = (*secopsMaskRepo)(nil)

// NewSecopsMaskRuleRepo 装配脱敏规则只读台账（nil Data → nil）。
func NewSecopsMaskRuleRepo(d *Data) biz.SecopsMaskRuleRepo {
	if d == nil {
		return nil
	}
	return &secopsMaskRepo{data: d}
}

// ListEnabledMaskRules 返回启用规则。id ASC 排序保证规则间先后稳定
// （身份证先于银行卡的种子设计依赖该顺序，见迁移 20261328）。
func (r *secopsMaskRepo) ListEnabledMaskRules(ctx context.Context) ([]biz.MaskRule, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, name, entity, pattern, wordlist_json, operator, enabled, shadow, rollout_pct, version
		   FROM mask_rules WHERE enabled = 1 ORDER BY id ASC`)
	if err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	out := make([]biz.MaskRule, 0)
	for rows.Next() {
		var rule biz.MaskRule
		var wordlistJSON string
		var enabled, shadow int
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Entity, &rule.Pattern, &wordlistJSON,
			&rule.Operator, &enabled, &shadow, &rule.RolloutPct, &rule.Version); err != nil {
			return nil, entErrToBizErr(err, "SECOPS")
		}
		rule.Enabled = enabled != 0
		rule.Shadow = shadow != 0
		if wordlistJSON != "" {
			var words []string
			if err := json.Unmarshal([]byte(wordlistJSON), &words); err == nil {
				rule.Wordlist = words
			}
			// 脏 JSON：词表作废但规则本身（正则/其它行）不因此失效——
			// 引擎层对无正则无词表的规则自动跳过，保持最大化可用性。
		}
		out = append(out, rule)
	}
	return out, entErrToBizErr(rows.Err(), "SECOPS")
}
