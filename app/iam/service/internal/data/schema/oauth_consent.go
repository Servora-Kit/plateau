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

// OAuthConsent remembers scopes approved by one user for a non-trusted OAuth client.
type OAuthConsent struct{ ent.Schema }

func (OAuthConsent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id").NotEmpty().Immutable(),
		field.String("client_id").NotEmpty().Immutable(),
		field.JSON("granted_scopes", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (OAuthConsent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "client_id").Unique(),
		index.Fields("client_id"),
	}
}

func (OAuthConsent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_consents"}}
}
