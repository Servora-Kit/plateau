package schema

import (
	"entgo.io/ent"
	entmixin "github.com/Servora-Kit/plateau/infra/entgo/mixin"
)

// HTTPSession 定义官方 SCS PostgreSQL Store 使用的表，运行时由 Store 读写。
type HTTPSession struct{ ent.Schema }

func (HTTPSession) Mixin() []ent.Mixin {
	return []ent.Mixin{entmixin.HTTPSessionMixin{}}
}
