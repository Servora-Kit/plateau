package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EmailVerificationToken stores a one-time email verification token hash.
type EmailVerificationToken struct{ ent.Schema }

func (EmailVerificationToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id").NotEmpty().Immutable(),
		field.String("login_identifier_id").NotEmpty().Immutable(),
		field.String("token_hash").NotEmpty().Unique().Sensitive(),
		field.Time("expires_time").Immutable(),
		field.Time("consumed_time").Optional().Nillable(),
		field.Time("create_time").Default(time.Now).Immutable(),
	}
}

func (EmailVerificationToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "expires_time"),
		index.Fields("expires_time", "consumed_time"),
	}
}

func (EmailVerificationToken) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "email_verification_tokens"}}
}
