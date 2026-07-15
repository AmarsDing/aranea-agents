package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SessionTurn struct {
	ent.Schema
}

func (SessionTurn) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "session_turns"},
	}
}

func (SessionTurn) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("session_id").MaxLen(256),
		field.String("run_id").Default(""),
		field.Int("turn_number").Default(0),
		field.String("user_message_id").Default(""),
		field.String("assistant_message_id").Default(""),
		field.String("owner_type").Default("agent"),
		field.String("agent_id").Default(""),
		field.String("team_id").Default(""),
		field.String("status").Default("running"),
		field.String("started_at").Default(""),
		field.String("ended_at").Default(""),
		field.Int("duration_ms").Default(0),
		field.Int("first_token_ms").Default(0),
		field.Int("model_call_count").Default(0),
		field.Int("tool_call_count").Default(0),
		field.Int("skill_call_count").Default(0),
		field.Int("mcp_call_count").Default(0),
		field.Int("input_tokens").Default(0),
		field.Int("output_tokens").Default(0),
		field.Int("total_tokens").Default(0),
		field.Int64("total_cost_micro_usd").Default(0),
		field.String("final_provider").Default(""),
		field.String("final_model").Default(""),
		field.Text("final_content_preview").Default(""),
		field.String("error_code").Default(""),
		field.Text("error_message").Default(""),
		field.Text("metadata_json").Default("{}"),
		// C-13: client/source-scoped idempotency key. Empty client keys are
		// stored as "__id__:<turn_id>" so the unique index never collides.
		field.String("idempotency_key").Default("").MaxLen(512),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (SessionTurn) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "turn_number").Unique().StorageKey("idx_session_turns_session_turn_unique"),
		index.Fields("session_id", "idempotency_key").Unique().StorageKey("idx_session_turns_session_idem"),
		index.Fields("status", "started_at"),
		index.Fields("run_id").StorageKey("idx_session_turns_run_id"),
		index.Fields("agent_id").StorageKey("idx_session_turns_agent_id"),
		index.Fields("team_id").StorageKey("idx_session_turns_team_id"),
	}
}
