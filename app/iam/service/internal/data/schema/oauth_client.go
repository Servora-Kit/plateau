package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthClient stores one static bootstrap or dynamically registered OAuth 2.0 / OIDC client.
type OAuthClient struct{ ent.Schema }

func (OAuthClient) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("source").Default("dynamic").Immutable(),
		field.String("status").Default("active"),
		field.String("display_name").NotEmpty(),
		field.String("description").Optional().Nillable(),
		field.String("logo_uri").Optional().Nillable(),
		field.String("application_type").Default("web").Immutable(),
		field.String("token_endpoint_auth_method").Default("client_secret_basic"),
		field.JSON("redirect_uris", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.JSON("post_logout_redirect_uris", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.JSON("allowed_grant_types", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.JSON("allowed_response_types", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.JSON("allowed_scopes", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.JSON("audiences", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.Bool("trusted").Default(false),
		field.String("etag").NotEmpty(),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (OAuthClient) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source"),
		index.Fields("status"),
		index.Fields("create_time"),
	}
}

func (OAuthClient) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_clients"}}
}
