package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LoginIdentifier maps a canonical login value to one IAM User.
type LoginIdentifier struct{ ent.Schema }

func (LoginIdentifier) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id").NotEmpty().Immutable(),
		field.String("type").NotEmpty().Immutable(),
		field.String("canonical_value").NotEmpty(),
		field.String("display_value").NotEmpty(),
		field.Time("verified_time").Optional().Nillable(),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (LoginIdentifier) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "type").Unique(),
		index.Fields("type", "canonical_value").Unique(),
		index.Fields("user_id"),
	}
}

func (LoginIdentifier) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "login_identifiers"}}
}
