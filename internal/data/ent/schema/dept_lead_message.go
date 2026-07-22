package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DeptLeadMessage holds the schema for the dept_lead_messages table (M71).
// Async mailbox between department lead agents: subject/body + optional
// DeliverableRef references, 3-state status (unread/read/replied), threaded
// via reply_to_id.
type DeptLeadMessage struct {
	ent.Schema
}

func (DeptLeadMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "dept_lead_messages"},
	}
}

func (DeptLeadMessage) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64),
		field.String("from_agent_id").MaxLen(256).Comment("sender dept lead agent ID"),
		field.String("from_dept_id").MaxLen(256).Comment("sender department ID"),
		field.String("to_agent_id").MaxLen(256).Comment("recipient dept lead agent ID (resolved at send time)"),
		field.String("to_dept_id").MaxLen(256).Comment("recipient department ID"),
		field.String("subject").MaxLen(200).Default(""),
		field.Text("body").Default(""),
		field.Text("refs_json").Default("[]").Comment("DeliverableRef array JSON"),
		field.String("status").MaxLen(16).Default("unread").Comment("unread | read | replied"),
		field.String("reply_to_id").MaxLen(64).Default("").Comment("thread: id of the message being replied to"),
		field.Time("created_at").Default(timeNow),
		field.Time("read_at").Optional().Nillable(),
	}
}

func (DeptLeadMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("to_agent_id", "status").StorageKey("idx_deptleadmsg_to_status"),
		index.Fields("from_agent_id", "created_at").StorageKey("idx_deptleadmsg_from_created"),
	}
}
