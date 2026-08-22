package authn

import authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/v1"

// Option configures generated AuthN route rules.
type Option func(*options)

type options struct {
	rulesFuncs []func() map[string]*authnpb.AuthnRule
}

// WithRulesFuncs registers generated AuthN rule providers.
func WithRulesFuncs(providers ...func() map[string]*authnpb.AuthnRule) Option {
	return func(options *options) {
		options.rulesFuncs = append(options.rulesFuncs, providers...)
	}
}

// NewRules applies options and merges generated rule maps once; later providers win on duplicate operations.
func NewRules(opts ...Option) map[string]*authnpb.AuthnRule {
	options := &options{}
	for _, opt := range opts {
		if opt == nil {
			panic("authn: option is nil")
		}
		opt(options)
	}

	rules := make(map[string]*authnpb.AuthnRule)
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
