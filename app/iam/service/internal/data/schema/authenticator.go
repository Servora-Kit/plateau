package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Authenticator stores common lifecycle metadata for a registered authenticator.
type Authenticator struct{ ent.Schema }

func (Authenticator) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id").NotEmpty().Immutable(),
		field.String("type").NotEmpty().Immutable(),
		field.String("state").Default("active"),
		field.String("display_name").Optional().Nillable(),
		field.Time("verified_time").Optional().Nillable(),
		field.Time("last_used_time").Optional().Nillable(),
		field.Time("revoked_time").Optional().Nillable(),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Authenticator) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "type", "state"),
		index.Fields("user_id"),
		index.Fields("revoked_time"),
	}
}

func (Authenticator) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "authenticators"}}
}
