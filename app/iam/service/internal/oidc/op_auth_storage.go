package oidc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthauthorizationcode"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthrefreshtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcauthorizationrequest"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

// tokenGrant translates ZITADEL token requests into IAM token-session state.
type tokenGrant struct {
	codeID            string
	refreshTokenID    string
	tokenSessionID    string
	userID            string
	clientID          string
	iamLoginSessionID *string
	scopes            []string
	authTime          time.Time
	amr               []string
}

func tokenGrantFromRequest(request op.TokenRequest) (tokenGrant, error) {
	switch typed := request.(type) {
	case *authorizationRequest:
		if typed.authorizationCodeID == "" || !typed.Done() || typed.entity.IamLoginSessionID == nil {
			return tokenGrant{}, fmt.Errorf("authorization request is not ready for token issuance")
		}
		return tokenGrant{
			codeID: typed.authorizationCodeID, userID: typed.GetSubject(), clientID: typed.GetClientID(),
			iamLoginSessionID: typed.entity.IamLoginSessionID, scopes: typed.GetScopes(), authTime: typed.GetAuthTime(), amr: typed.GetAMR(),
		}, nil
	case *refreshTokenRequest:
		return tokenGrant{
			refreshTokenID: typed.tokenID, tokenSessionID: typed.tokenSessionID, userID: typed.subject,
			clientID: typed.clientID, scopes: typed.GetScopes(), authTime: typed.authTime, amr: typed.GetAMR(),
		}, nil
	default:
		return tokenGrant{}, fmt.Errorf("unsupported token request type %T", request)
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

// Access tokens and refresh-token rotation.
func (storage *OIDCStorage) CreateAccessToken(
	ctx context.Context,
	request op.TokenRequest,
) (string, time.Time, error) {
	grant, err := tokenGrantFromRequest(request)
	if err != nil {
		return "", time.Time{}, err
	}
	accessTokenID, err := newID()
	if err != nil {
		return "", time.Time{}, err
	}
	tokenSessionID, err := newID()
	if err != nil {
		return "", time.Time{}, err
	}
	familyID, err := newID()
	if err != nil {
		return "", time.Time{}, err
	}
	now := storage.now().UTC()
	expires := now.Add(accessTokenTTL)
	err = storage.inTx(ctx, func(tx *entmodel.Tx) error {
		if err := storage.consumeAuthorizationCode(ctx, tx, grant.codeID, tokenSessionID, now); err != nil {
			return err
		}
		if err := ensureActiveUser(ctx, tx, grant.userID); err != nil {
			return err
		}
		sessionBuilder := tx.OAuthTokenSession.Create().
			SetID(tokenSessionID).
			SetUserID(grant.userID).
			SetClientID(grant.clientID).
			SetRefreshFamilyID(familyID).
			SetScopes(grant.scopes).
			SetAuthTime(grant.authTime.UTC()).
			SetAmr(grant.amr)
		if grant.iamLoginSessionID != nil {
			sessionBuilder.SetIamLoginSessionID(*grant.iamLoginSessionID)
		}
		if _, err := sessionBuilder.Save(ctx); err != nil {
			return fmt.Errorf("create OAuth token session: %w", err)
		}
		if _, err := tx.OAuthAccessToken.Create().
			SetID(accessTokenID).
			SetTokenSessionID(tokenSessionID).
			SetClientID(grant.clientID).
			SetSubject(grant.userID).
			SetScopes(grant.scopes).
			SetIssuedTime(now).
			SetExpiresTime(expires).
			Save(ctx); err != nil {
			return fmt.Errorf("create OAuth access token: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", time.Time{}, tokenGrantError(err)
	}
	return accessTokenID, expires, nil
}

func (storage *OIDCStorage) CreateAccessAndRefreshTokens(
	ctx context.Context,
	request op.TokenRequest,
	_ string,
) (string, string, time.Time, error) {
	grant, err := tokenGrantFromRequest(request)
	if err != nil {
		return "", "", time.Time{}, err
	}
	accessTokenID, err := newID()
	if err != nil {
		return "", "", time.Time{}, err
	}
	refreshTokenID, err := newID()
	if err != nil {
		return "", "", time.Time{}, err
	}
	refreshToken, _, err := biz.NewOpaqueSecret()
	if err != nil {
		return "", "", time.Time{}, err
	}
	now := storage.now().UTC()
	accessExpires := now.Add(accessTokenTTL)
	refreshExpires := now.Add(refreshTokenTTL)
	replayDetected := false
	err = storage.inTx(ctx, func(tx *entmodel.Tx) error {
		tokenSessionID := grant.tokenSessionID
		var refreshFamilyID string
		var parentRefreshTokenID *string
		if tokenSessionID == "" {
			tokenSessionID, err = newID()
			if err != nil {
				return err
			}
			refreshFamilyID, err = newID()
			if err != nil {
				return err
			}
			if err := storage.consumeAuthorizationCode(ctx, tx, grant.codeID, tokenSessionID, now); err != nil {
				return err
			}
			if err := ensureActiveUser(ctx, tx, grant.userID); err != nil {
				return err
			}
			sessionBuilder := tx.OAuthTokenSession.Create().
				SetID(tokenSessionID).
				SetUserID(grant.userID).
				SetClientID(grant.clientID).
				SetRefreshFamilyID(refreshFamilyID).
				SetScopes(grant.scopes).
				SetAuthTime(grant.authTime.UTC()).
				SetAmr(grant.amr)
			if grant.iamLoginSessionID != nil {
				sessionBuilder.SetIamLoginSessionID(*grant.iamLoginSessionID)
			}
			if _, err := sessionBuilder.Save(ctx); err != nil {
				return fmt.Errorf("create OAuth token session: %w", err)
			}
		} else {
			current, queryErr := tx.OAuthRefreshToken.Query().
				Where(oauthrefreshtoken.IDEQ(grant.refreshTokenID)).
				Only(ctx)
			if queryErr != nil {
				return fmt.Errorf("query OAuth refresh token: %w", queryErr)
			}
			if current.ConsumedTime != nil {
				if err := revokeTokenSession(ctx, tx, tokenSessionID, now); err != nil {
					return err
				}
				replayDetected = true
				return nil
			}
			if current.RevokedTime != nil || !current.ExpiresTime.After(now) {
				return storageNotFoundError{cause: fmt.Errorf("refresh token is inactive")}
			}
			session, queryErr := tx.OAuthTokenSession.Query().
				Where(oauthtokensession.IDEQ(tokenSessionID)).
				Only(ctx)
			if queryErr != nil {
				return fmt.Errorf("query OAuth token session: %w", queryErr)
			}
			refreshFamilyID = session.RefreshFamilyID
			parentRefreshTokenID = &current.ID
			if session.RevokedTime != nil {
				return storageNotFoundError{cause: fmt.Errorf("OAuth token session is revoked")}
			}
			if err := ensureActiveUser(ctx, tx, grant.userID); err != nil {
				return err
			}
			activeSessions, updateErr := tx.OAuthTokenSession.Update().
				Where(oauthtokensession.IDEQ(tokenSessionID), oauthtokensession.RevokedTimeIsNil()).
				SetScopes(grant.scopes).
				SetUpdateTime(now).
				Save(ctx)
			if updateErr != nil {
				return fmt.Errorf("validate OAuth token session: %w", updateErr)
			}
			if activeSessions != 1 {
				return storageNotFoundError{cause: fmt.Errorf("OAuth token session is revoked")}
			}
			consumed, updateErr := tx.OAuthRefreshToken.Update().
				Where(
					oauthrefreshtoken.IDEQ(current.ID),
					oauthrefreshtoken.ConsumedTimeIsNil(),
					oauthrefreshtoken.RevokedTimeIsNil(),
					oauthrefreshtoken.ExpiresTimeGT(now),
				).
				SetConsumedTime(now).
				Save(ctx)
			if updateErr != nil {
				return fmt.Errorf("consume OAuth refresh token: %w", updateErr)
			}
			if consumed != 1 {
				if err := revokeTokenSession(ctx, tx, tokenSessionID, now); err != nil {
					return err
				}
				replayDetected = true
				return nil
			}
		}
		if replayDetected {
			return nil
		}
		if _, err := tx.OAuthAccessToken.Create().
			SetID(accessTokenID).
			SetTokenSessionID(tokenSessionID).
			SetClientID(grant.clientID).
			SetSubject(grant.userID).
			SetScopes(grant.scopes).
			SetIssuedTime(now).
			SetExpiresTime(accessExpires).
			Save(ctx); err != nil {
			return fmt.Errorf("create OAuth access token: %w", err)
		}
		refreshBuilder := tx.OAuthRefreshToken.Create().
			SetID(refreshTokenID).
			SetTokenSessionID(tokenSessionID).
			SetFamilyID(refreshFamilyID).
			SetTokenHash(biz.HashOpaqueSecret(refreshToken)).
			SetIssuedTime(now).
			SetExpiresTime(refreshExpires)
		if parentRefreshTokenID != nil {
			refreshBuilder.SetParentTokenID(*parentRefreshTokenID)
		}
		if _, err := refreshBuilder.Save(ctx); err != nil {
			return fmt.Errorf("create OAuth refresh token: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", "", time.Time{}, tokenGrantError(err)
	}
	if replayDetected {
		return "", "", time.Time{}, oidc.ErrInvalidGrant().WithDescription("refresh token replay detected")
	}
	return accessTokenID, refreshToken, accessExpires, nil
}

// Refresh-token lookup, session termination, and revocation.
func (storage *OIDCStorage) TokenRequestByRefreshToken(ctx context.Context, refreshToken string) (op.RefreshTokenRequest, error) {
	var request *refreshTokenRequest
	replayDetected := false
	now := storage.now().UTC()
	err := storage.inTx(ctx, func(tx *entmodel.Tx) error {
		token, err := tx.OAuthRefreshToken.Query().Where(oauthrefreshtoken.TokenHashEQ(biz.HashOpaqueSecret(refreshToken))).Only(ctx)
		if entmodel.IsNotFound(err) {
			return storageNotFoundError{cause: fmt.Errorf("OAuth refresh token not found")}
		}
		if err != nil {
			return fmt.Errorf("query OAuth refresh token: %w", err)
		}
		session, err := tx.OAuthTokenSession.Query().Where(oauthtokensession.IDEQ(token.TokenSessionID)).Only(ctx)
		if err != nil {
			return fmt.Errorf("query OAuth token session: %w", err)
		}
		if token.ConsumedTime != nil {
			if err := revokeTokenSession(ctx, tx, session.ID, now); err != nil {
				return err
			}
			replayDetected = true
			return nil
		}
		if token.RevokedTime != nil || !token.ExpiresTime.After(now) || session.RevokedTime != nil {
			return storageNotFoundError{cause: fmt.Errorf("OAuth refresh token is inactive")}
		}
		if err := ensureActiveUser(ctx, tx, session.UserID); err != nil {
			return storageNotFoundError{cause: err}
		}
		request = &refreshTokenRequest{
			tokenID:        token.ID,
			tokenSessionID: session.ID,
			clientID:       session.ClientID,
			subject:        session.UserID,
			scopes:         append([]string(nil), session.Scopes...),
			authTime:       session.AuthTime,
			amr:            append([]string(nil), session.Amr...),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if replayDetected {
		return nil, storageNotFoundError{cause: fmt.Errorf("OAuth refresh token replay detected")}
	}
	return request, nil
}

func (storage *OIDCStorage) TerminateSession(ctx context.Context, userID, clientID string) error {
	now := storage.now().UTC()
	return storage.inTx(ctx, func(tx *entmodel.Tx) error {
		sessions, err := tx.OAuthTokenSession.Query().
			Where(
				oauthtokensession.UserIDEQ(userID),
				oauthtokensession.ClientIDEQ(clientID),
				oauthtokensession.RevokedTimeIsNil(),
			).
			IDs(ctx)
		if err != nil {
			return fmt.Errorf("query OAuth token sessions for termination: %w", err)
		}
		for _, sessionID := range sessions {
			if err := revokeTokenSession(ctx, tx, sessionID, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (storage *OIDCStorage) RevokeToken(ctx context.Context, tokenID, userID, clientID string) *oidc.Error {
	now := storage.now().UTC()
	err := storage.inTx(ctx, func(tx *entmodel.Tx) error {
		sessionID, err := tokenSessionIDByToken(ctx, tx, tokenID, userID, clientID)
		if errors.Is(err, errTokenNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return revokeTokenSession(ctx, tx, sessionID, now)
	})
	if err != nil {
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

func (storage *OIDCStorage) consumeAuthorizationCode(ctx context.Context, tx *entmodel.Tx, codeID, tokenSessionID string, now time.Time) error {
	consumed, err := tx.OAuthAuthorizationCode.Update().
		Where(
			oauthauthorizationcode.IDEQ(codeID),
			oauthauthorizationcode.ConsumedTimeIsNil(),
			oauthauthorizationcode.ExpiresTimeGT(now),
		).
		SetTokenSessionID(tokenSessionID).
		SetConsumedTime(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("consume OAuth authorization code: %w", err)
	}
	if consumed != 1 {
		return storageNotFoundError{cause: fmt.Errorf("OAuth authorization code is inactive")}
	}
	return nil
}

func ensureActiveUser(ctx context.Context, tx *entmodel.Tx, userID string) error {
	exists, err := tx.User.Query().Where(user.IDEQ(userID), user.StatusEQ(biz.UserStatusActive)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check token subject: %w", err)
	}
	if !exists {
		return errTokenSubjectInactive
	}
	return nil
}

var errTokenSubjectInactive = errors.New("token subject is not active")

func tokenGrantError(err error) error {
	var notFound storageNotFoundError
	if errors.As(err, &notFound) || errors.Is(err, errTokenSubjectInactive) {
		return oidc.ErrInvalidGrant().WithParent(err)
	}
	return err
}

func revokeTokenSession(ctx context.Context, tx *entmodel.Tx, sessionID string, now time.Time) error {
	if _, err := tx.OAuthTokenSession.Update().
		Where(oauthtokensession.IDEQ(sessionID), oauthtokensession.RevokedTimeIsNil()).
		SetRevokedTime(now).
		Save(ctx); err != nil {
		return fmt.Errorf("revoke OAuth token session: %w", err)
	}
	if _, err := tx.OAuthAccessToken.Update().
		Where(oauthaccesstoken.TokenSessionIDEQ(sessionID), oauthaccesstoken.RevokedTimeIsNil()).
		SetRevokedTime(now).
		Save(ctx); err != nil {
		return fmt.Errorf("revoke OAuth access tokens: %w", err)
	}
	if _, err := tx.OAuthRefreshToken.Update().
		Where(oauthrefreshtoken.TokenSessionIDEQ(sessionID), oauthrefreshtoken.RevokedTimeIsNil()).
		SetRevokedTime(now).
		Save(ctx); err != nil {
		return fmt.Errorf("revoke OAuth refresh tokens: %w", err)
	}
	return nil
}

var errTokenNotFound = errors.New("OAuth token not found")

func tokenSessionIDByToken(ctx context.Context, tx *entmodel.Tx, tokenID, userID, clientID string) (string, error) {
	access, err := tx.OAuthAccessToken.Query().
		Where(
			oauthaccesstoken.IDEQ(tokenID),
			oauthaccesstoken.SubjectEQ(userID),
			oauthaccesstoken.ClientIDEQ(clientID),
		).
		Only(ctx)
	if err == nil {
		return access.TokenSessionID, nil
	}
	if !entmodel.IsNotFound(err) {
		return "", fmt.Errorf("query OAuth access token for revocation: %w", err)
	}
	refresh, err := tx.OAuthRefreshToken.Query().
		Where(oauthrefreshtoken.IDEQ(tokenID)).
		Only(ctx)
	if entmodel.IsNotFound(err) {
		return "", errTokenNotFound
	}
	if err != nil {
		return "", fmt.Errorf("query OAuth refresh token for revocation: %w", err)
	}
	session, err := tx.OAuthTokenSession.Get(ctx, refresh.TokenSessionID)
	if err != nil || session.UserID != userID || session.ClientID != clientID {
		return "", errTokenNotFound
	}
	return session.ID, nil
}
