package security

import (
	"testing"

	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/v1"
	authzpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authz/v1"
)

func TestNewRulesMergesGeneratedProvidersAcrossRuleTypes(t *testing.T) {
	authnRules := NewRules(WithRulesFuncs(
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
	if got := authnRules["/test.v1.Service/Public"].GetMode(); got != authnpb.AuthnMode_AUTHN_MODE_PUBLIC {
		t.Fatalf("public mode = %s", got)
	}
	if got := authnRules["/test.v1.Service/Get"].GetMode(); got != authnpb.AuthnMode_AUTHN_MODE_REQUIRED {
		t.Fatalf("overridden authn mode = %s", got)
	}

	authzRules := NewRules(WithRulesFuncs(
		func() map[string]*authzpb.AuthzRule {
			return map[string]*authzpb.AuthzRule{
				"/test.v1.Service/Get": {Mode: authzpb.AuthzMode_AUTHZ_MODE_NONE},
			}
		},
		func() map[string]*authzpb.AuthzRule {
			return map[string]*authzpb.AuthzRule{
				"/test.v1.Service/Get": {Mode: authzpb.AuthzMode_AUTHZ_MODE_REQUIRED},
			}
		},
	))
	if got := authzRules["/test.v1.Service/Get"].GetMode(); got != authzpb.AuthzMode_AUTHZ_MODE_REQUIRED {
		t.Fatalf("overridden authz mode = %s", got)
	}

	auditRules := NewRules(WithRulesFuncs(
		func() map[string]*auditpb.AuditRule {
			return map[string]*auditpb.AuditRule{
				"/test.v1.Service/Save": {
					Mode:     auditpb.AuditMode_AUDIT_MODE_ENABLED,
					Category: auditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE,
				},
			}
		},
	))
	if got := auditRules["/test.v1.Service/Save"].GetCategory(); got != auditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE {
		t.Fatalf("audit category = %s", got)
	}
}

func TestNewRulesPanicsOnNilOption(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil RulesOption")
		}
	}()
	_ = NewRules[*authnpb.AuthnRule](nil)
}
