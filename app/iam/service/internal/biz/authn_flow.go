package biz

import (
	"errors"
	"fmt"
	"net/url"
	"slices"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
)

// AuthPolicy defines factor satisfaction for extensible multi-factor authentication evaluation.
type AuthPolicy interface {
	Satisfied(completedAMR []string) (satisfied bool, available []authnpb.AuthFactorMethod)
}

type passwordAuthPolicy struct{}

func (passwordAuthPolicy) Satisfied(completedAMR []string) (bool, []authnpb.AuthFactorMethod) {
	if slices.Contains(completedAMR, AMRPassword) {
		return true, nil
	}
	return false, []authnpb.AuthFactorMethod{authnpb.AuthFactorMethod_AUTH_FACTOR_METHOD_PASSWORD}
}

// DefaultAuthPolicy returns the phase-1 password factor policy.
func DefaultAuthPolicy() AuthPolicy {
	return passwordAuthPolicy{}
}

// EvaluateFlowProgress computes the next AuthFlow Proto stage from completed AMR values, factor policy, and required actions.
func EvaluateFlowProgress(completedAMR []string, policy AuthPolicy, remainingActions []userpb.UserRequiredAction) *authnpb.AuthFlow {
	normalizedAMR := NormalizeAMR(completedAMR)
	if ok, methods := policy.Satisfied(normalizedAMR); !ok {
		return &authnpb.AuthFlow{
			State:            authnpb.AuthFlowState_AUTH_FLOW_STATE_AUTHENTICATE,
			AvailableMethods: slices.Clone(methods),
			CompletedAmr:     normalizedAMR,
		}
	}
	if len(remainingActions) > 0 {
		cloned := slices.Clone(remainingActions)
		return &authnpb.AuthFlow{
			State:                    authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION,
			CompletedAmr:             normalizedAMR,
			RemainingRequiredActions: cloned,
			NextRequiredAction:       cloned[0],
		}
	}
	return &authnpb.AuthFlow{
		State:        authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED,
		CompletedAmr: normalizedAMR,
	}
}

// NormalizeAMR deduplicates and deterministically sorts RFC 8176 authentication method references.
func NormalizeAMR(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// BuildContinueURL formats the browser continuation target once primary authentication and required actions finish.
func BuildContinueURL(oidcRequestID string) string {
	if oidcRequestID != "" {
		return "/authorize/callback?id=" + url.QueryEscape(oidcRequestID)
	}
	return "/account/"
}

func buildOIDCCancelRedirectURI(rawRedirectURI, state, issuer string) (string, error) {
	parsed, err := url.Parse(rawRedirectURI)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("parse OIDC cancel redirect URI: %w", err)
	}
	query := parsed.Query()
	query.Set("error", "access_denied")
	if state != "" {
		query.Set("state", state)
	}
	query.Set("iss", issuer)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func buildOIDCContext(clientID string) *authnpb.AuthFlowOidcContext {
	if clientID == "" {
		return nil
	}
	return &authnpb.AuthFlowOidcContext{ClientId: clientID}
}

func isTerminalFlowStatus(status string) bool {
	switch status {
	case AuthFlowStatusCompleted, AuthFlowStatusCancelled, AuthFlowStatusReset:
		return true
	default:
		return false
	}
}

func isInactiveLoginError(err error) bool {
	return errors.Is(err, ErrUnauthenticated) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrSessionRevoked) ||
		errors.Is(err, ErrUserDisabled) ||
		errors.Is(err, ErrUserNotActive) ||
		errors.Is(err, ErrRequiredActionPending)
}
