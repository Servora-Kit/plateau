package authz

import (
	"testing"

	authzpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authz/v1"
)

func TestNewRulesMergesGeneratedProvidersInOrder(t *testing.T) {
	rules := NewRules(WithRulesFuncs(
		func() map[string]*authzpb.AuthzRule {
			return map[string]*authzpb.AuthzRule{
				"/test.v1.Service/Public": {Mode: authzpb.AuthzMode_AUTHZ_MODE_NONE},
				"/test.v1.Service/Get":    {Mode: authzpb.AuthzMode_AUTHZ_MODE_NONE},
			}
		},
		nil,
		func() map[string]*authzpb.AuthzRule {
			return map[string]*authzpb.AuthzRule{
				"/test.v1.Service/Get": {Mode: authzpb.AuthzMode_AUTHZ_MODE_REQUIRED},
			}
		},
	))

	if got := rules["/test.v1.Service/Public"].GetMode(); got != authzpb.AuthzMode_AUTHZ_MODE_NONE {
		t.Fatalf("public mode = %s", got)
	}
	if got := rules["/test.v1.Service/Get"].GetMode(); got != authzpb.AuthzMode_AUTHZ_MODE_REQUIRED {
		t.Fatalf("overridden mode = %s", got)
	}
}
