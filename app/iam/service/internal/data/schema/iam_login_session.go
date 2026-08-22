package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// IAMLoginSession stores one independently revocable browser login session.
type IAMLoginSession struct{ ent.Schema }

func (IAMLoginSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id").NotEmpty().Immutable(),
		field.String("secret_hash").NotEmpty().Unique().Sensitive(),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("last_seen_time").Default(time.Now),
		field.Time("idle_expires_time"),
		field.Time("absolute_expires_time").Immutable(),
		field.Time("revoked_time").Optional().Nillable(),
	}
}

func (IAMLoginSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "revoked_time"),
		index.Fields("idle_expires_time"),
		index.Fields("absolute_expires_time"),
	}
}

func (IAMLoginSession) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "iam_login_sessions"}}
}
