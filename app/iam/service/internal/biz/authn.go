package biz

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"time"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	iamconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/conf/v1"
	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/password"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

// PasswordCredential contains server-only authenticator fields absent from public Proto.
type PasswordCredential struct {
	UserID                  string
	AuthenticatorID         string
	PasswordAuthenticatorID string
	PasswordHash            string
	ChangedTime             time.Time
}

type CredentialRepo interface {
	FindActivePassword(context.Context, string) (*PasswordCredential, error)
	ReplacePassword(context.Context, string, string, string, string, string, time.Time) error
	SetPasswordAndRevokeSessions(context.Context, string, string, time.Time) error
}

// AuthFlowSnapshot pairs the public AuthFlow Proto with server-only fields absent from public Proto.
type AuthFlowSnapshot struct {
	Flow                    *authnpb.AuthFlow
	OwnerHash               string
	Status                  string
	OIDCRequestID           string
	UserID                  string
	UserEtag                string
	AuthenticatorID         string
	PasswordAuthenticatorID string
	PasswordChangedTime     *time.Time
}

type AuthFlowRepo interface {
	FindPendingOIDCRequest(ctx context.Context, requestID string, now time.Time) (*authnpb.AuthFlowOidcContext, time.Time, error)
	Create(ctx context.Context, flow *authnpb.AuthFlow, ownerHash, oidcRequestID string, now time.Time) (*authnpb.AuthFlow, error)
	Get(ctx context.Context, flowID string) (*AuthFlowSnapshot, error)
	Invalidate(ctx context.Context, flowID string, now time.Time) error
	AdvanceAfterPassword(ctx context.Context, flowID, ownerHash string, credential PasswordCredential, userEtag string, amr []string, stepTTL time.Duration, now time.Time) (*authnpb.AuthFlow, *LoginSession, error)
	CompleteUpdatePassword(ctx context.Context, flowID, ownerHash, newPasswordHash string, stepTTL time.Duration, now time.Time) (*authnpb.AuthFlow, *LoginSession, error)
	Reset(ctx context.Context, flowID, newFlowID, ownerHash string, now time.Time) (*authnpb.AuthFlow, string, error)
	Cancel(ctx context.Context, flowID, ownerHash string, now time.Time) (redirectURI, state, actorUserID string, err error)
}

type AuthenticationUsecase struct {
	issuer      string
	ttl         time.Duration
	stepTTL     time.Duration
	users       UserRepo
	credentials CredentialRepo
	sessions    *SessionUsecase
	flows       AuthFlowRepo
	consents    OAuthConsentRepo
	now         func() time.Time
}

// NewAuthenticationUsecase validates auth_flow lifetime settings and wires authentication flow dependencies.
func NewAuthenticationUsecase(
	config *iamconfpb.IAM,
	oidcConfig *oidcconfpb.OIDC,
	users UserRepo,
	credentials CredentialRepo,
	sessions *SessionUsecase,
	flows AuthFlowRepo,
	consents OAuthConsentRepo,
) (*AuthenticationUsecase, error) {
	if config == nil || oidcConfig == nil || users == nil || credentials == nil || sessions == nil || flows == nil || consents == nil {
		return nil, fmt.Errorf("authentication: dependency is nil")
	}
	issuer, _, err := NormalizeOIDCIssuer(oidcConfig.GetIssuer())
	if err != nil {
		return nil, fmt.Errorf("authentication config: %w", err)
	}
	cloned := proto.CloneOf(config)
	if cloned.AuthFlow == nil {
		cloned.AuthFlow = &iamconfpb.AuthFlow{}
	}
	if err := cloned.AuthFlow.Apply(); err != nil {
		return nil, fmt.Errorf("authentication config: %w", err)
	}
	if err := cloned.GetAuthFlow().GetTtl().CheckValid(); err != nil {
		return nil, fmt.Errorf("authentication config auth_flow.ttl: %w", err)
	}
	if err := cloned.GetAuthFlow().GetStepTtl().CheckValid(); err != nil {
		return nil, fmt.Errorf("authentication config auth_flow.step_ttl: %w", err)
	}
	ttl := cloned.GetAuthFlow().GetTtl().AsDuration()
	stepTTL := cloned.GetAuthFlow().GetStepTtl().AsDuration()
	if ttl <= 0 {
		return nil, fmt.Errorf("authentication config: auth_flow.ttl must be positive")
	}
	if stepTTL <= 0 {
		return nil, fmt.Errorf("authentication config: auth_flow.step_ttl must be positive")
	}
	if stepTTL > ttl {
		return nil, fmt.Errorf("authentication config: auth_flow.step_ttl (%s) must not exceed auth_flow.ttl (%s)", stepTTL, ttl)
	}
	return &AuthenticationUsecase{
		issuer:      issuer,
		ttl:         ttl,
		stepTTL:     stepTTL,
		users:       users,
		credentials: credentials,
		sessions:    sessions,
		flows:       flows,
		consents:    consents,
		now:         time.Now,
	}, nil
}

