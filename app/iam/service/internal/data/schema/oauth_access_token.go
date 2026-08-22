package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthAccessToken stores JWT access-token metadata for revocation and UserInfo.
type OAuthAccessToken struct{ ent.Schema }

func (OAuthAccessToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("token_session_id").NotEmpty().Immutable(),
		field.String("subject").NotEmpty().Immutable(),
		field.String("client_id").NotEmpty().Immutable(),
		field.JSON("scopes", []string{}),
		field.Time("issued_time").Default(time.Now).Immutable(),
		field.Time("expires_time").Immutable(),
		field.Time("revoked_time").Optional().Nillable(),
	}
}

func (OAuthAccessToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token_session_id", "revoked_time"),
		index.Fields("subject", "expires_time"),
		index.Fields("expires_time"),
	}
}

func (OAuthAccessToken) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_access_tokens"}}
}
