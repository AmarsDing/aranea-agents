package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// KnowledgeLinkUsed maps table knowledge_link_used (B4 #8): one row per
// (collection, doc) recording the last time the doc was picked as a wikilink
// completion target; drives empty-query [[ completion recency ordering.
type KnowledgeLinkUsed struct {
	ent.Schema
}

func (KnowledgeLinkUsed) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "knowledge_link_used"},
	}
}

func (KnowledgeLinkUsed) Fields() []ent.Field {
	return []ent.Field{
		field.String("collection_id").MaxLen(64).NotEmpty(),
		field.String("doc_id").MaxLen(64).NotEmpty(),
		field.Time("last_used_at"),
	}
}

func (KnowledgeLinkUsed) Indexes() []ent.Index {
	return []ent.Index{
		// upsert 冲突键 + collection 内按 recency 排序扫描。
		index.Fields("collection_id", "doc_id").Unique(),
		index.Fields("collection_id", "last_used_at"),
	}
}
