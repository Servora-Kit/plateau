package oidc

import (
	"context"
	"slices"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type serviceTokenRequest struct {
	clientID          string
	scopes, audiences []string
}

func (request *serviceTokenRequest) GetSubject() string    { return request.clientID }
func (request *serviceTokenRequest) GetScopes() []string   { return slices.Clone(request.scopes) }
func (request *serviceTokenRequest) GetAudience() []string { return slices.Clone(request.audiences) }

func (storage *OIDCStorage) ClientCredentials(ctx context.Context, id, secret string) (op.Client, error) {
	if err := storage.AuthorizeClientIDSecret(ctx, id, secret); err != nil {
		return nil, err
	}
	return storage.GetClientByClientID(ctx, id)
}

func (storage *OIDCStorage) ClientCredentialsTokenRequest(ctx context.Context, id string, scopes []string) (op.TokenRequest, error) {
	application, err := storage.GetClientByClientID(ctx, id)
	if err != nil {
		return nil, err
	}
	registered := application.(*client).entity
	if !slices.Contains(registered.AllowedGrantTypes, string(oidc.GrantTypeClientCredentials)) || len(registered.Audiences) == 0 {
		return nil, oidc.ErrUnauthorizedClient()
	}
	for _, scope := range scopes {
		if !application.IsScopeAllowed(scope) {
			return nil, oidc.ErrInvalidScope()
		}
	}
	return &serviceTokenRequest{clientID: id, scopes: slices.Clone(scopes), audiences: slices.Clone(registered.Audiences)}, nil
}

func (storage *OIDCStorage) GetPrivateClaimsFromRequest(_ context.Context, request op.TokenRequest, _ []string) (map[string]any, error) {
	if _, ok := request.(*serviceTokenRequest); ok {
		return map[string]any{"token_use": "access", "actor_type": "service"}, nil
	}
	return map[string]any{}, nil
}

var _ op.ClientCredentialsStorage = (*OIDCStorage)(nil)
var _ op.CanGetPrivateClaimsFromRequest = (*OIDCStorage)(nil)
