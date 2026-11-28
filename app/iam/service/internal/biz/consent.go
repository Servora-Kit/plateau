package biz

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
)

// OAuthConsentRepo manages user scope grants and third-party application revocations.
type OAuthConsentRepo interface {
	GetConsentView(ctx context.Context, userID, ownerHash, requestID string, now time.Time) (*authnpb.ConsentView, error)
	ApproveConsent(ctx context.Context, userID, ownerHash, requestID string, now time.Time) (clientID string, err error)
	DenyConsent(ctx context.Context, ownerHash, requestID string, now time.Time) (redirectURI, state, clientID string, err error)
	ListAuthorizedApps(ctx context.Context, userID string) ([]*accountpb.AuthorizedApp, error)
	RevokeAuthorizedApp(ctx context.Context, userID, clientID string, now time.Time) error
}

// MergeScopes returns the sorted, deduplicated union of two scope slices.
func MergeScopes(existing, requested []string) []string {
	merged := make([]string, 0, len(existing)+len(requested))
	for _, s := range slices.Concat(existing, requested) {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			merged = append(merged, trimmed)
		}
	}
	if len(merged) == 0 {
		return []string{}
	}
	slices.Sort(merged)
	return slices.Compact(merged)
}

// GetConsentView loads the application and requested/granted scopes for an authenticated user.
func (uc *AuthenticationUsecase) GetConsentView(
	ctx context.Context,
	user *userpb.User,
	ownerToken, requestID string,
) (*authnpb.ConsentView, error) {
	if user.GetUserId() == "" {
		return nil, ErrUnauthenticated
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || ownerToken == "" {
		return nil, ErrOIDCRequestInvalid
	}
	view, err := uc.consents.GetConsentView(ctx, user.GetUserId(), HashOpaqueSecret(ownerToken), requestID, uc.now().UTC())
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrOIDCRequestInvalid) {
			return nil, ErrOIDCRequestInvalid
		}
		return nil, fmt.Errorf("get consent view: %w", err)
	}
	view.UserEmail = user.GetEmail()
	return view, nil
}

// SubmitConsentDecision records user approval or denial for a pending OIDC authorization request.
func (uc *AuthenticationUsecase) SubmitConsentDecision(
	ctx context.Context,
	user *userpb.User,
	ownerToken, requestID string,
	approved bool,
) (redirectURI, clientID string, err error) {
	if user.GetUserId() == "" {
		return "", "", ErrUnauthenticated
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || ownerToken == "" {
		return "", "", ErrOIDCRequestInvalid
	}
	ownerHash := HashOpaqueSecret(ownerToken)
	now := uc.now().UTC()
	if approved {
		cid, err := uc.consents.ApproveConsent(ctx, user.GetUserId(), ownerHash, requestID, now)
		if err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrOIDCRequestInvalid) {
				return "", "", ErrOIDCRequestInvalid
			}
			if errors.Is(err, ErrUnauthenticated) || errors.Is(err, ErrUserNotActive) || errors.Is(err, ErrUserDisabled) {
				return "", "", ErrUnauthenticated
			}
			return "", "", fmt.Errorf("approve consent: %w", err)
		}
		return BuildContinueURL(requestID), cid, nil
	}

	rawRedirectURI, state, cid, err := uc.consents.DenyConsent(ctx, ownerHash, requestID, now)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrOIDCRequestInvalid) {
			return "", "", ErrOIDCRequestInvalid
		}
		return "", "", fmt.Errorf("deny consent: %w", err)
	}
	builtRedirect, err := buildOIDCCancelRedirectURI(rawRedirectURI, state, uc.issuer)
	if err != nil {
		return "", cid, err
	}
	return builtRedirect, cid, nil
}

// ListAuthorizedApps lists third-party OAuth applications authorized by the current user.
func (uc *AccountUsecase) ListAuthorizedApps(ctx context.Context, userID string) ([]*accountpb.AuthorizedApp, error) {
	if userID == "" {
		return nil, ErrUnauthenticated
	}
	return uc.consents.ListAuthorizedApps(ctx, userID)
}

// RevokeAuthorizedApp deletes the user's scope grant for one OAuth client and revokes its active token sessions.
func (uc *AccountUsecase) RevokeAuthorizedApp(ctx context.Context, userID, clientID string) error {
	if userID == "" {
		return ErrUnauthenticated
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return ErrNotFound
	}
	return uc.consents.RevokeAuthorizedApp(ctx, userID, clientID, uc.now().UTC())
}
