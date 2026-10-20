package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PublishJob is MVP media/commerce domain table `publish_jobs`.
type PublishJob struct {
	ent.Schema
}

func (PublishJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "publish_jobs"},
	}
}

func (PublishJob) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),

		field.String("review_task_id").MaxLen(256),
		field.String("content_item_id").Default("").MaxLen(256),
		field.String("platform").Default("generic").MaxLen(128),
		field.String("mode").Default("package").MaxLen(32),
		field.String("status").Default("pending").MaxLen(64),
		field.Int("attempt").Default(0),
		field.Text("last_error").Default(""),
		field.String("artifact_id").Default("").MaxLen(256),
		field.Text("package_dir").Default(""),
		field.String("idempotency_key").Default("").MaxLen(512),
		field.String("title").Default("").MaxLen(1024),
		field.Text("body").Default(""),
		field.Text("tags_json").Default("[]"),
		field.Text("media_paths_json").Default("[]"),

		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (PublishJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
	}
}