// CreateAuthFlow creates a browser-bound flow or short-circuits when the browser already holds an active login session.
func (uc *AuthenticationUsecase) CreateAuthFlow(
	ctx context.Context,
	loginID, ownerToken, oidcRequestID string,
) (*authnpb.AuthFlow, string, error) {
	now := uc.now().UTC()
	var (
		oidcCtx     *authnpb.AuthFlowOidcContext
		oidcExpires time.Time
	)
	if oidcRequestID != "" {
		var err error
		oidcCtx, oidcExpires, err = uc.flows.FindPendingOIDCRequest(ctx, oidcRequestID, now)
		if errors.Is(err, ErrNotFound) {
			return nil, "", ErrOIDCRequestInvalid
		}
		if err != nil {
			return nil, "", fmt.Errorf("find pending OIDC request: %w", err)
		}
	}

	if loginID != "" {
		user, loginSession, err := uc.sessions.Resolve(ctx, loginID)
		if err != nil {
			if !isInactiveLoginError(err) {
				return nil, "", fmt.Errorf("resolve existing login session: %w", err)
			}
		} else {
			amr := NormalizeAMR(loginSession.AMR)
			if oidcRequestID == "" || len(amr) > 0 {
				return &authnpb.AuthFlow{
					State:        authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED,
					CompletedAmr: amr,
					Email:        user.GetEmail(),
					Oidc:         oidcCtx,
					ContinueUrl:  BuildContinueURL(oidcRequestID),
					User:         user,
				}, "", nil
			}
		}
	}

	var (
		newOwnerToken string
		ownerHash     string
	)
	if ownerToken == "" {
		token, hash, err := NewOpaqueSecret()
		if err != nil {
			return nil, "", err
		}
		newOwnerToken = token
		ownerHash = hash
	} else {
		ownerHash = HashOpaqueSecret(ownerToken)
	}
	flowID, _, err := NewOpaqueSecret()
	if err != nil {
		return nil, "", err
	}
	expiresTime := now.Add(uc.ttl)
	if oidcRequestID != "" && oidcExpires.Before(expiresTime) {
		expiresTime = oidcExpires
	}
	if !now.Before(expiresTime) {
		return nil, "", ErrOIDCRequestInvalid
	}
	seed := EvaluateFlowProgress(nil, DefaultAuthPolicy(), nil)
	seed.FlowId = flowID
	seed.Oidc = oidcCtx
	seed.ExpiresTime = timestamppb.New(expiresTime)
	created, err := uc.flows.Create(ctx, seed, ownerHash, oidcRequestID, now)
	if err != nil {
		return nil, "", err
	}
	return created, newOwnerToken, nil
}

