package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BackgroundJob persists async work units for the Unified BackgroundJob subsystem (M56 BLO-5).
// All columns use string/int/bytes primitives for SQLite compatibility.
type BackgroundJob struct {
	ent.Schema
}

func (BackgroundJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "background_jobs"},
	}
}

func (BackgroundJob) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(128),
		field.String("kind").Default(""),          // registered runner kind
		field.String("owner_type").Default(""),    // session / channel / system
		field.String("owner_id").Default(""),      // owning entity ID
		field.String("parent_job_id").Default(""), // parent job for DAG dependencies
		field.Int("priority").Default(50),         // lower = higher urgency
		field.String("status").Default("queued"),  // queued / claimed / succeeded / failed / cancelled
		field.Bytes("payload").Default([]byte("{}")),
		field.String("worker_id").Default(""),
		field.Int("attempts").Default(0),
		field.Int("max_attempts").Default(3),
		field.Text("last_error").Default(""),
		field.Int64("scheduled_at").Default(0),  // Unix milliseconds; 0 = immediately claimable
		field.Int64("claimed_at").Default(0),
		field.Int64("finished_at").Default(0),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (BackgroundJob) Indexes() []ent.Index {
	return []ent.Index{
		// Primary claim query: find claimable jobs ordered by priority.
		index.Fields("status", "priority", "scheduled_at"),
		// Jobs by owner (for listing and bulk-cancel).
		index.Fields("owner_type", "owner_id", "status"),
		// DAG parent lookup: find children of a given parent.
		index.Fields("parent_job_id", "status"),
		// Cleanup query: find terminal jobs older than N.
		index.Fields("status", "finished_at"),
	}
}
