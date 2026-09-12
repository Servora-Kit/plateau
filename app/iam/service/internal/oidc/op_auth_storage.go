package oidc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthauthorizationcode"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthrefreshtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcauthorizationrequest"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

func tokenGrantFromRequest(request op.TokenRequest) (biz.OAuthGrant, error) {
	switch typed := request.(type) {
	case *authorizationRequest:
		if typed.authorizationCodeID == "" || !typed.Done() || typed.entity.IamLoginSessionID == nil {
			return biz.OAuthGrant{}, biz.ErrOAuthGrantInvalid
		}
		return biz.OAuthGrant{CodeID: typed.authorizationCodeID, Subject: typed.GetSubject(), ClientID: typed.GetClientID(),
			LoginID: *typed.entity.IamLoginSessionID, Scopes: typed.GetScopes(), Audiences: typed.GetAudience(), AuthTime: typed.GetAuthTime(), AMR: typed.GetAMR()}, nil
	case *refreshTokenRequest:
		return biz.OAuthGrant{RefreshTokenID: typed.tokenID, TokenSessionID: typed.tokenSessionID, Subject: typed.subject,
			ClientID: typed.clientID, Scopes: typed.GetScopes(), Audiences: typed.GetAudience(), AuthTime: typed.authTime, AMR: typed.GetAMR()}, nil
	case *serviceTokenRequest:
		return biz.OAuthGrant{Service: true, Subject: typed.clientID, ClientID: typed.clientID, Scopes: typed.GetScopes(), Audiences: typed.GetAudience()}, nil
	default:
		return biz.OAuthGrant{}, fmt.Errorf("unsupported token request type %T", request)
	}
}

// Authorization requests and authorization codes.
func (storage *OIDCStorage) CreateAuthRequest(
	ctx context.Context,
	request *oidc.AuthRequest,
	_ string,
) (op.AuthRequest, error) {
	if err := validateAuthorizationRequest(request); err != nil {
		return nil, err
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := storage.now().UTC()
	builder := storage.client.OIDCAuthorizationRequest.Create().
		SetID(id).
		SetClientID(request.ClientID).
		SetRedirectURI(request.RedirectURI).
		SetResponseType(string(request.ResponseType)).
		SetResponseMode(string(request.ResponseMode)).
		SetScopes(slices.Clone(request.Scopes)).
		SetPkceChallenge(request.CodeChallenge).
		SetPkceChallengeMethod(string(request.CodeChallengeMethod)).
		SetExpiresTime(now.Add(authorizationRequestTTL))
	if request.State != "" {
		builder.SetState(request.State)
	}
	if request.Nonce != "" {
		builder.SetNonce(request.Nonce)
	}
	entity, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OIDC authorization request: %w", err)
	}
	return &authorizationRequest{entity: entity}, nil
}
func (storage *OIDCStorage) AuthRequestByID(ctx context.Context, requestID string) (op.AuthRequest, error) {
	entity, err := storage.client.OIDCAuthorizationRequest.Query().
		Where(
			oidcauthorizationrequest.IDEQ(requestID),
			oidcauthorizationrequest.ExpiresTimeGT(storage.now().UTC()),
		).
		Only(ctx)
	if entmodel.IsNotFound(err) {
		return nil, storageNotFoundError{cause: fmt.Errorf("OIDC authorization request not found")}
	}
	if err != nil {
		return nil, fmt.Errorf("query OIDC authorization request: %w", err)
	}
	return &authorizationRequest{entity: entity}, nil
}

