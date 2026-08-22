package authz

import authzpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authz/v1"

// Option configures generated AuthZ route rules.
type Option func(*options)

type options struct {
	rulesFuncs []func() map[string]*authzpb.AuthzRule
}

// WithRulesFuncs registers generated AuthZ rule providers.
func WithRulesFuncs(providers ...func() map[string]*authzpb.AuthzRule) Option {
	return func(options *options) {
		options.rulesFuncs = append(options.rulesFuncs, providers...)
	}
}

// NewRules applies options and merges generated rule maps once; later providers win on duplicate operations.
func NewRules(opts ...Option) map[string]*authzpb.AuthzRule {
	options := &options{}
	for _, opt := range opts {
		if opt == nil {
			panic("authz: option is nil")
		}
		opt(options)
	}

	rules := make(map[string]*authzpb.AuthzRule)
	for _, provider := range options.rulesFuncs {
		if provider == nil {
			continue
		}
		for operation, rule := range provider() {
			rules[operation] = rule
		}
	}
	return rules
}