// GetAuthFlow returns the current state of an owned authentication flow with zero write side-effects.
func (uc *AuthenticationUsecase) GetAuthFlow(
	ctx context.Context,
	ownerToken, loginID, flowID string,
) (*authnpb.AuthFlow, error) {
	now := uc.now().UTC()
	snapshot, _, err := uc.loadOwnedFlow(ctx, ownerToken, flowID, now)
	if err != nil {
		return nil, err
	}

	if loginID != "" {
		user, loginSession, err := uc.sessions.Resolve(ctx, loginID)
		if err != nil {
			if !isInactiveLoginError(err) {
				return nil, fmt.Errorf("resolve existing login session: %w", err)
			}
		} else {
			amr := NormalizeAMR(loginSession.AMR)
			if snapshot.OIDCRequestID == "" {
				return &authnpb.AuthFlow{
					FlowId:       snapshot.Flow.GetFlowId(),
					State:        authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED,
					CompletedAmr: amr,
					Email:        user.GetEmail(),
					ExpiresTime:  proto.CloneOf(snapshot.Flow.GetExpiresTime()),
					ContinueUrl:  "/account/",
					User:         user,
				}, nil
			}
			if len(amr) > 0 {
				oidcCtx, _, err := uc.flows.FindPendingOIDCRequest(ctx, snapshot.OIDCRequestID, now)
				if err != nil && !errors.Is(err, ErrNotFound) {
					return nil, fmt.Errorf("find pending OIDC request: %w", err)
				}
				continueTarget := "/account/"
				if err == nil {
					continueTarget = BuildContinueURL(snapshot.OIDCRequestID)
				}
				return &authnpb.AuthFlow{
					FlowId:       snapshot.Flow.GetFlowId(),
					State:        authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED,
					CompletedAmr: amr,
					Email:        user.GetEmail(),
					Oidc:         oidcCtx,
					ExpiresTime:  proto.CloneOf(snapshot.Flow.GetExpiresTime()),
					ContinueUrl:  continueTarget,
					User:         user,
				}, nil
			}
		}
	}

	if snapshot.Status == AuthFlowStatusInvalidated {
		return nil, ErrAuthFlowInvalidated
	}
	if snapshot.Flow.GetState() == authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION {
		if snapshot.Flow.GetStepExpiresTime() == nil || !now.Before(snapshot.Flow.GetStepExpiresTime().AsTime()) {
			return nil, ErrAuthFlowExpired
		}
		_, _, valid, err := uc.checkRequiredActionSnapshot(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		if !valid {
			return nil, ErrAuthFlowInvalidated
		}
	}
	out := proto.CloneOf(snapshot.Flow)
	if snapshot.OIDCRequestID != "" {
		oidcCtx, _, err := uc.flows.FindPendingOIDCRequest(ctx, snapshot.OIDCRequestID, now)
		if errors.Is(err, ErrNotFound) {
			return nil, ErrOIDCRequestInvalid
		}
		if err != nil {
			return nil, fmt.Errorf("find pending OIDC request: %w", err)
		}
		out.Oidc = oidcCtx
	}
	return out, nil
}

// SubmitPassword verifies email and password credentials and advances or completes an owned authentication flow.
func (uc *AuthenticationUsecase) SubmitPassword(
	ctx context.Context,
	ownerToken, flowID, email, plainPassword string,
) (*authnpb.AuthFlow, *LoginSession, string, error) {
	now := uc.now().UTC()
	snapshot, ownerHash, err := uc.loadOwnedFlow(ctx, ownerToken, flowID, now)
	if err != nil {
		return nil, nil, "", err
	}
	if snapshot.Status == AuthFlowStatusInvalidated {
		return nil, nil, "", ErrAuthFlowInvalidated
	}
	if snapshot.Flow.GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_AUTHENTICATE {
		if snapshot.Flow.GetState() == authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION &&
			(snapshot.Flow.GetStepExpiresTime() == nil || !now.Before(snapshot.Flow.GetStepExpiresTime().AsTime())) {
			return nil, nil, "", ErrAuthFlowExpired
		}
		return nil, nil, "", ErrAuthFlowInvalidated
	}
	if snapshot.OIDCRequestID != "" {
		if _, _, err := uc.flows.FindPendingOIDCRequest(ctx, snapshot.OIDCRequestID, now); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, nil, "", ErrOIDCRequestInvalid
			}
			return nil, nil, "", fmt.Errorf("find pending OIDC request: %w", err)
		}
	}

	canonical := security.NormalizeEmail(email)
	if canonical == "" {
		return nil, nil, "", ErrInvalidCredentials
	}
	user, err := uc.users.FindByEmail(ctx, canonical)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, "", ErrInvalidCredentials
		}
		return nil, nil, "", fmt.Errorf("find login user: %w", err)
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !user.GetEmailVerified() {
		return nil, nil, "", ErrInvalidCredentials
	}
	credential, err := uc.credentials.FindActivePassword(ctx, user.GetUserId())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, "", ErrInvalidCredentials
		}
		return nil, nil, "", fmt.Errorf("find password credential: %w", err)
	}
	match, _, err := password.Compare(plainPassword, credential.PasswordHash)
	if err != nil || !match {
		return nil, nil, "", ErrInvalidCredentials
	}

	flow, loginSession, err := uc.flows.AdvanceAfterPassword(
		ctx,
		snapshot.Flow.GetFlowId(),
		ownerHash,
		*credential,
		user.GetEtag(),
		[]string{AMRPassword},
		uc.stepTTL,
		now,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrAuthFlowNotFound),
			errors.Is(err, ErrAuthFlowExpired),
			errors.Is(err, ErrAuthFlowInvalidated),
			errors.Is(err, ErrOIDCRequestInvalid):
			return nil, nil, "", err
		case errors.Is(err, ErrNotFound),
			errors.Is(err, ErrMutationMiss),
			errors.Is(err, ErrUserEtagMismatch),
			errors.Is(err, ErrInvalidCredentials),
			errors.Is(err, ErrUserNotActive),
			errors.Is(err, ErrUserDisabled),
			errors.Is(err, ErrUnauthenticated):
			return nil, nil, "", ErrInvalidCredentials
		default:
			return nil, nil, "", fmt.Errorf("advance auth flow after password: %w", err)
		}
	}
	return flow, loginSession, credential.UserID, nil
}

