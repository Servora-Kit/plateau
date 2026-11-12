// Package mixin provides shared Ent schema mixins for Plateau services.
package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// HTTPSessionMixin defines the sessions table, columns, and expiry index
// required by the SCS PostgreSQL store.
type HTTPSessionMixin struct {
	mixin.Schema
}

func (HTTPSessionMixin) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("token").SchemaType(map[string]string{dialect.Postgres: "text"}).Sensitive(),
		field.Bytes("data").Sensitive(),
		field.Time("expiry").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (HTTPSessionMixin) Indexes() []ent.Index {
	return []ent.Index{index.Fields("expiry")}
}

func (HTTPSessionMixin) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sessions"}}
}
