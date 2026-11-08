package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CeoboardDeadLetter maps ceoboard_dead_letters rows
// (整合 P2，自 ceo-dashboard SQLite dead_letters 表).
//
// kind ∈ webhook-receive|webhook-process|nats-decode；
// 同 (kind, source, ref_key) 未重放时幂等返回已有 id（biz 层语义）；
// replayed_at NULL = 未重放。
type CeoboardDeadLetter struct {
	ent.Schema
}

func (CeoboardDeadLetter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ceoboard_dead_letters"},
	}
}

func (CeoboardDeadLetter) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("kind").MaxLen(32),
		field.String("source").Default("").MaxLen(64),
		field.String("ref_key").Default("").MaxLen(512),
		field.Text("payload").Default(""),
		field.String("reason").Default("").MaxLen(1024).
			Comment("失败原因（biz 层截断 1024）"),
		field.Time("failed_at"),
		field.Time("replayed_at").Optional().Nillable(),
	}
}

func (CeoboardDeadLetter) Indexes() []ent.Index {
	return []ent.Index{
		// 幂等查找（未重放过滤在查询条件中表达）。
		index.Fields("kind", "source", "ref_key"),
		index.Fields("replayed_at"),
	}
}