// CompleteAuthRequest binds a verified IAM browser session to a pending OAuth request.
func (storage *OIDCStorage) CompleteAuthRequest(ctx context.Context, requestID, userID, iamSessionID string, authTime time.Time) error {
	updated, err := storage.client.OIDCAuthorizationRequest.Update().
		Where(
			oidcauthorizationrequest.IDEQ(requestID),
			oidcauthorizationrequest.DoneEQ(false),
			oidcauthorizationrequest.ExpiresTimeGT(storage.now().UTC()),
		).
		SetSubject(userID).
		SetIamLoginSessionID(iamSessionID).
		SetAuthTime(authTime.UTC()).
		SetDone(true).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("complete OIDC authorization request: %w", err)
	}
	if updated != 1 {
		return storageNotFoundError{cause: fmt.Errorf("OIDC authorization request is missing, expired, or already completed")}
	}
	return nil
}
func (storage *OIDCStorage) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	codeEntity, err := storage.client.OAuthAuthorizationCode.Query().
		Where(
			oauthauthorizationcode.CodeHashEQ(biz.HashOpaqueSecret(code)),
			oauthauthorizationcode.ConsumedTimeIsNil(),
			oauthauthorizationcode.ExpiresTimeGT(storage.now().UTC()),
		).
		Only(ctx)
	if entmodel.IsNotFound(err) {
		return nil, storageNotFoundError{cause: fmt.Errorf("OAuth authorization code not found")}
	}
	if err != nil {
		return nil, fmt.Errorf("query OAuth authorization code: %w", err)
	}
	requestEntity, err := storage.client.OIDCAuthorizationRequest.Get(ctx, codeEntity.AuthorizationRequestID)
	if entmodel.IsNotFound(err) {
		return nil, storageNotFoundError{cause: fmt.Errorf("OIDC authorization request for code not found")}
	}
	if err != nil {
		return nil, fmt.Errorf("query OIDC authorization request for code: %w", err)
	}
	if !requestEntity.Done || requestEntity.Subject == nil || requestEntity.AuthTime == nil {
		return nil, storageNotFoundError{cause: fmt.Errorf("OIDC authorization request is incomplete")}
	}
	return &authorizationRequest{entity: requestEntity, authorizationCodeID: codeEntity.ID}, nil
}
func (storage *OIDCStorage) SaveAuthCode(ctx context.Context, requestID, code string) error {
	request, err := storage.client.OIDCAuthorizationRequest.Query().
		Where(
			oidcauthorizationrequest.IDEQ(requestID),
			oidcauthorizationrequest.DoneEQ(true),
			oidcauthorizationrequest.ExpiresTimeGT(storage.now().UTC()),
		).
		Only(ctx)
	if entmodel.IsNotFound(err) {
		return storageNotFoundError{cause: fmt.Errorf("completed OIDC authorization request not found")}
	}
	if err != nil {
		return fmt.Errorf("query completed OIDC authorization request: %w", err)
	}
	id, err := newID()
	if err != nil {
		return err
	}
	if _, err := storage.client.OAuthAuthorizationCode.Create().
		SetID(id).
		SetAuthorizationRequestID(request.ID).
		SetCodeHash(biz.HashOpaqueSecret(code)).
		SetClientID(request.ClientID).
		SetSubject(*request.Subject).
		SetRedirectURI(request.RedirectURI).
		SetScopes(slices.Clone(request.Scopes)).
		SetPkceChallenge(request.PkceChallenge).
		SetPkceChallengeMethod(request.PkceChallengeMethod).
		SetExpiresTime(storage.now().UTC().Add(authorizationCodeTTL)).
		Save(ctx); err != nil {
		return fmt.Errorf("save OAuth authorization code: %w", err)
	}
	return nil
}
func (storage *OIDCStorage) DeleteAuthRequest(ctx context.Context, requestID string) error {
	if _, err := storage.client.OIDCAuthorizationRequest.Delete().
		Where(oidcauthorizationrequest.IDEQ(requestID)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete OIDC authorization request: %w", err)
	}
	return nil
}

// Token persistence is owned by the biz repository and implemented in data.
func (storage *OIDCStorage) CreateAccessToken(ctx context.Context, request op.TokenRequest) (string, time.Time, error) {
	grant, err := tokenGrantFromRequest(request)
	if err != nil {
		return "", time.Time{}, tokenGrantError(err)
	}
	id, err := newID()
	if err != nil {
		return "", time.Time{}, err
	}
	now, ttl := storage.now().UTC(), accessTokenTTL
	if grant.Service {
		ttl = storage.serviceAccessTokenTTL
	}
	expires := now.Add(ttl)
	err = storage.tokens.Issue(ctx, grant, biz.OAuthTokenIssue{AccessID: id, IssuedAt: now, AccessExpiresAt: expires})
	if err != nil {
		return "", time.Time{}, tokenGrantError(err)
	}
	return id, expires, nil
}

