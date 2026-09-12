package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// HTTPSession 定义官方 SCS PostgreSQL Store 使用的表，运行时由 Store 读写。
type HTTPSession struct{ ent.Schema }

func (HTTPSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("token").SchemaType(map[string]string{dialect.Postgres: "text"}).Sensitive(),
		field.Bytes("data").Sensitive(),
		field.Time("expiry").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (HTTPSession) Indexes() []ent.Index { return []ent.Index{index.Fields("expiry")} }
func (HTTPSession) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sessions"}}
}
