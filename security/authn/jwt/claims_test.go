package jwt

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	security "github.com/Servora-Kit/plateau/security"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

type CustomClaims struct {
	HumanClaims
	WorkspaceID string `json:"workspace_id"`
}

func (c *CustomClaims) Validate() error {
	if err := c.HumanClaims.Validate(); err != nil {
		return err
	}
	if c.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	return nil
}

func validServiceAccessClaims(clientID string) *ServiceClaims {
	now := time.Now()
	return &ServiceClaims{
		AccessClaims: AccessClaims{
			RegisteredClaims: jwtlib.RegisteredClaims{
				Issuer:    "issuer-a",
				Audience:  jwtlib.ClaimStrings{"audience-a"},
				Subject:   clientID,
				IssuedAt:  jwtlib.NewNumericDate(now),
				ExpiresAt: jwtlib.NewNumericDate(now.Add(5 * time.Minute)),
				ID:        "jti-svc-1",
			},
			ClientID:  clientID,
			TokenUse:  "access",
			ActorType: security.ActorTypeService,
		},
	}
}

func validHumanAccessClaims(userID, clientID string) *HumanClaims {
	now := time.Now()
	return &HumanClaims{
		AccessClaims: AccessClaims{
			RegisteredClaims: jwtlib.RegisteredClaims{
				Issuer:    "issuer-a",
				Audience:  jwtlib.ClaimStrings{"audience-a"},
				Subject:   userID,
				IssuedAt:  jwtlib.NewNumericDate(now),
				ExpiresAt: jwtlib.NewNumericDate(now.Add(5 * time.Minute)),
				ID:        "jti-hum-1",
			},
			ClientID:  clientID,
			TokenUse:  "access",
			ActorType: security.ActorTypeHuman,
		},
	}
}

