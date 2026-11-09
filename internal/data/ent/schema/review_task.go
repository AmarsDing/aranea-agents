package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ReviewTask is MVP media/commerce domain table `review_tasks`.
type ReviewTask struct {
	ent.Schema
}

func (ReviewTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "review_tasks"},
	}
}

func (ReviewTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),

		field.String("content_item_id").MaxLen(256),
		field.String("graph_execution_id").Default("").MaxLen(256),
		field.String("graph_node_id").Default("").MaxLen(256),
		field.String("graph_task_id").Default("").MaxLen(256),
		field.String("draft_artifact_id").Default("").MaxLen(256),
		field.Text("platform_targets_json").Default("[]"),
		field.Text("machine_flags_json").Default("[]"),
		field.String("draft_title").Default("").MaxLen(1024),
		field.Text("draft_body").Default(""),
		field.String("status").Default("pending").MaxLen(64),
		field.Text("decision_note").Default(""),
		field.String("reviewer_id").Default("").MaxLen(256),
		// creator_id（E 阶段 W1b 收编）：内容创建者 JWT 主体 id，供「不可自批」
		// 服务端强制校验；图执行机器入队任务为空串。存量行默认 ''。
		field.String("creator_id").Default("").MaxLen(256),
		field.String("decided_at").Default(""),

		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (ReviewTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("graph_execution_id"),
	}
}
