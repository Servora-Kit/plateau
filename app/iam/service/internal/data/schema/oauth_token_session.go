package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthTokenSession owns one OAuth client refresh-token family.
type OAuthTokenSession struct{ ent.Schema }

func (OAuthTokenSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id").NotEmpty().Immutable(),
		field.String("client_id").NotEmpty().Immutable(),
		field.String("iam_login_session_id").NotEmpty().Immutable(),
		field.String("refresh_family_id").NotEmpty().Immutable(),
		field.JSON("scopes", []string{}),
		field.Time("auth_time").Immutable(),
		field.JSON("amr", []string{}),
		field.Time("revoked_time").Optional().Nillable(),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (OAuthTokenSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("iam_login_session_id", "revoked_time"),
		index.Fields("user_id", "revoked_time"),
		index.Fields("client_id", "user_id"),
		index.Fields("refresh_family_id").Unique(),
	}
}

func (OAuthTokenSession) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_token_sessions"}}
}
