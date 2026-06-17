package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// GatewayWebhook maps table gateway_webhooks (outbound run lifecycle callbacks).
type GatewayWebhook struct {
	ent.Schema
}

func (GatewayWebhook) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "gateway_webhooks"},
	}
}

func (GatewayWebhook) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64),
		field.String("name").MaxLen(256),
		field.String("url").MaxLen(2048),
		field.Text("event_types_json").Default("[]"),
		field.String("secret").Default("").MaxLen(512).Sensitive(),
		field.Text("headers_json").Default("{}"),
		field.Bool("enabled").Default(true),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
