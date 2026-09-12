package authn

import (
	"context"
	"fmt"
	"strings"

	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	jwtpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	"github.com/Servora-Kit/plateau/security"
	jwtAuthn "github.com/Servora-Kit/plateau/security/authn/jwt"
	securityjwt "github.com/Servora-Kit/plateau/security/jwt"
	"github.com/golang-jwt/jwt/v5"
)

// ServiceClaims contains the signed fields required by IAM's service API.
type ServiceClaims struct {
	jwt.RegisteredClaims
	ClientID  string `json:"client_id"`
	TokenUse  string `json:"token_use"`
	ActorType string `json:"actor_type"`
}

func NewServiceClaims() *ServiceClaims { return &ServiceClaims{} }

func (claims *ServiceClaims) Validate() error {
	if claims.TokenUse != "access" || claims.ActorType != "service" || claims.ClientID == "" || claims.Subject != claims.ClientID {
		return fmt.Errorf("IAM requires a service access token")
	}
	if claims.IssuedAt == nil || claims.ID == "" {
		return fmt.Errorf("IAM access token is missing issuance claims")
	}
	return nil
}

func ServiceActor(claims *ServiceClaims) (security.Actor, error) {
	return security.Actor{Type: security.ActorTypeService, ID: claims.ClientID}, nil
}

func NewServiceAuthenticator(config *oidcpb.OIDC, verifier *securityjwt.Verifier) (*jwtAuthn.Authenticator, func(), error) {
	if config == nil {
		return nil, nil, fmt.Errorf("IAM OIDC config is nil")
	}
	return jwtAuthn.New(context.Background(), &jwtpb.JwtAuthnConfig{
		Issuer: strings.TrimSuffix(strings.TrimSpace(config.GetIssuer()), "/"), Audience: "iam",
	}, jwtAuthn.WithVerifier(verifier))
}
