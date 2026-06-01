package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CronTask maps legacy table cron_task.
type CronTask struct {
	ent.Schema
}

func (CronTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "cron_task"},
	}
}

func (CronTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("task_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.String("agent_id").Default("").MaxLen(256),
		field.Text("config_json").Default(""),
		field.Text("metadata_json").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}

func (CronTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_id", "deleted_at").StorageKey("idx_cron_task_agent"),
	}
}
