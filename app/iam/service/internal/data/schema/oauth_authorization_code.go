package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthAuthorizationCode stores a one-time authorization code hash and binding metadata.
type OAuthAuthorizationCode struct{ ent.Schema }

func (OAuthAuthorizationCode) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("code_hash").NotEmpty().Unique().Sensitive(),
		field.String("authorization_request_id").NotEmpty().Immutable(),
		field.String("token_session_id").Optional().Nillable(),
		field.String("client_id").NotEmpty().Immutable(),
		field.String("subject").NotEmpty().Immutable(),
		field.String("redirect_uri").NotEmpty().Immutable(),
		field.JSON("scopes", []string{}),
		field.String("pkce_challenge").NotEmpty().Sensitive(),
		field.String("pkce_challenge_method").Default("S256"),
		field.Time("expires_time").Immutable(),
		field.Time("consumed_time").Optional().Nillable(),
		field.Time("create_time").Default(time.Now).Immutable(),
	}
}

func (OAuthAuthorizationCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("authorization_request_id").Unique(),
		index.Fields("client_id", "expires_time"),
		index.Fields("expires_time", "consumed_time"),
	}
}

func (OAuthAuthorizationCode) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_authorization_codes"}}
}