// CompleteUpdatePassword satisfies UPDATE_PASSWORD for an authenticated flow without re-prompting for the initial password.
func (uc *AuthenticationUsecase) CompleteUpdatePassword(
	ctx context.Context,
	ownerToken, flowID, newPassword string,
) (*authnpb.AuthFlow, *LoginSession, string, error) {
	now := uc.now().UTC()
	snapshot, ownerHash, err := uc.loadOwnedFlow(ctx, ownerToken, flowID, now)
	if err != nil {
		return nil, nil, "", err
	}
	if snapshot.Status == AuthFlowStatusInvalidated {
		return nil, nil, "", ErrAuthFlowInvalidated
	}
	if snapshot.Flow.GetState() != authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION || snapshot.UserID == "" {
		return nil, nil, "", ErrAuthFlowInvalidated
	}
	if snapshot.Flow.GetStepExpiresTime() == nil || !now.Before(snapshot.Flow.GetStepExpiresTime().AsTime()) {
		return nil, nil, "", ErrAuthFlowExpired
	}

	actorUserID := snapshot.UserID
	if snapshot.Flow.GetNextRequiredAction() != userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD {
		return nil, nil, actorUserID, ErrAuthFlowInvalidated
	}

	_, credential, valid, err := uc.checkRequiredActionSnapshot(ctx, snapshot)
	if err != nil {
		return nil, nil, actorUserID, err
	}
	if !valid {
		_ = uc.flows.Invalidate(ctx, flowID, now)
		return nil, nil, actorUserID, ErrAuthFlowInvalidated
	}

	if err := ValidatePassword(newPassword); err != nil {
		return nil, nil, actorUserID, ErrInvalidPassword
	}
	same, _, err := password.Compare(newPassword, credential.PasswordHash)
	if err != nil {
		return nil, nil, actorUserID, fmt.Errorf("compare new password: %w", err)
	}
	if same {
		return nil, nil, actorUserID, ErrInvalidPassword
	}

	newHash, err := password.Hash(newPassword)
	if err != nil {
		return nil, nil, actorUserID, fmt.Errorf("hash new password: %w", err)
	}
	flow, loginSession, err := uc.flows.CompleteUpdatePassword(ctx, flowID, ownerHash, newHash, uc.stepTTL, now)
	if err != nil {
		switch {
		case errors.Is(err, ErrAuthFlowNotFound),
			errors.Is(err, ErrAuthFlowExpired),
			errors.Is(err, ErrAuthFlowInvalidated),
			errors.Is(err, ErrOIDCRequestInvalid):
			return nil, nil, actorUserID, err
		default:
			return nil, nil, actorUserID, fmt.Errorf("complete update password: %w", err)
		}
	}
	return flow, loginSession, actorUserID, nil
}

// ResetAuthFlow rotates flow_id back to AUTHENTICATE while preserving any still-valid OIDC authorization request.
func (uc *AuthenticationUsecase) ResetAuthFlow(
	ctx context.Context,
	ownerToken, flowID string,
) (*authnpb.AuthFlow, string, error) {
	now := uc.now().UTC()
	snapshot, ownerHash, err := uc.loadOwnedFlow(ctx, ownerToken, flowID, now)
	if err != nil {
		return nil, "", err
	}
	newFlowID, _, err := NewOpaqueSecret()
	if err != nil {
		return nil, snapshot.UserID, err
	}
	resetFlow, actorUserID, err := uc.flows.Reset(ctx, flowID, newFlowID, ownerHash, now)
	if err != nil {
		return nil, snapshot.UserID, err
	}
	return resetFlow, actorUserID, nil
}

