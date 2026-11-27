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

// AuthFlow stores browser-bound multi-step authentication progress without ever holding password material.
type AuthFlow struct{ ent.Schema }

func (AuthFlow) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("owner_hash").NotEmpty().Immutable().Sensitive(),
		field.Enum("status").Values("active", "completed", "cancelled", "reset", "invalidated").Default("active"),
		field.Enum("state").Values("authenticate", "required_action", "completed").Default("authenticate"),
		field.String("oidc_request_id").Optional().Nillable().Immutable(),
		field.String("user_id").Optional().Nillable(),
		field.String("user_email").Optional().Nillable(),
		field.String("user_etag").Optional().Nillable(),
		field.String("authenticator_id").Optional().Nillable(),
		field.String("password_authenticator_id").Optional().Nillable(),
		field.Time("password_changed_time").Optional().Nillable(),
		field.JSON("completed_amr", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.JSON("required_actions", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Annotations(entsql.Default("[]")),
		field.Time("expires_time").Immutable(),
		field.Time("step_expires_time").Optional().Nillable(),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (AuthFlow) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("oidc_request_id"),
		index.Fields("expires_time"),
	}
}

func (AuthFlow) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "auth_flows"}}
}
