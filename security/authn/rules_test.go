package authn

import (
	"testing"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/v1"
)

func TestNewRulesMergesGeneratedProvidersInOrder(t *testing.T) {
	rules := NewRules(WithRulesFuncs(
		func() map[string]*authnpb.AuthnRule {
			return map[string]*authnpb.AuthnRule{
				"/test.v1.Service/Public": {Mode: authnpb.AuthnMode_AUTHN_MODE_PUBLIC},
				"/test.v1.Service/Get":    {Mode: authnpb.AuthnMode_AUTHN_MODE_PUBLIC},
			}
		},
		nil,
		func() map[string]*authnpb.AuthnRule {
			return map[string]*authnpb.AuthnRule{
				"/test.v1.Service/Get": {Mode: authnpb.AuthnMode_AUTHN_MODE_REQUIRED},
			}
		},
	))

	if got := rules["/test.v1.Service/Public"].GetMode(); got != authnpb.AuthnMode_AUTHN_MODE_PUBLIC {
		t.Fatalf("public mode = %s", got)
	}
	if got := rules["/test.v1.Service/Get"].GetMode(); got != authnpb.AuthnMode_AUTHN_MODE_REQUIRED {
		t.Fatalf("overridden mode = %s", got)
	}
}
