package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/Servora-Kit/plateau/infra/entgo/mixin"
)

// User stores the global IAM identity and OIDC standard profile.
type User struct{ ent.Schema }

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("status").Default("pending_verification"),
		field.String("name").Optional().Nillable(),
		field.String("given_name").Optional().Nillable(),
		field.String("family_name").Optional().Nillable(),
		field.String("nickname").Optional().Nillable(),
		field.String("preferred_username").Optional().Nillable(),
		field.String("picture").Optional().Nillable(),
		field.String("locale").Optional().Nillable(),
		field.String("etag").NotEmpty(),
		field.JSON("required_actions", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.SoftDeleteMixin{},
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("create_time"),
	}
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "users"}}
}
