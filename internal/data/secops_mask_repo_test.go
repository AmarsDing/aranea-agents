package data

// ensureMaskRuleTable + D6 L-2 脱敏规则只读台账测试（镜像迁移 20261327 DDL
// 的 mask_rules 子集；测试自包含，与 ensureOffboardTable 同策略）。db 必须
// 与测试 Data 同一 schema——复用 setupEnvelopeTestRepo 返回的句柄。

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
)

func ensureMaskRuleTable(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	stmt := `CREATE TABLE IF NOT EXISTS mask_rules (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL DEFAULT '',
		entity       TEXT NOT NULL,
		pattern      TEXT NOT NULL DEFAULT '',
		wordlist_json TEXT NOT NULL DEFAULT '[]',
		operator     TEXT NOT NULL DEFAULT 'placeholder',
		enabled      INTEGER NOT NULL DEFAULT 1,
		shadow       INTEGER NOT NULL DEFAULT 0,
		rollout_pct  INTEGER NOT NULL DEFAULT 100,
		version      INTEGER NOT NULL DEFAULT 1,
		created_at   TEXT NOT NULL DEFAULT ''
	)`
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("create mask_rules: %v", err)
	}
}

func newMaskRuleTestRepo(t *testing.T) (*secopsMaskRepo, context.Context, *sql.DB) {
	t.Helper()
	d, _, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()
	ensureMaskRuleTable(t, ctx, db)
	return &secopsMaskRepo{data: d}, ctx, db
}

func insertMaskRuleRow(t *testing.T, ctx context.Context, db *sql.DB, id, entity, wordlistJSON string, enabled, shadow, rollout int) {
	t.Helper()
	_, err := db.ExecContext(ctx,
		`INSERT INTO mask_rules (id, name, entity, pattern, wordlist_json, operator, enabled, shadow, rollout_pct, version)
		 VALUES ($1, $1, $2, $3, $4, 'placeholder', $5, $6, $7, 1)
		 ON CONFLICT (id) DO NOTHING`,
		id, entity, `\bX\b`, wordlistJSON, enabled, shadow, rollout)
	if err != nil {
		t.Fatalf("insert mask rule %s: %v", id, err)
	}
}

func TestSecopsMaskRepo_ListEnabledMaskRules(t *testing.T) {
	repo, ctx, db := newMaskRuleTestRepo(t)

	insertMaskRuleRow(t, ctx, db, "mr_02_phone", "手机号", "[]", 1, 0, 100)
	insertMaskRuleRow(t, ctx, db, "mr_01_idcard", "身份证号", `["甲","乙"]`, 1, 1, 50)
	insertMaskRuleRow(t, ctx, db, "mr_disabled", "OFF", "[]", 0, 0, 100)

	rules, err := repo.ListEnabledMaskRules(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("enabled 过滤后应为 2 条，got %d", len(rules))
	}
	// id ASC 排序（身份证先于银行卡的种子设计依赖该顺序）。
	if rules[0].ID != "mr_01_idcard" || rules[1].ID != "mr_02_phone" {
		t.Fatalf("order = %s,%s, want id ASC", rules[0].ID, rules[1].ID)
	}
	if rules[0].Entity != "身份证号" || !rules[0].Shadow || rules[0].RolloutPct != 50 {
		t.Fatalf("row0 = %+v", rules[0])
	}
	if len(rules[0].Wordlist) != 2 || rules[0].Wordlist[0] != "甲" {
		t.Fatalf("wordlist_json 解析失败: %+v", rules[0].Wordlist)
	}
	if !rules[0].Enabled || !rules[1].Enabled {
		t.Fatal("enabled INTEGER 0/1 → bool 映射失败")
	}
}

func TestSecopsMaskRepo_GarbageWordlistKeptAsEmpty(t *testing.T) {
	repo, ctx, db := newMaskRuleTestRepo(t)
	insertMaskRuleRow(t, ctx, db, "mr_bad_json", "X", "{not-json", 1, 0, 100)

	rules, err := repo.ListEnabledMaskRules(ctx)
	if err != nil || len(rules) != 1 {
		t.Fatalf("脏 wordlist 不得拖垮整条规则的读出，rules=%v err=%v", rules, err)
	}
	if len(rules[0].Wordlist) != 0 {
		t.Fatalf("脏 JSON 词表应作废为空，got %+v", rules[0].Wordlist)
	}
}

func TestSecopsMaskRepo_RepoSatisfiesBizInterface(t *testing.T) {
	repo, _, _ := newMaskRuleTestRepo(t)
	var _ biz.SecopsMaskRuleRepo = repo
}