func signAnyClaims(t *testing.T, claims jwtlib.Claims, signer interface {
	Sign(jwtlib.Claims) (string, error)
}) string {
	t.Helper()
	raw, err := signer.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAuthenticateForServiceAndHumanClaimsAndCrossRejection(t *testing.T) {
	signer, _, authenticator := newAuthenticator(t)
	ctx := context.Background()

	svcToken := signAnyClaims(t, validServiceAccessClaims("admin"), signer)
	humToken := signAnyClaims(t, validHumanAccessClaims("user-42", "web-app"), signer)

	svcActor, err := AuthenticateFor[ServiceClaims](ctx, authenticator, "Bearer "+svcToken)
	if err != nil || svcActor != (security.Actor{Type: security.ActorTypeService, ID: "admin"}) {
		t.Fatalf("AuthenticateFor[ServiceClaims] = %+v, err = %v", svcActor, err)
	}

	humActor, err := AuthenticateFor[HumanClaims](ctx, authenticator, "Bearer "+humToken)
	if err != nil || humActor != (security.Actor{Type: security.ActorTypeHuman, ID: "user-42"}) {
		t.Fatalf("AuthenticateFor[HumanClaims] = %+v, err = %v", humActor, err)
	}

	// Cross-actor rejection: human token rejected by ServiceClaims and service token rejected by HumanClaims.
	if _, err := AuthenticateFor[ServiceClaims](ctx, authenticator, "Bearer "+humToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected human token to be rejected by ServiceClaims with ErrInvalidToken, got %v", err)
	}
	if _, err := AuthenticateFor[HumanClaims](ctx, authenticator, "Bearer "+svcToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected service token to be rejected by HumanClaims with ErrInvalidToken, got %v", err)
	}

	// ID token (without token_use=access) must be rejected by both ServiceClaims and HumanClaims.
	now := time.Now()
	idToken := signAnyClaims(t, &jwtlib.MapClaims{
		"iss":   "issuer-a",
		"aud":   "audience-a",
		"sub":   "user-42",
		"iat":   now.Unix(),
		"exp":   now.Add(5 * time.Minute).Unix(),
		"jti":   "id-token-jti",
		"nonce": "n-1",
	}, signer)
	if _, err := AuthenticateFor[HumanClaims](ctx, authenticator, "Bearer "+idToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ID token without token_use=access to be rejected by HumanClaims, got %v", err)
	}
	if _, err := AuthenticateFor[ServiceClaims](ctx, authenticator, "Bearer "+idToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ID token without token_use=access to be rejected by ServiceClaims, got %v", err)
	}

	// Missing jti, missing iat, mismatched service sub/client_id, and empty human sub must be rejected.
	missingJTI := validHumanAccessClaims("user-42", "web-app")
	missingJTI.ID = ""
	if _, err := AuthenticateFor[HumanClaims](ctx, authenticator, "Bearer "+signAnyClaims(t, missingJTI, signer)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("missing jti err = %v, want ErrInvalidToken", err)
	}

	missingIAT := validServiceAccessClaims("admin")
	missingIAT.IssuedAt = nil
	if _, err := AuthenticateFor[ServiceClaims](ctx, authenticator, "Bearer "+signAnyClaims(t, missingIAT, signer)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("missing iat err = %v, want ErrInvalidToken", err)
	}

	mismatchedSvc := validServiceAccessClaims("admin")
	mismatchedSvc.Subject = "other-service"
	if _, err := AuthenticateFor[ServiceClaims](ctx, authenticator, "Bearer "+signAnyClaims(t, mismatchedSvc, signer)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("mismatched sub/client_id err = %v, want ErrInvalidToken", err)
	}

	emptySubHum := validHumanAccessClaims("", "web-app")
	if _, err := AuthenticateFor[HumanClaims](ctx, authenticator, "Bearer "+signAnyClaims(t, emptySubHum, signer)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("empty human sub err = %v, want ErrInvalidToken", err)
	}
}

func TestServerForAndEmbeddedCustomClaims(t *testing.T) {
	signer, _, authenticator := newAuthenticator(t)
	requiredRules := middlewareRules(authnpb.AuthnMode_AUTHN_MODE_REQUIRED)

	svcMiddleware := ServerFor[ServiceClaims](authenticator, requiredRules)
	humMiddleware := ServerFor[HumanClaims](authenticator, requiredRules)
	customMiddleware := ServerFor[CustomClaims](authenticator, requiredRules)

	svcToken := signAnyClaims(t, validServiceAccessClaims("iam-sync"), signer)
	humToken := signAnyClaims(t, validHumanAccessClaims("user-99", "admin"), signer)

	gotSvc, err := invokeMiddleware(t, svcMiddleware, middlewareContext("Bearer "+svcToken))
	if err != nil || gotSvc != (security.Actor{Type: security.ActorTypeService, ID: "iam-sync"}) {
		t.Fatalf("ServerFor[ServiceClaims] actor=%+v err=%v", gotSvc, err)
	}
	if _, err := invokeMiddleware(t, svcMiddleware, middlewareContext("Bearer "+humToken)); !securityerrorspb.IsSecurityErrorReasonUnauthenticated(err) {
		t.Fatalf("ServerFor[ServiceClaims] accepted human token or wrong error: %v", err)
	}

	gotHum, err := invokeMiddleware(t, humMiddleware, middlewareContext("Bearer "+humToken))
	if err != nil || gotHum != (security.Actor{Type: security.ActorTypeHuman, ID: "user-99"}) {
		t.Fatalf("ServerFor[HumanClaims] actor=%+v err=%v", gotHum, err)
	}
	if _, err := invokeMiddleware(t, humMiddleware, middlewareContext("Bearer "+svcToken)); !securityerrorspb.IsSecurityErrorReasonUnauthenticated(err) {
		t.Fatalf("ServerFor[HumanClaims] accepted service token or wrong error: %v", err)
	}

	// CustomClaims without workspace_id fails validation; with workspace_id succeeds and maps human Actor.
	if _, err := invokeMiddleware(t, customMiddleware, middlewareContext("Bearer "+humToken)); !securityerrorspb.IsSecurityErrorReasonUnauthenticated(err) {
		t.Fatalf("ServerFor[CustomClaims] accepted token without workspace_id: %v", err)
	}

	withWorkspace := &CustomClaims{
		HumanClaims: *validHumanAccessClaims("user-99", "admin"),
		WorkspaceID: "ws-001",
	}
	customToken := signAnyClaims(t, withWorkspace, signer)
	gotCustom, err := invokeMiddleware(t, customMiddleware, middlewareContext("Bearer "+customToken))
	if err != nil || gotCustom != (security.Actor{Type: security.ActorTypeHuman, ID: "user-99"}) {
		t.Fatalf("ServerFor[CustomClaims] actor=%+v err=%v", gotCustom, err)
	}
}
