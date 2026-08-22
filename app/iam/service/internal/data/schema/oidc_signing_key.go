package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OIDCSigningKey stores public signing-key metadata; private keys remain mounted files.
type OIDCSigningKey struct{ ent.Schema }

func (OIDCSigningKey) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("public_jwk").NotEmpty(),
		field.String("algorithm").Default("RS256").Immutable(),
		field.Time("not_before_time").Immutable(),
		field.Time("expires_time").Immutable(),
		field.Time("revoked_time").Optional().Nillable(),
		field.Time("create_time").Default(time.Now).Immutable(),
	}
}

func (OIDCSigningKey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("algorithm", "not_before_time", "expires_time"),
		index.Fields("expires_time", "revoked_time"),
	}
}

func (OIDCSigningKey) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oidc_signing_keys"}}
}
