package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformChannelCredential maps table `channel_credential`.
type PlatformChannelCredential struct {
	ent.Schema
}

func (PlatformChannelCredential) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_credential"},
	}
}

func (PlatformChannelCredential) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("channel_id").MaxLen(256).NotEmpty(),
		field.String("credential_key").MaxLen(512).NotEmpty(),
		field.String("status").Default("active"),
		field.String("secret_ref").Default(""),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}

func (PlatformChannelCredential) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel_id", "credential_key").Unique(),
	}
}
