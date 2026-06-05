package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

// FailurePattern maps table failure_pattern for unified failure pattern knowledge base.
type FailurePattern struct {
	ent.Schema
}

func (FailurePattern) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "failure_pattern"},
	}
}

func (FailurePattern) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable().DefaultFunc(uuid.NewString),
		field.String("source").MaxLen(32).NotEmpty(),       // runtime / ci / mined
		field.String("type").MaxLen(64).NotEmpty(),         // failure type classification
		field.String("pattern_hash").MaxLen(64).NotEmpty(), // SHA256 hash of pattern
		field.Text("pattern_regex").NotEmpty(),             // regex pattern for matching
		field.Text("fix_action").NotEmpty(),                // JSON-encoded FixAction
		field.Float("confidence").Default(0.5),             // 0-1 confidence score
		field.Int("success_count").Default(0),              // number of successful applications
		field.Int("fail_count").Default(0),                 // number of failed applications
		field.Int("version").Default(1),                    // version number for rollback
		field.Bool("is_active").Default(true),              // active flag
		field.Time("created_at").Immutable(),               // creation time
		field.Time("updated_at"),                           // auto-update via SQL
	}
}

func (FailurePattern) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "type"),
		index.Fields("pattern_hash"),
		index.Fields("is_active", "confidence"),
	}
}
