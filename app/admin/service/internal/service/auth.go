package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	adminauthpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/admin/auth/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	"github.com/Servora-Kit/plateau/security"
	"github.com/Servora-Kit/plateau/security/audit"
	"github.com/Servora-Kit/plateau/security/authn/session"
	securityoidc "github.com/Servora-Kit/plateau/security/oidc"
	"github.com/alexedwards/scs/v2"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

const (
	oidcLoginPath    = "/v1/admin/auth/login"
	oidcCallbackPath = "/v1/admin/auth/callback"
	webLoginPath     = "/auth/login"
)

// AuthService implements plateau.admin.auth.v1.AuthServiceServer and browser OIDC login/callback handlers.
type AuthService struct {
	adminauthpb.UnimplementedAuthServiceServer
	uc      *biz.AuthUsecase
	seed    *biz.SeedInitializer
	oidcRP  *securityoidc.RelyingParty
	manager *scs.SessionManager
	log     *slog.Logger
}

// NewAuthService constructs AuthService for OIDC browser handshake, GetSession, and local Logout.
func NewAuthService(
	uc *biz.AuthUsecase,
	seed *biz.SeedInitializer,
	oidcCfg *oidcpb.OIDCClient,
	manager *scs.SessionManager,
	l *slog.Logger,
) (*AuthService, error) {
	if uc == nil || seed == nil || manager == nil {
		return nil, fmt.Errorf("admin auth service: dependency is nil")
	}
	oidcRP, err := securityoidc.NewRelyingParty(oidcCfg)
	if err != nil {
		return nil, fmt.Errorf("admin auth service: %w", err)
	}
	if l == nil {
		l = slog.Default()
	}
	return &AuthService{
		uc:      uc,
		seed:    seed,
		oidcRP:  oidcRP,
		manager: manager,
		log:     l.With("scope", "admin/service/auth"),
	}, nil
}

// RegisterOIDCHTTP registers the browser OIDC authorization redirect and callback endpoints.
func (s *AuthService) RegisterOIDCHTTP(srv *khttp.Server, auditor audit.Auditor) {
	srv.Handle(oidcLoginPath, http.HandlerFunc(s.handleLogin))
	srv.Handle(
		oidcCallbackPath,
		audit.HTTPHandler(auditor, "plateau.admin.auth.callback.v1", audit.CategoryAuthn)(http.HandlerFunc(s.handleCallback)),
	)
}

func (s *AuthService) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if err := s.oidcRP.BeginLogin(w, r, "/"); err != nil {
		s.log.ErrorContext(r.Context(), "OIDC BeginLogin failed", "err", err)
		http.Redirect(w, r, webLoginPath+"?error=service_unavailable", http.StatusFound)
	}
}

func (s *AuthService) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	user, returnTo, err := s.oidcRP.CompleteLogin(w, r)
	if err != nil {
		_ = s.manager.Destroy(r.Context())
		if errors.Is(err, securityoidc.ErrProviderUnavailable) {
			audit.SetError(r.Context(), "OIDC_PROVIDER_UNAVAILABLE", "identity provider is unavailable")
			s.log.ErrorContext(r.Context(), "OIDC CompleteLogin provider unavailable", "err", err)
			http.Redirect(w, r, webLoginPath+"?error=service_unavailable", http.StatusFound)
			return
		}
		audit.SetError(r.Context(), "OIDC_LOGIN_FAILED", "OIDC callback verification failed")
		http.Redirect(w, r, webLoginPath+"?error=login_failed", http.StatusFound)
		return
	}

	audit.SetActor(r.Context(), security.Actor{Type: security.ActorTypeHuman, ID: user.GetUserId()})
	audit.SetResource(r.Context(), "iam.plateau.dev/User", "users/"+user.GetUserId())

	if !s.seed.Seeded() {
		if seedErr := s.seed.Ensure(r.Context()); seedErr != nil {
			s.log.WarnContext(r.Context(), "lazy admin seed sync failed during callback", "err", seedErr)
		}
	}

	allowed, err := s.uc.CheckAccess(r.Context(), user.GetUserId())
	if err != nil {
		_ = s.manager.Destroy(r.Context())
		audit.SetError(r.Context(), "AUTHZ_CHECK_UNAVAILABLE", "authorization check unavailable")
		s.log.ErrorContext(r.Context(), "OpenFGA access check failed during callback", "err", err)
		http.Redirect(w, r, webLoginPath+"?error=service_unavailable", http.StatusFound)
		return
	}
	if !allowed {
		_ = s.manager.Destroy(r.Context())
		audit.SetEventOverride(r.Context(), audit.AuthzDeniedEventType, audit.CategoryAuthz)
		audit.SetResource(r.Context(), biz.AdminResourceType, biz.AdminResourceType+"/"+biz.AdminResourceID)
		audit.SetError(r.Context(), "SECURITY_ERROR_REASON_PERMISSION_DENIED", "admin access denied")
		http.Redirect(w, r, webLoginPath+"?error=access_denied", http.StatusFound)
		return
	}

	if err := session.SaveUser(r.Context(), s.manager, user); err != nil {
		_ = s.manager.Destroy(r.Context())
		audit.SetError(r.Context(), "SESSION_PERSIST_FAILED", "admin session persistence failed")
		s.log.ErrorContext(r.Context(), "persist admin session failed during callback", "err", err)
		http.Redirect(w, r, webLoginPath+"?error=service_unavailable", http.StatusFound)
		return
	}

	http.Redirect(w, r, returnTo, http.StatusFound)
}

func (s *AuthService) GetSession(ctx context.Context, _ *adminauthpb.GetSessionRequest) (*adminauthpb.GetSessionResponse, error) {
	user, ok := session.UserFrom(ctx)
	actor, actorOK := security.ActorFrom(ctx)
	if !ok || !actorOK || !actor.Valid() {
		return nil, securityerrorspb.ErrorSecurityErrorReasonUnauthenticated("authentication required")
	}

	perms, err := s.uc.Permissions(ctx, actor)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, kerrors.ClientClosed("CLIENT_CLOSED", "client closed request").WithCause(err)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, kerrors.GatewayTimeout("GATEWAY_TIMEOUT", "request deadline exceeded").WithCause(err)
		}
		return nil, securityerrorspb.ErrorSecurityErrorReasonUnavailable("authorization service unavailable").WithCause(err)
	}

	return &adminauthpb.GetSessionResponse{
		User:        user,
		Permissions: perms,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, _ *adminauthpb.LogoutRequest) (*adminauthpb.LogoutResponse, error) {
	if actor, ok := security.ActorFrom(ctx); ok && actor.Type == security.ActorTypeHuman && actor.ID != "" {
		audit.SetResource(ctx, "iam.plateau.dev/User", "users/"+actor.ID)
	}
	if err := s.manager.Destroy(ctx); err != nil {
		return nil, kerrors.InternalServer("SESSION_DESTROY_FAILED", "failed to destroy local admin session").WithCause(err)
	}
	return &adminauthpb.LogoutResponse{}, nil
}