// CancelAuthFlow aborts an active owned authentication flow and destroys any pending OIDC authorization request.
func (uc *AuthenticationUsecase) CancelAuthFlow(
	ctx context.Context,
	ownerToken, flowID string,
) (string, string, error) {
	now := uc.now().UTC()
	snapshot, ownerHash, err := uc.loadOwnedFlow(ctx, ownerToken, flowID, now)
	if err != nil {
		return "", "", err
	}
	if snapshot.Status == AuthFlowStatusInvalidated {
		return "", "", ErrAuthFlowInvalidated
	}
	rawRedirectURI, state, actorUserID, err := uc.flows.Cancel(ctx, flowID, ownerHash, now)
	if err != nil {
		return "", snapshot.UserID, err
	}
	if rawRedirectURI == "" {
		return "", actorUserID, nil
	}
	redirectURI, err := buildOIDCCancelRedirectURI(rawRedirectURI, state, uc.issuer)
	if err != nil {
		return "", actorUserID, err
	}
	return redirectURI, actorUserID, nil
}

func (uc *AuthenticationUsecase) loadOwnedFlow(
	ctx context.Context,
	ownerToken, flowID string,
	now time.Time,
) (*AuthFlowSnapshot, string, error) {
	if flowID == "" || ownerToken == "" {
		return nil, "", ErrAuthFlowNotFound
	}
	snapshot, err := uc.flows.Get(ctx, flowID)
	if errors.Is(err, ErrNotFound) {
		return nil, "", ErrAuthFlowNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("get auth flow: %w", err)
	}
	if snapshot.OwnerHash == "" {
		return nil, "", ErrAuthFlowNotFound
	}
	ownerHash := HashOpaqueSecret(ownerToken)
	if subtle.ConstantTimeCompare([]byte(ownerHash), []byte(snapshot.OwnerHash)) != 1 {
		return nil, "", ErrAuthFlowNotFound
	}
	if isTerminalFlowStatus(snapshot.Status) {
		return nil, "", ErrAuthFlowNotFound
	}
	if snapshot.Flow.GetExpiresTime() == nil || !now.Before(snapshot.Flow.GetExpiresTime().AsTime()) {
		return nil, "", ErrAuthFlowExpired
	}
	return snapshot, ownerHash, nil
}

func (uc *AuthenticationUsecase) checkRequiredActionSnapshot(
	ctx context.Context,
	snapshot *AuthFlowSnapshot,
) (*userpb.User, *PasswordCredential, bool, error) {
	if snapshot.UserID == "" || snapshot.UserEtag == "" || snapshot.AuthenticatorID == "" || snapshot.PasswordAuthenticatorID == "" || snapshot.PasswordChangedTime == nil {
		return nil, nil, false, nil
	}
	user, err := uc.users.Get(ctx, snapshot.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, false, nil
		}
		return nil, nil, false, fmt.Errorf("find flow user: %w", err)
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !user.GetEmailVerified() || user.GetEtag() != snapshot.UserEtag {
		return nil, nil, false, nil
	}
	if !slices.Equal(user.GetRequiredActions(), snapshot.Flow.GetRemainingRequiredActions()) ||
		!slices.Contains(user.GetRequiredActions(), userpb.UserRequiredAction_USER_REQUIRED_ACTION_UPDATE_PASSWORD) {
		return nil, nil, false, nil
	}
	credential, err := uc.credentials.FindActivePassword(ctx, snapshot.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, false, nil
		}
		return nil, nil, false, fmt.Errorf("find flow password credential: %w", err)
	}
	if credential.AuthenticatorID != snapshot.AuthenticatorID || credential.PasswordAuthenticatorID != snapshot.PasswordAuthenticatorID {
		return nil, nil, false, nil
	}
	if !NormalizeTimestamp(credential.ChangedTime).Equal(NormalizeTimestamp(*snapshot.PasswordChangedTime)) {
		return nil, nil, false, nil
	}
	return user, credential, true, nil
}
