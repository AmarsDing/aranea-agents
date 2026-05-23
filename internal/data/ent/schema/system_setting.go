package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// SystemSetting holds singleton (id=1) platform settings in SQLite.
type SystemSetting struct {
	ent.Schema
}

func (SystemSetting) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.Text("root_directory").Default(""),
		field.Text("work_directory").Default(""),
		field.Int64("global_monthly_micro_usd").Default(0),
		// Public A2A endpoint URL prefix (no trailing slash). Empty = fall back to env/config/derived.
		field.Text("a2a_public_base_url").Default(""),
		// AES-256-GCM key for provider/channel credentials (hex-encoded 32 bytes). Auto-generated when empty.
		field.Text("credential_encryption_key").Default("").Sensitive(),
		// Knowledge RAG embedder defaults (env KRATOS_KNOWLEDGE_EMBED_* overrides at startup).
		field.Text("knowledge_embed_provider").Default(""),
		field.Text("knowledge_embed_base_url").Default(""),
		field.Text("knowledge_embed_api_key").Default("").Sensitive(),
		field.Text("knowledge_embed_model").Default(""),
		field.Int("knowledge_embed_dim").Default(0),
		field.Bool("mcp_allow_adhoc_http").Default(false),
		// Evaluation UserSim / LLM-as-Judge model defaults (env KRATOS_EVAL_* overrides at runtime).
		field.Text("eval_sim_provider").Default(""),
		field.Text("eval_sim_model").Default(""),
		field.Text("eval_judge_provider").Default(""),
		field.Text("eval_judge_model").Default(""),
		// Web research (Tavily / SerpAPI) for web_research tool.
		field.Text("web_research_provider").Default("tavily"),
		field.Text("web_research_api_key").Default("").Sensitive(),
		field.Int("web_research_max_results").Default(8),
		field.Int("web_research_fetch_top").Default(5),
		field.Text("web_research_search_depth").Default("basic"),
		field.Int("web_research_timeout_sec").Default(15),
		field.Text("web_research_http_proxy").Default(""),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}
