package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// OAuthClient stores one statically seeded confidential client.
type OAuthClient struct{ ent.Schema }

func (OAuthClient) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("secret_hash").NotEmpty().Sensitive(),
		field.JSON("redirect_uris", []string{}),
		field.JSON("allowed_grant_types", []string{}),
		field.JSON("allowed_response_types", []string{}),
		field.JSON("allowed_scopes", []string{}),
		field.Bool("trusted").Default(false),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (OAuthClient) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_clients"}}
}
