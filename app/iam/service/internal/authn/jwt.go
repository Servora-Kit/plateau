package authn

import (
	"context"
	"fmt"
	"strings"

	oidcpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	jwtpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authn/jwt/v1"
	jwtAuthn "github.com/Servora-Kit/plateau/security/authn/jwt"
	securityjwt "github.com/Servora-Kit/plateau/security/jwt"
)

func NewServiceAuthenticator(config *oidcpb.OIDC, verifier *securityjwt.Verifier) (*jwtAuthn.Authenticator, func(), error) {
	if config == nil {
		return nil, nil, fmt.Errorf("IAM OIDC config is nil")
	}
	if err := config.Apply(); err != nil {
		return nil, nil, fmt.Errorf("IAM OIDC config: %w", err)
	}
	return jwtAuthn.New(context.Background(), &jwtpb.JwtAuthnConfig{
		Issuer: strings.TrimSuffix(strings.TrimSpace(config.GetIssuer()), "/"), Audience: "iam",
	}, jwtAuthn.WithVerifier(verifier))
}
