package oidc

import (
	"context"
	"crypto/subtle"
	"fmt"
	"slices"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclient"
	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

func (storage *OIDCStorage) GetClientByClientID(ctx context.Context, clientID string) (op.Client, error) {
	entity, err := storage.client.OAuthClient.Query().Where(oauthclient.IDEQ(clientID)).Only(ctx)
	if entmodel.IsNotFound(err) {
		return nil, storageNotFoundError{cause: fmt.Errorf("OAuth client %q not found", clientID)}
	}
	if err != nil {
		return nil, fmt.Errorf("query OAuth client: %w", err)
	}
	return &client{entity: entity}, nil
}
func (storage *OIDCStorage) AuthorizeClientIDSecret(ctx context.Context, clientID, clientSecret string) error {
	entity, err := storage.client.OAuthClient.Query().Where(oauthclient.IDEQ(clientID)).Only(ctx)
	if entmodel.IsNotFound(err) {
		return storageNotFoundError{cause: fmt.Errorf("OAuth client %q not found", clientID)}
	}
	if err != nil {
		return fmt.Errorf("query OAuth client secret: %w", err)
	}
	expectedHash := biz.HashOpaqueSecret(clientSecret)
	if entity.SecretHash == "" || subtle.ConstantTimeCompare([]byte(expectedHash), []byte(entity.SecretHash)) != 1 {
		return fmt.Errorf("OAuth client credentials rejected")
	}
	return nil
}

func (storage *OIDCStorage) SetUserinfoFromScopes(
	context.Context,
	*oidc.UserInfo,
	string,
	string,
	[]string,
) error {
	return nil
}

func (storage *OIDCStorage) SetUserinfoFromToken(
	ctx context.Context,
	userinfo *oidc.UserInfo,
	tokenID, subject, _ string,
) error {
	token, err := storage.activeAccessToken(ctx, tokenID, subject)
	if err != nil {
		return err
	}
	return storage.populateUserinfo(ctx, userinfo, token.Subject, token.ClientID, token.Scopes)
}

func (storage *OIDCStorage) SetIntrospectionFromToken(
	ctx context.Context,
	introspection *oidc.IntrospectionResponse,
	tokenID, subject, clientID string,
) error {
	token, err := storage.activeAccessToken(ctx, tokenID, subject)
	if err != nil {
		return err
	}
	if clientID == "" || token.ClientID != clientID {
		return storageNotFoundError{cause: fmt.Errorf("OAuth access token does not belong to introspecting client")}
	}
	session, err := storage.client.OAuthTokenSession.Get(ctx, token.TokenSessionID)
	if err != nil {
		return fmt.Errorf("query OAuth token session for introspection: %w", err)
	}
	userinfo := new(oidc.UserInfo)
	if err := storage.populateUserinfo(ctx, userinfo, token.Subject, token.ClientID, token.Scopes); err != nil {
		return err
	}
	introspection.Active = true
	introspection.Scope = slices.Clone(token.Scopes)
	introspection.ClientID = token.ClientID
	introspection.TokenType = string(oidc.BearerToken)
	introspection.Expiration = oidc.FromTime(token.ExpiresTime)
	introspection.IssuedAt = oidc.FromTime(token.IssuedTime)
	introspection.AuthTime = oidc.FromTime(session.AuthTime)
	introspection.Subject = token.Subject
	introspection.Audience = []string{token.ClientID}
	introspection.AuthenticationMethodsReferences = slices.Clone(session.Amr)
	introspection.JWTID = token.ID
	introspection.SetUserInfo(userinfo)
	return nil
}
func (storage *OIDCStorage) GetPrivateClaimsFromScopes(context.Context, string, string, []string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (storage *OIDCStorage) GetKeyByIDAndClientID(context.Context, string, string) (*jose.JSONWebKey, error) {
	return nil, storageNotFoundError{cause: fmt.Errorf("client signing key not found")}
}
func (storage *OIDCStorage) ValidateJWTProfileScopes(context.Context, string, []string) ([]string, error) {
	return nil, fmt.Errorf("JWT profile grant is unsupported")
}
func (storage *OIDCStorage) SetUserinfoFromRequest(
	ctx context.Context,
	userinfo *oidc.UserInfo,
	request op.IDTokenRequest,
	scopes []string,
) error {
	return storage.populateUserinfo(ctx, userinfo, request.GetSubject(), request.GetClientID(), scopes)
}
