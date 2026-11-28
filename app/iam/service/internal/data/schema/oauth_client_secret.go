package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthClientSecret stores a hashed client secret for zero-downtime rotation on confidential OAuth clients.
type OAuthClientSecret struct{ ent.Schema }

func (OAuthClientSecret) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").NotEmpty().Immutable(),
		field.String("name").NotEmpty().Default("默认密钥"),
		field.String("secret_prefix").NotEmpty(),
		field.String("secret_hash").NotEmpty().Sensitive(),
		field.Time("expires_time").Optional().Nillable(),
		field.Time("last_used_time").Optional().Nillable(),
		field.Time("create_time").Default(time.Now).Immutable(),
	}
}

func (OAuthClientSecret) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id", "create_time"),
		index.Fields("client_id"),
	}
}

func (OAuthClientSecret) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_client_secrets"}}
}
