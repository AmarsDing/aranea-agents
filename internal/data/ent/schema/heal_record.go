package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// HealRecord maps table heal_records for self-heal action persistence.
type HealRecord struct {
	ent.Schema
}

func (HealRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "heal_records"},
	}
}

func (HealRecord) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("rule_id").MaxLen(128).Default(""),
		field.String("trigger_type").MaxLen(32).Default(""), // manual / auto_error_event / auto_repeated_failure
		field.String("trace_id").MaxLen(128).Default(""),
		field.String("session_id").MaxLen(128).Default(""),
		field.String("step_id").MaxLen(128).Default(""),
		field.String("fix_action_type").MaxLen(32).Default(""), // retry / reconnect / fallback / log_only
		field.Float("confidence").Default(0),
		field.String("status").MaxLen(32).Default(""), // applied / observed_healed / observed_failed / alert_fired / skipped_* / failed
		field.Bool("runtime_auto_healed").Default(false),
		field.Int("runtime_heal_attempts").Default(0),
		field.Text("reason").Default(""),
		field.Time("created_at"),
	}
}

func (HealRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("rule_id", "created_at"),
		index.Fields("session_id", "created_at"),
		index.Fields("status", "created_at"),
	}
}
