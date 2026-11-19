package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CronTaskRun maps legacy table cron_task_run.
type CronTaskRun struct {
	ent.Schema
}

func (CronTaskRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "cron_task_run"},
	}
}

func (CronTaskRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("task_id").MaxLen(256),
		field.String("status").Default("pending"),
		field.String("started_at").Default(""),
		field.String("finished_at").Default(""),
		field.Text("output_json").Default(""),
		field.Text("error_message").Default(""),
		field.String("created_at").Default(""),
		// 失败核销（workbench 告警 lane）：resolved_at 非空 = 已忽略/核销。
		field.String("resolved_at").Default(""),
		field.String("resolved_by").Default(""),
	}
}

func (CronTaskRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "created_at").StorageKey("idx_cron_run_task"),
		index.Fields("status", "resolved_at").StorageKey("idx_cron_run_status"),
	}
}
