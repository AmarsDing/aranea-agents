package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// SchemaMigration records one-time data migration versions (version is the primary key).
type SchemaMigration struct {
	ent.Schema
}

func (SchemaMigration) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "schema_migrations"},
	}
}

func (SchemaMigration) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").StorageKey("version").Unique().Immutable(),
		field.Text("name"),
		field.Text("applied_at"),
	}
}
