package security

import (
	"maps"

	"google.golang.org/protobuf/proto"
)

// RulesOption configures generated RPC route rules of protobuf message type R.
type RulesOption[R proto.Message] func(*rulesOptions[R])

type rulesOptions[R proto.Message] struct {
	rulesFuncs []func() map[string]R
}

// WithRulesFuncs registers generated route rule providers (AuthN, AuthZ, or Audit).
// Go infers R directly from the passed Rules() functions.
func WithRulesFuncs[R proto.Message](providers ...func() map[string]R) RulesOption[R] {
	return func(o *rulesOptions[R]) {
		o.rulesFuncs = append(o.rulesFuncs, providers...)
	}
}

// NewRules applies opts and merges generated rule maps once; later providers
// win on duplicate operations.
func NewRules[R proto.Message](opts ...RulesOption[R]) map[string]R {
	o := &rulesOptions[R]{}
	for _, opt := range opts {
		if opt == nil {
			panic("security: rules option is nil")
		}
		opt(o)
	}

	rules := make(map[string]R)
	for _, provider := range o.rulesFuncs {
		if provider == nil {
			continue
		}
		maps.Copy(rules, provider())
	}
	return rules
}
