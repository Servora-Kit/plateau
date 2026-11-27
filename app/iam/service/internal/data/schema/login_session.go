package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LoginSession stores one independently revocable browser login session.
type LoginSession struct{ ent.Schema }

func (LoginSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id").NotEmpty().Immutable(),
		field.JSON("amr", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("revoked_time").Optional().Nillable(),
	}
}

func (LoginSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "revoked_time"),
	}
}

func (LoginSession) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "login_sessions"}}
}
