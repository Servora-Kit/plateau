package biz

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Servora-Kit/plateau/security"
)

var ErrOAuthGrantInvalid = errors.New("OAuth grant is inactive or inconsistent")

// OAuthGrant is the verified protocol input rechecked within the issuance transaction.
type OAuthGrant struct {
	CodeID, RefreshTokenID, TokenSessionID string
	Subject, ClientID, LoginID             string
	Scopes, Audiences, AMR                 []string
	AuthTime                               time.Time
	Service                                bool
}

// OAuthTokenIssue contains application-generated IDs, hash and issuance deadlines.
type OAuthTokenIssue struct {
	AccessID, RefreshID, RefreshHash            string
	IssuedAt, AccessExpiresAt, RefreshExpiresAt time.Time
}

// OAuthRepo serializes user issuance and revocation with identity changes.
type OAuthRepo interface {
	CompleteAuthorizationRequest(context.Context, string, string, string, time.Time) error
	SaveAuthorizationCode(context.Context, string, string, string, time.Time, time.Time) error
	Issue(context.Context, OAuthGrant, OAuthTokenIssue) error
	RevokeSession(context.Context, string, time.Time) error
	RevokeClient(context.Context, string, string, time.Time) error
	RevokeToken(context.Context, string, string, string, time.Time) error
}

// NormalizeOIDCIssuer validates and canonicalizes the OIDC issuer URL (HTTPS, or HTTP on RFC 6761 loopback hosts).
func NormalizeOIDCIssuer(raw string) (string, bool, error) {
	issuer := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("OIDC issuer must be an absolute URL without query or fragment")
	}
	if parsed.Path != "" {
		return "", false, fmt.Errorf("OIDC issuer path is unsupported")
	}
	if parsed.Scheme == "https" {
		return issuer, false, nil
	}
	if parsed.Scheme == "http" && security.IsLoopbackHost(parsed.Hostname()) {
		return issuer, true, nil
	}
	return "", false, fmt.Errorf("OIDC issuer must use HTTPS except on localhost")
}
