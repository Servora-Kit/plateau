package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthRefreshToken stores one rotating refresh-token hash.
type OAuthRefreshToken struct{ ent.Schema }

func (OAuthRefreshToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("token_session_id").NotEmpty().Immutable(),
		field.String("family_id").NotEmpty().Immutable(),
		field.String("token_hash").NotEmpty().Unique().Sensitive(),
		field.String("parent_token_id").Optional().Nillable().Immutable(),
		field.Time("issued_time").Default(time.Now).Immutable(),
		field.Time("expires_time").Immutable(),
		field.Time("consumed_time").Optional().Nillable(),
		field.Time("revoked_time").Optional().Nillable(),
	}
}

func (OAuthRefreshToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token_session_id", "revoked_time"),
		index.Fields("family_id", "issued_time"),
		index.Fields("expires_time", "consumed_time"),
	}
}

func (OAuthRefreshToken) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_refresh_tokens"}}
}
