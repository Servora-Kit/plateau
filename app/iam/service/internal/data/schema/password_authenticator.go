package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PasswordAuthenticator stores Argon2id PHC material for one Authenticator.
type PasswordAuthenticator struct{ ent.Schema }

func (PasswordAuthenticator) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("authenticator_id").NotEmpty().Immutable(),
		field.String("password_hash").NotEmpty().Sensitive(),
		field.Time("changed_time").Default(time.Now),
	}
}

func (PasswordAuthenticator) Indexes() []ent.Index {
	return []ent.Index{index.Fields("authenticator_id").Unique()}
}

func (PasswordAuthenticator) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "password_authenticators"}}
}
