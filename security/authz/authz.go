// Package authz provides engine-neutral authorization contracts and Kratos
// server middleware.
package authz

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"reflect"

	authnpb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/authn/v1"
	authzpb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/authz/v1"
	authn "github.com/Servora-Kit/servora-platform/security/authn"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
)

// Resource identifies one authorization target.
type Resource struct {
	Type string
	ID   string
}

// CheckRequest describes one engine-neutral authorization decision.
// Subject must be a stable, non-replayable identity identifier, never a credential.
type CheckRequest struct {
	Subject  string
	Action   string
	Resource Resource
}

// Authorizer is the single-method authorization contract.
type Authorizer interface {
	Check(context.Context, CheckRequest) (bool, error)
}

// BatchAuthorizer is the optional ordered batch-check capability.
type BatchAuthorizer interface {
	Authorizer
	BatchCheck(context.Context, []CheckRequest) ([]bool, error)
}

// Lister is the optional capability for listing allowed resource IDs.
type Lister interface {
	Authorizer
	ListAllowed(ctx context.Context, subject, action, resourceType string) ([]string, error)
}

// ErrUnavailable identifies a temporarily unavailable authorization dependency.
var ErrUnavailable = stderrors.New("authz: unavailable")

var (
	errMissingSubject   = stderrors.New("authz: trusted subject is missing")
	errMissingTransport = stderrors.New("authz: server transport is missing")
)

// Option configures Server.
type Option func(*serverConfig)

type serverConfig struct {
	rules       map[string]*authzpb.AuthzRule
	subjectFunc func(context.Context) (string, bool)
	logger      *slog.Logger
}

type compiledRule struct {
	mode            authzpb.AuthzMode
	action          string
	resourceType    string
	resourceIDField string
}

type authorizationOutcome string

func (outcome authorizationOutcome) String() string { return string(outcome) }

const authorizationAllowed authorizationOutcome = "ALLOWED"

// WithRulesFuncs merges generated rule tables. Later entries win.
func WithRulesFuncs(functions ...func() map[string]*authzpb.AuthzRule) Option {
	return func(config *serverConfig) {
		for _, function := range functions {
			if function == nil {
				continue
			}
			for operation, rule := range function() {
				if rule == nil {
					continue
				}
				if config.rules == nil {
					config.rules = make(map[string]*authzpb.AuthzRule)
				}
				config.rules[operation] = rule
			}
		}
	}
}

// WithSubjectFunc overrides the standard authn.SubjectFrom reader.
func WithSubjectFunc(function func(context.Context) (string, bool)) Option {
	return func(config *serverConfig) { config.subjectFunc = function }
}

// WithLogger configures root middleware diagnostics. It does not inject the
// logger into Authorizer implementations.
func WithLogger(logger *slog.Logger) Option {
	return func(config *serverConfig) { config.logger = logger }
}

