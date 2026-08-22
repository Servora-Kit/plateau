package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PasswordResetToken stores a one-time password-reset token hash.
type PasswordResetToken struct{ ent.Schema }

func (PasswordResetToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id").NotEmpty().Immutable(),
		field.String("token_hash").NotEmpty().Unique().Sensitive(),
		field.Time("expires_time").Immutable(),
		field.Time("consumed_time").Optional().Nillable(),
		field.Time("create_time").Default(time.Now).Immutable(),
	}
}

func (PasswordResetToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "expires_time"),
		index.Fields("expires_time", "consumed_time"),
	}
}

func (PasswordResetToken) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "password_reset_tokens"}}
}
