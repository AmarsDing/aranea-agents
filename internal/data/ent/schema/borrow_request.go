package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BorrowRequest holds the schema for the borrow_requests table.
// Tracks cross-department agent borrowing requests and their approval lifecycle.
type BorrowRequest struct {
	ent.Schema
}

func (BorrowRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "borrow_requests"},
	}
}

func (BorrowRequest) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("team_id").MaxLen(256).Comment("target team that wants to borrow the agent"),
		field.String("agent_id").MaxLen(256).Comment("agent being borrowed"),
		field.String("from_dept_id").MaxLen(256).Comment("department that owns the agent"),
		field.String("to_dept_id").MaxLen(256).Comment("department that wants to borrow the agent"),
		field.String("status").Default("pending").Comment("pending | approved | rejected | auto_approved"),
		field.Text("reason").Default("").Comment("reason for the borrow request"),
		field.String("reviewed_by").Default("").Comment("dept lead agent ID who reviewed"),
		field.Text("review_reason").Default("").Comment("reason for approval/rejection"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (BorrowRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("from_dept_id", "status").StorageKey("idx_borrow_from_dept_status"),
		index.Fields("team_id").StorageKey("idx_borrow_team_id"),
		index.Fields("status", "created_at").StorageKey("idx_borrow_status_created"),
	}
}
