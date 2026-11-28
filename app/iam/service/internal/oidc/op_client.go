package oidc

import (
	"net/url"
	"slices"
	"time"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/security"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type client struct {
	entity *entmodel.OAuthClient
}

func (client *client) GetID() string { return client.entity.ID }

func (client *client) RedirectURIs() []string { return slices.Clone(client.entity.RedirectUris) }

func (client *client) PostLogoutRedirectURIs() []string {
	return slices.Clone(client.entity.PostLogoutRedirectUris)
}

func (client *client) ApplicationType() op.ApplicationType {
	switch client.entity.ApplicationType {
	case biz.AppTypeUserAgent:
		return op.ApplicationTypeUserAgent
	case biz.AppTypeNative:
		return op.ApplicationTypeNative
	default:
		return op.ApplicationTypeWeb
	}
}

func (client *client) AuthMethod() oidc.AuthMethod {
	switch client.entity.TokenEndpointAuthMethod {
	case biz.AuthMethodPost:
		return oidc.AuthMethodPost
	case biz.AuthMethodNone:
		return oidc.AuthMethodNone
	default:
		return oidc.AuthMethodBasic
	}
}

func (client *client) ResponseTypes() []oidc.ResponseType {
	result := make([]oidc.ResponseType, len(client.entity.AllowedResponseTypes))
	for index, value := range client.entity.AllowedResponseTypes {
		result[index] = oidc.ResponseType(value)
	}
	return result
}

func (client *client) GrantTypes() []oidc.GrantType {
	result := make([]oidc.GrantType, len(client.entity.AllowedGrantTypes))
	for index, value := range client.entity.AllowedGrantTypes {
		result[index] = oidc.GrantType(value)
	}
	return result
}

func (client *client) LoginURL(requestID string) string {
	return "/authorize/callback?id=" + url.QueryEscape(requestID)
}

func (client *client) AccessTokenType() op.AccessTokenType { return op.AccessTokenTypeJWT }

func (client *client) IDTokenLifetime() time.Duration { return idTokenTTL }

func (client *client) DevMode() bool {
	hasHTTP := false
	for _, raw := range slices.Concat(client.entity.RedirectUris, client.entity.PostLogoutRedirectUris) {
		parsed, err := url.Parse(raw)
		if err != nil {
			return false
		}
		if parsed.Scheme == "http" {
			if !security.IsLoopbackHost(parsed.Hostname()) {
				return false
			}
			hasHTTP = true
		}
	}
	return hasHTTP
}

func (client *client) RestrictAdditionalIdTokenScopes() func([]string) []string {
	return func(scopes []string) []string { return slices.Clone(scopes) }
}

func (client *client) RestrictAdditionalAccessTokenScopes() func([]string) []string {
	return func(scopes []string) []string { return slices.Clone(scopes) }
}

func (client *client) IsScopeAllowed(scope string) bool {
	return slices.Contains(client.entity.AllowedScopes, scope)
}

func (client *client) IDTokenUserinfoClaimsAssertion() bool { return false }

func (client *client) ClockSkew() time.Duration { return 0 }
