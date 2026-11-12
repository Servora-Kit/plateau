package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	adminauthpb "github.com/Servora-Kit/plateau/api/gen/go/admin/auth/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	adminauthn "github.com/Servora-Kit/plateau/app/admin/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	"github.com/Servora-Kit/plateau/security"
	securityoidc "github.com/Servora-Kit/plateau/security/oidc"
	"github.com/alexedwards/scs/v2"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	oidcLoginPath    = "/v1/admin/auth/login"
	oidcCallbackPath = "/v1/admin/auth/callback"
	webLoginPath     = "/auth/login"
)

// AuthService implements admin.auth.v1.AuthServiceServer and browser OIDC login/callback handlers.
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
func (s *AuthService) RegisterOIDCHTTP(srv *khttp.Server) {
	srv.Handle(oidcLoginPath, http.HandlerFunc(s.handleLogin))
	srv.Handle(oidcCallbackPath, http.HandlerFunc(s.handleCallback))
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

	identity, returnTo, err := s.oidcRP.CompleteLogin(w, r)
	if err != nil {
		_ = s.manager.Destroy(r.Context())
		if errors.Is(err, securityoidc.ErrProviderUnavailable) {
			s.log.ErrorContext(r.Context(), "OIDC CompleteLogin provider unavailable", "err", err)
			http.Redirect(w, r, webLoginPath+"?error=service_unavailable", http.StatusFound)
			return
		}
		http.Redirect(w, r, webLoginPath+"?error=login_failed", http.StatusFound)
		return
	}

	if !s.seed.Seeded() {
		if seedErr := s.seed.Ensure(r.Context()); seedErr != nil {
			s.log.WarnContext(r.Context(), "lazy admin seed sync failed during callback", "err", seedErr)
		}
	}

	allowed, err := s.uc.CheckAccess(r.Context(), identity.Subject)
	if err != nil {
		_ = s.manager.Destroy(r.Context())
		s.log.ErrorContext(r.Context(), "OpenFGA access check failed during callback", "err", err)
		http.Redirect(w, r, webLoginPath+"?error=service_unavailable", http.StatusFound)
		return
	}
	if !allowed {
		_ = s.manager.Destroy(r.Context())
		http.Redirect(w, r, webLoginPath+"?error=access_denied", http.StatusFound)
		return
	}

	var authTime *timestamppb.Timestamp
	if !identity.AuthTime.IsZero() {
		authTime = timestamppb.New(identity.AuthTime)
	}
	if err := adminauthn.SavePrincipal(r.Context(), s.manager, &adminauthpb.AdminPrincipal{
		UserId:   identity.Subject,
		Email:    identity.Email,
		Name:     identity.Name,
		Picture:  identity.Picture,
		AuthTime: authTime,
	}); err != nil {
		_ = s.manager.Destroy(r.Context())
		s.log.ErrorContext(r.Context(), "persist admin session failed during callback", "err", err)
		http.Redirect(w, r, webLoginPath+"?error=service_unavailable", http.StatusFound)
		return
	}

	http.Redirect(w, r, returnTo, http.StatusFound)
}

func (s *AuthService) GetSession(ctx context.Context, _ *adminauthpb.GetSessionRequest) (*adminauthpb.GetSessionResponse, error) {
	principal, ok := adminauthn.PrincipalFrom(ctx)
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
		User:        proto.Clone(principal).(*adminauthpb.AdminPrincipal),
		Permissions: perms,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, _ *adminauthpb.LogoutRequest) (*adminauthpb.LogoutResponse, error) {
	if err := s.manager.Destroy(ctx); err != nil {
		return nil, kerrors.InternalServer("SESSION_DESTROY_FAILED", "failed to destroy local admin session").WithCause(err)
	}
	return &adminauthpb.LogoutResponse{}, nil
}
