package data

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite/compat"
)

func TestMigrateProviderBindings_agentsAndLLM(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:catalog_migrate_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ddl := []string{
		`CREATE TABLE agents (id TEXT PRIMARY KEY, provider TEXT, updated_at TEXT)`,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, default_provider TEXT, last_provider TEXT)`,
		`CREATE TABLE system_settings (id INTEGER PRIMARY KEY, eval_sim_provider TEXT, eval_judge_provider TEXT, knowledge_embed_provider TEXT, web_research_provider TEXT, update_time TEXT)`,
		`CREATE TABLE agent_runtime_settings (id TEXT PRIMARY KEY, l0_compress_provider TEXT, memory_worker_provider TEXT)`,
		`CREATE TABLE skill (id TEXT PRIMARY KEY, provider TEXT, deleted_at TEXT, updated_at TEXT)`,
		`CREATE TABLE llm_provider_models (id TEXT PRIMARY KEY, provider TEXT, model TEXT, model_key TEXT, updated_at TEXT)`,
	}
	for _, q := range ddl {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO agents (id, provider, updated_at) VALUES ('a1', 'aliyun-qwen', 't')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO llm_provider_models (id, provider, model, model_key, updated_at) VALUES ('m1', 'aliyun-qwen', 'qwen-max', 'aliyun-qwen:qwen-max', 't')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO system_settings (id, eval_sim_provider, eval_judge_provider, knowledge_embed_provider, web_research_provider, update_time) VALUES (1, 'aliyun-qwen', '', '', '', 't')`); err != nil {
		t.Fatal(err)
	}

	backend := &modelCatalogApplyBackend{data: &Data{rawDB: db}}
	stats, err := backend.MigrateProviderBindings(ctx, "aliyun-qwen", "alibaba-cn")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Agents != 1 || stats.Eval != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	var provider string
	if err := db.QueryRowContext(ctx, `SELECT provider FROM agents WHERE id = 'a1'`).Scan(&provider); err != nil {
		t.Fatal(err)
	}
	if provider != "alibaba-cn" {
		t.Fatalf("agent provider not migrated: %q", provider)
	}
	if err := db.QueryRowContext(ctx, `SELECT provider FROM llm_provider_models WHERE id = 'm1'`).Scan(&provider); err != nil {
		t.Fatal(err)
	}
	if provider != "alibaba-cn" {
		t.Fatalf("llm row not migrated: %q", provider)
	}
}
