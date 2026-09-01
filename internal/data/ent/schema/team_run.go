package schema

import (
	"aranea-agents/internal/biz"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TeamRun maps legacy table team_runs.
type TeamRun struct {
	ent.Schema
}

func (TeamRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_runs"},
	}
}

func (TeamRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("team_id").MaxLen(256),
		field.String("session_id").Default(""),
		field.String("message_id").Default(""),
		field.String("mode").Default(""),
		field.String("status").Default(biz.TeamRunStatusRunning),
		field.Text("input_preview").Default(""),
		field.Text("output_preview").Default(""),
		field.Int("token_in").Default(0),
		field.Int("token_out").Default(0),
		field.Int64("cost_micro_usd").Default(0),
		field.Int("duration_ms").Default(0),
		field.Text("error_message").Default(""),
		field.Text("topology_json").Default("{}"),
		field.String("started_at").Default(""),
		field.String("finished_at").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("graph_execution_id").Default(""),
		field.Text("definition_snapshot_json").Default(""),
		field.String("trace_id").Default("").MaxLen(128),
		// team_run_v2_id: 桥接到 team_runs_v2.id（确定性 SHA1 派生，
		// agent.NewTeamRunV2ID）。v1 run.ID 是随机 UUID，与 v2 id 独立生成，
		// 导致 team_run_steps.run_id 无法 join team_runs_v2（取证断裂）。
		// 空 = 无 root task 上下文无法派生（legacy/测试路径）。
		field.String("team_run_v2_id").Default("").MaxLen(64),
	}
}

func (TeamRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_id", "created_at").StorageKey("idx_team_runs_team_created"),
		index.Fields("session_id", "created_at").StorageKey("idx_team_runs_session"),
		index.Fields("trace_id", "created_at").StorageKey("idx_team_runs_trace"),
	}
}