// Server constructs authorization middleware and validates static wiring before returning it.
func Server(authorizer Authorizer, options ...Option) middleware.Middleware {
	if isNilAuthorizer(authorizer) {
		panic("authz: authorizer is nil")
	}

	config := &serverConfig{subjectFunc: authn.SubjectFrom}
	for index, option := range options {
		if option == nil {
			panic(fmt.Sprintf("authz: option[%d] is nil", index))
		}
		option(config)
		if config.subjectFunc == nil {
			panic(fmt.Sprintf("authz: option[%d] set subject function to nil", index))
		}
	}
	compiledRules := compileRules(config.rules)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, request any) (any, error) {
			transport, ok := transport.FromServerContext(ctx)
			if !ok || transport == nil {
				return nil, failAuthorization(ctx, config.logger, "", CheckRequest{}, authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_INTERNAL, errMissingTransport)
			}

			operation := transport.Operation()
			rule, found := compiledRules[operation]
			if !found {
				cause := fmt.Errorf("authz: no authorization rule for operation %q", operation)
				return nil, failAuthorization(ctx, config.logger, operation, CheckRequest{}, authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_INTERNAL, cause)
			}
			if rule.mode == authzpb.AuthzMode_AUTHZ_MODE_NONE {
				return handler(ctx, request)
			}

			subject, ok := config.subjectFunc(ctx)
			if !ok || subject == "" {
				return nil, authnpb.ErrorAuthnErrorReasonUnauthenticated("authentication failed").WithCause(concealAuthorizationCause(errMissingSubject))
			}

			checkRequest := CheckRequest{
				Subject: subject,
				Action:  rule.action,
				Resource: Resource{
					Type: rule.resourceType,
				},
			}
			resource, err := resolveResource(rule, request)
			if err != nil {
				checkRequest.Resource = resource
				return nil, failAuthorization(ctx, config.logger, operation, checkRequest, authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_INVALID_REQUEST, err)
			}
			checkRequest.Resource = resource
			if err := validateCheckRequest(checkRequest); err != nil {
				return nil, failAuthorization(ctx, config.logger, operation, checkRequest, authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_INVALID_REQUEST, err)
			}

			allowed, checkErr := authorizer.Check(ctx, checkRequest)
			if checkErr != nil {
				reason := authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_INTERNAL
				if stderrors.Is(checkErr, ErrUnavailable) {
					reason = authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_UNAVAILABLE
				}
				return nil, failAuthorization(ctx, config.logger, operation, checkRequest, reason, checkErr)
			}
			if !allowed {
				return nil, failAuthorization(ctx, config.logger, operation, checkRequest, authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_DENIED, nil)
			}
			logAuthorization(ctx, config.logger, slog.LevelInfo, operation, checkRequest, authorizationAllowed)
			return handler(ctx, request)
		}
	}
}

func compileRules(rules map[string]*authzpb.AuthzRule) map[string]compiledRule {
	compiled := make(map[string]compiledRule, len(rules))
	for operation, rule := range rules {
		compiled[operation] = compiledRule{
			mode:            rule.GetMode(),
			action:          rule.GetAction(),
			resourceType:    rule.GetResourceType(),
			resourceIDField: rule.GetResourceIdField(),
		}
	}
	return compiled
}

func isNilAuthorizer(authorizer Authorizer) bool {
	if authorizer == nil {
		return true
	}
	value := reflect.ValueOf(authorizer)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func validateCheckRequest(request CheckRequest) error {
	if request.Subject == "" {
		return fmt.Errorf("authz: subject is empty")
	}
	if request.Action == "" {
		return fmt.Errorf("authz: action is empty")
	}
	if request.Resource.Type == "" {
		return fmt.Errorf("authz: resource type is empty")
	}
	if request.Resource.ID == "" {
		return fmt.Errorf("authz: resource ID is empty")
	}
	return nil
}

func failAuthorization(ctx context.Context, logger *slog.Logger, operation string, request CheckRequest, reason authzpb.AuthzErrorReason, cause error) error {
	logAuthorization(ctx, logger, slog.LevelError, operation, request, reason)
	concealed := concealAuthorizationCause(cause)
	switch reason {
	case authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_DENIED:
		return authzpb.ErrorAuthzErrorReasonDenied("permission denied").WithCause(concealed)
	case authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_INVALID_REQUEST:
		return authzpb.ErrorAuthzErrorReasonInvalidRequest("invalid authorization request").WithCause(concealed)
	case authzpb.AuthzErrorReason_AUTHZ_ERROR_REASON_UNAVAILABLE:
		return authzpb.ErrorAuthzErrorReasonUnavailable("authorization service unavailable").WithCause(concealed)
	default:
		return authzpb.ErrorAuthzErrorReasonInternal("internal authorization error").WithCause(concealed)
	}
}

type concealedAuthorizationCause struct {
	cause error
}

func (cause concealedAuthorizationCause) Error() string { return "authorization cause withheld" }
func (cause concealedAuthorizationCause) Unwrap() error { return cause.cause }

func concealAuthorizationCause(cause error) error {
	if cause == nil {
		return nil
	}
	return concealedAuthorizationCause{cause: cause}
}

func logAuthorization(ctx context.Context, logger *slog.Logger, level slog.Level, operation string, request CheckRequest, reason fmt.Stringer) {
	if logger == nil {
		return
	}
	logger.LogAttrs(ctx, level, "authorization decision",
		slog.String("operation", operation),
		slog.String("action", request.Action),
		slog.String("resource_type", request.Resource.Type),
		slog.String("reason", reason.String()),
	)
}
