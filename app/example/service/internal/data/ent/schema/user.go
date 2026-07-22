package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	entgomixin "github.com/Servora-Kit/servora/contrib/db/entgo/mixin"
)


// User stores the private persistence shape for example.servora.dev/User.
type User struct {
	ent.Schema
}

// Fields declares application-owned fields. Tombstone fields come from SoftDeleteMixin.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("tenant_id").NotEmpty().Immutable(),
		field.String("resource_id").NotEmpty().Immutable(),
		field.String("display_name").Default(""),
		field.String("email").NotEmpty(),
		field.String("tenant_plan").Default("").Immutable(),
		field.String("nickname").Optional().Nillable(),
		field.String("password_hash").Optional().Nillable().Sensitive(),
		field.String("etag").NotEmpty(),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes keep canonical identity unique for active and tombstoned rows.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "resource_id").Unique(),
	}
}

// Mixin installs tombstone storage, default filtering, and delete rewriting.
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		entgomixin.SoftDeleteMixin{},
	}
}
