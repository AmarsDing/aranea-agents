package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Team struct {
	ent.Schema
}

func (Team) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "teams"},
	}
}

func (Team) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("team_key").Unique().MaxLen(512),
		field.String("display_name").MaxLen(1024),
		field.String("status").Default("active"),
		field.Bool("is_default").Default(false),
		field.Text("definition_json").Default(""),
		field.String("adk_app_name").Default(""),
		field.String("category_industry_id").Default(""),
		field.String("spirit_session_id").Default("").MaxLen(256),
		field.Text("task_description").Default(""),
		field.Bool("auto_created").Default(false),
		field.String("dag_node_id").Default("").MaxLen(256),
		field.Text("depends_on_json").Default(""),
		field.Text("parallel_config_json").Default(""),
		field.String("topology").Default("").MaxLen(64),
		field.Bool("readonly").Default(false).Comment("system teams cannot be deleted"),
		field.Enum("source").Values("user", "system", "imported").Default("user").Comment("team source: user | system | imported"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}

func (Team) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("spirit_session_id", "deleted_at").StorageKey("idx_teams_spirit_session"),
	}
}
