package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type CompiledTeam struct {
	ent.Schema
}

func (CompiledTeam) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "compiled_teams"},
	}
}

func (CompiledTeam) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(192).Unique().Immutable(),
		field.String("team_id").MaxLen(64),
		field.String("graph_id").MaxLen(64),
		field.String("session_id").MaxLen(64).Default(""),
		field.Text("config_json"),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (CompiledTeam) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_id"),
		index.Fields("graph_id"),
		index.Fields("session_id"),
		index.Fields("team_id", "graph_id").Unique(),
	}
}