func (storage *OIDCStorage) CreateAccessAndRefreshTokens(ctx context.Context, request op.TokenRequest, _ string) (string, string, time.Time, error) {
	grant, err := tokenGrantFromRequest(request)
	if err != nil || grant.Service {
		return "", "", time.Time{}, oidc.ErrInvalidGrant().WithParent(err)
	}
	accessID, err := newID()
	if err != nil {
		return "", "", time.Time{}, err
	}
	refreshID, err := newID()
	if err != nil {
		return "", "", time.Time{}, err
	}
	refresh, hash, err := biz.NewOpaqueSecret()
	if err != nil {
		return "", "", time.Time{}, err
	}
	now := storage.now().UTC()
	expires := now.Add(accessTokenTTL)
	err = storage.tokens.Issue(ctx, grant, biz.OAuthTokenIssue{
		AccessID: accessID, RefreshID: refreshID, RefreshHash: hash, IssuedAt: now,
		AccessExpiresAt: expires, RefreshExpiresAt: now.Add(refreshTokenTTL),
	})
	if err != nil {
		return "", "", time.Time{}, tokenGrantError(err)
	}
	return accessID, refresh, expires, nil
}

func (storage *OIDCStorage) TokenRequestByRefreshToken(ctx context.Context, refresh string) (op.RefreshTokenRequest, error) {
	token, err := storage.client.OAuthRefreshToken.Query().Where(oauthrefreshtoken.TokenHashEQ(biz.HashOpaqueSecret(refresh))).Only(ctx)
	if err != nil {
		return nil, tokenGrantError(err)
	}
	session, err := storage.client.OAuthTokenSession.Get(ctx, token.TokenSessionID)
	if err != nil {
		return nil, tokenGrantError(err)
	}
	if token.ConsumedTime != nil {
		if err := storage.tokens.RevokeSession(ctx, session.ID, storage.now()); err != nil {
			return nil, err
		}
		return nil, oidc.ErrInvalidGrant()
	}
	if token.RevokedTime != nil || session.RevokedTime != nil || !token.ExpiresTime.After(storage.now()) {
		return nil, oidc.ErrInvalidGrant()
	}
	return &refreshTokenRequest{tokenID: token.ID, tokenSessionID: session.ID, clientID: session.ClientID,
		subject: session.UserID, scopes: slices.Clone(session.Scopes), authTime: session.AuthTime, amr: slices.Clone(session.Amr)}, nil
}

func (storage *OIDCStorage) TerminateSession(ctx context.Context, userID, clientID string) error {
	return storage.tokens.RevokeClient(ctx, userID, clientID, storage.now())
}

func (storage *OIDCStorage) RevokeToken(ctx context.Context, tokenID, subject, clientID string) *oidc.Error {
	if err := storage.tokens.RevokeToken(ctx, tokenID, subject, clientID, storage.now()); err != nil {
		return oidc.ErrServerError().WithParent(err)
	}
	return nil
}

func (storage *OIDCStorage) GetRefreshTokenInfo(ctx context.Context, clientID, refreshToken string) (string, string, error) {
	token, err := storage.client.OAuthRefreshToken.Query().
		Where(oauthrefreshtoken.TokenHashEQ(biz.HashOpaqueSecret(refreshToken))).
		Only(ctx)
	if entmodel.IsNotFound(err) {
		return "", "", op.ErrInvalidRefreshToken
	}
	if err != nil {
		return "", "", fmt.Errorf("query OAuth refresh token info: %w", err)
	}
	session, err := storage.client.OAuthTokenSession.Get(ctx, token.TokenSessionID)
	if err != nil || session.ClientID != clientID {
		return "", "", op.ErrInvalidRefreshToken
	}
	return session.UserID, token.ID, nil
}

func tokenGrantError(err error) error {
	var notFound storageNotFoundError
	if errors.Is(err, biz.ErrOAuthGrantInvalid) || entmodel.IsNotFound(err) || errors.As(err, &notFound) {
		return oidc.ErrInvalidGrant().WithParent(err)
	}
	return err
}
