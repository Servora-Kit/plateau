package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OIDCAuthorizationRequest stores recoverable authorization interaction state.
type OIDCAuthorizationRequest struct{ ent.Schema }

func (OIDCAuthorizationRequest) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").NotEmpty().Immutable(),
		field.String("redirect_uri").NotEmpty().Immutable(),
		field.String("response_type").NotEmpty().Immutable(),
		field.String("response_mode").Optional().Default(""),
		field.JSON("scopes", []string{}),
		field.String("state").Optional().Nillable().Sensitive(),
		field.String("nonce").Optional().Nillable().Sensitive(),
		field.String("pkce_challenge").NotEmpty().Sensitive(),
		field.String("pkce_challenge_method").Default("S256"),
		field.String("subject").Optional().Nillable(),
		field.String("iam_login_session_id").Optional().Nillable(),
		field.Time("auth_time").Optional().Nillable(),
		field.Bool("done").Default(false),
		field.Time("expires_time").Immutable(),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (OIDCAuthorizationRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id", "expires_time"),
		index.Fields("subject", "expires_time"),
		index.Fields("expires_time"),
	}
}

func (OIDCAuthorizationRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oidc_authorization_requests"}}
}
