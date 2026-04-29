package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// UserEmbeddingSetting stores per-user embedding model preferences from the UI (SQLite).
// PostgreSQL vector tables are keyed by vector_dimension (see pgvector package).
type UserEmbeddingSetting struct {
	ent.Schema
}

func (UserEmbeddingSetting) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique(),
		field.String("user_id").MaxLen(256).Unique(),
		field.Int("vector_dimension").Positive(),
		field.String("provider").Default(""),
		field.String("model").Default(""),
		field.Text("options_json").Default("{}"),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}
