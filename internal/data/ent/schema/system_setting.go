package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// SystemSetting holds singleton (id=1) platform settings in SQLite.
type SystemSetting struct {
	ent.Schema
}

func (SystemSetting) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.Text("root_directory").Default(""),
		field.Text("work_directory").Default(""),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}
