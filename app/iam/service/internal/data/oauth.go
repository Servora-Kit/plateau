package data

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
)

type oauthRepository struct{ ent *entmodel.Client }

func NewOAuthRepository(data *Data) (biz.OAuthRepo, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("OAuth repository: client is nil")
	}
	return &oauthRepository{ent: data.ent}, nil
}

func (repo *oauthRepository) CompleteAuthorizationRequest(ctx context.Context, requestID, userID, loginID string, now time.Time) error {
	if requestID == "" || userID == "" || loginID == "" {
		return biz.ErrOAuthGrantInvalid
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrOAuthGrantInvalid
			}
			return err
		}
		if current.Status != biz.UserStatusActive || len(current.RequiredActions) > 0 {
			return biz.ErrOAuthGrantInvalid
		}
		if _, err := ensureVerifiedEmail(ctx, tx, userID); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrOAuthGrantInvalid
			}
			return err
		}
		login, err := tx.IAMLoginSession.Get(ctx, loginID)
		if entmodel.IsNotFound(err) {
			return biz.ErrOAuthGrantInvalid
		}
		if err != nil {
			return translateEntError(err)
		}
		if login.UserID != userID || login.RevokedTime != nil {
			return biz.ErrOAuthGrantInvalid
		}
		updated, err := tx.OIDCAuthorizationRequest.Update().
			Where(
				oidcauthorizationrequest.IDEQ(requestID),
				oidcauthorizationrequest.DoneEQ(false),
				oidcauthorizationrequest.ExpiresTimeGT(now),
			).
			SetSubject(userID).
			SetIamLoginSessionID(loginID).
			SetAuthTime(login.CreateTime.UTC()).
			SetDone(true).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if updated != 1 {
			return biz.ErrOAuthGrantInvalid
		}
		return nil
	})
}

func (repo *oauthRepository) SaveAuthorizationCode(ctx context.Context, requestID, codeID, codeHash string, now, expires time.Time) error {
	if requestID == "" || codeID == "" || codeHash == "" {
		return biz.ErrOAuthGrantInvalid
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	expires = expires.UTC()
	initialRequest, err := repo.ent.OIDCAuthorizationRequest.Query().
		Where(
			oidcauthorizationrequest.IDEQ(requestID),
			oidcauthorizationrequest.DoneEQ(true),
			oidcauthorizationrequest.ExpiresTimeGT(now),
		).
		Only(ctx)
	if entmodel.IsNotFound(err) {
		return biz.ErrOAuthGrantInvalid
	}
	if err != nil {
		return translateEntError(err)
	}
	if initialRequest.Subject == nil || *initialRequest.Subject == "" || initialRequest.IamLoginSessionID == nil || *initialRequest.IamLoginSessionID == "" {
		return biz.ErrOAuthGrantInvalid
	}
	subject := *initialRequest.Subject
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, subject)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrOAuthGrantInvalid
			}
			return err
		}
		if current.Status != biz.UserStatusActive || len(current.RequiredActions) > 0 {
			return biz.ErrOAuthGrantInvalid
		}
		if _, err := ensureVerifiedEmail(ctx, tx, subject); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrOAuthGrantInvalid
			}
			return err
		}
		request, err := tx.OIDCAuthorizationRequest.Get(ctx, requestID)
		if entmodel.IsNotFound(err) {
			return biz.ErrOAuthGrantInvalid
		}
		if err != nil {
			return translateEntError(err)
		}
		if !request.Done || !request.ExpiresTime.After(now) || request.Subject == nil || *request.Subject != subject || request.IamLoginSessionID == nil || *request.IamLoginSessionID == "" {
			return biz.ErrOAuthGrantInvalid
		}
		login, err := tx.IAMLoginSession.Get(ctx, *request.IamLoginSessionID)
		if entmodel.IsNotFound(err) {
			return biz.ErrOAuthGrantInvalid
		}
		if err != nil {
			return translateEntError(err)
		}
		if login.UserID != subject || login.RevokedTime != nil {
			return biz.ErrOAuthGrantInvalid
		}
		_, err = tx.OAuthAuthorizationCode.Create().
			SetID(codeID).
			SetAuthorizationRequestID(request.ID).
			SetCodeHash(codeHash).
			SetClientID(request.ClientID).
			SetSubject(subject).
			SetRedirectURI(request.RedirectURI).
			SetScopes(slices.Clone(request.Scopes)).
			SetPkceChallenge(request.PkceChallenge).
			SetPkceChallengeMethod(request.PkceChallengeMethod).
			SetExpiresTime(expires).
			Save(ctx)
		return translateEntError(err)
	})
}

func (repo *oauthRepository) Issue(ctx context.Context, grant biz.OAuthGrant, issue biz.OAuthTokenIssue) error {
	var replay bool
	err := inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if grant.Service {
			return issueServiceToken(ctx, tx, grant, issue)
		}
		current, err := lockUser(ctx, tx, grant.Subject)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrOAuthGrantInvalid
			}
			return err
		}
		if current.Status != biz.UserStatusActive || len(current.RequiredActions) > 0 {
			return biz.ErrOAuthGrantInvalid
		}
		if _, err := ensureVerifiedEmail(ctx, tx, grant.Subject); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrOAuthGrantInvalid
			}
			return err
		}
		loginID := grant.LoginID
		sessionID, familyID, parentID := grant.TokenSessionID, "", ""
		if sessionID != "" {
			session, err := tx.OAuthTokenSession.Get(ctx, sessionID)
			if err != nil {
				return err
			}
			if session.UserID != grant.Subject || session.ClientID != grant.ClientID || session.RevokedTime != nil {
				return biz.ErrOAuthGrantInvalid
			}
			loginID, familyID = session.IamLoginSessionID, session.RefreshFamilyID
			token, err := tx.OAuthRefreshToken.Get(ctx, grant.RefreshTokenID)
			if err != nil {
				return err
			}
			if token.TokenSessionID != sessionID {
				return biz.ErrOAuthGrantInvalid
			}
			if token.ConsumedTime != nil {
				replay = true
				return revokeOAuthSessions(ctx, tx, issue.IssuedAt, oauthtokensession.IDEQ(sessionID))
			}
			if token.RevokedTime != nil || !token.ExpiresTime.After(issue.IssuedAt) {
				return biz.ErrOAuthGrantInvalid
			}
			for _, scope := range grant.Scopes {
				if !slices.Contains(session.Scopes, scope) {
					return biz.ErrOAuthGrantInvalid
				}
			}
			parentID = token.ID
			if _, err := tx.OAuthRefreshToken.UpdateOneID(token.ID).SetConsumedTime(issue.IssuedAt).Save(ctx); err != nil {
				return err
			}
			if _, err := tx.OAuthTokenSession.UpdateOneID(sessionID).SetScopes(grant.Scopes).Save(ctx); err != nil {
				return err
			}
		} else {
			code, err := tx.OAuthAuthorizationCode.Get(ctx, grant.CodeID)
			if err != nil {
				return err
			}
			if code.ConsumedTime != nil || !code.ExpiresTime.After(issue.IssuedAt) || code.Subject != grant.Subject || code.ClientID != grant.ClientID {
				return biz.ErrOAuthGrantInvalid
			}
			request, err := tx.OIDCAuthorizationRequest.Get(ctx, code.AuthorizationRequestID)
			if err != nil {
				return err
			}
			if !request.Done || request.IamLoginSessionID == nil || *request.IamLoginSessionID != loginID {
				return biz.ErrOAuthGrantInvalid
			}
			sessionID, err = biz.NewUserID()
			if err != nil {
				return err
			}
			familyID, err = biz.NewUserID()
			if err != nil {
				return err
			}
			if _, err := tx.OAuthAuthorizationCode.UpdateOneID(code.ID).Where(oauthauthorizationcode.ConsumedTimeIsNil()).SetConsumedTime(issue.IssuedAt).SetTokenSessionID(sessionID).Save(ctx); err != nil {
				return err
			}
		}
		login, err := tx.IAMLoginSession.Get(ctx, loginID)
		if err != nil {
			return err
		}
		if login.RevokedTime != nil || login.UserID != grant.Subject {
			return biz.ErrOAuthGrantInvalid
		}
		if grant.TokenSessionID == "" {
			_, err := tx.OAuthTokenSession.Create().SetID(sessionID).SetUserID(grant.Subject).SetClientID(grant.ClientID).
				SetIamLoginSessionID(loginID).SetRefreshFamilyID(familyID).SetScopes(grant.Scopes).SetAuthTime(login.CreateTime).SetAmr(grant.AMR).Save(ctx)
			if err != nil {
				return err
			}
		}
		_, err = tx.OAuthAccessToken.Create().SetID(issue.AccessID).SetTokenSessionID(sessionID).
			SetSubject(grant.Subject).SetClientID(grant.ClientID).SetScopes(grant.Scopes).SetAudiences(grant.Audiences).
			SetIssuedTime(issue.IssuedAt).SetExpiresTime(issue.AccessExpiresAt).Save(ctx)
		if err != nil {
			return err
		}
		if issue.RefreshID != "" {
			builder := tx.OAuthRefreshToken.Create().SetID(issue.RefreshID).SetTokenSessionID(sessionID).SetFamilyID(familyID).
				SetTokenHash(issue.RefreshHash).SetIssuedTime(issue.IssuedAt).SetExpiresTime(issue.RefreshExpiresAt)
			if parentID != "" {
				builder.SetParentTokenID(parentID)
			}
			_, err = builder.Save(ctx)
		}
		return err
	})
	if replay || entmodel.IsNotFound(err) {
		return biz.ErrOAuthGrantInvalid
	}
	return translateEntError(err)
}

func issueServiceToken(ctx context.Context, tx *entmodel.Tx, grant biz.OAuthGrant, issue biz.OAuthTokenIssue) error {
	client, err := tx.OAuthClient.Get(ctx, grant.ClientID)
	if err != nil {
		return err
	}
	if grant.Subject != client.ID || issue.RefreshID != "" || !slices.Contains(client.AllowedGrantTypes, "client_credentials") || len(client.Audiences) == 0 || !slices.Equal(client.Audiences, grant.Audiences) {
		return biz.ErrOAuthGrantInvalid
	}
	_, err = tx.OAuthAccessToken.Create().SetID(issue.AccessID).SetActorType(oauthaccesstoken.ActorTypeService).
		SetSubject(client.ID).SetClientID(client.ID).SetScopes(grant.Scopes).SetAudiences(client.Audiences).
		SetIssuedTime(issue.IssuedAt).SetExpiresTime(issue.AccessExpiresAt).Save(ctx)
	return err
}

func (repo *oauthRepository) RevokeSession(ctx context.Context, sessionID string, now time.Time) error {
	session, err := repo.ent.OAuthTokenSession.Get(ctx, sessionID)
	if entmodel.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if _, err := lockUser(ctx, tx, session.UserID); err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return nil
			}
			return err
		}
		return revokeOAuthSessions(ctx, tx, now, oauthtokensession.IDEQ(sessionID))
	})
}

func (repo *oauthRepository) RevokeClient(ctx context.Context, userID, clientID string, now time.Time) error {
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return nil
			}
			return err
		}
		return revokeOAuthSessions(ctx, tx, now, oauthtokensession.UserIDEQ(userID), oauthtokensession.ClientIDEQ(clientID))
	})
}

func (repo *oauthRepository) RevokeToken(ctx context.Context, tokenID, subject, clientID string, now time.Time) error {
	access, err := repo.ent.OAuthAccessToken.Query().Where(oauthaccesstoken.IDEQ(tokenID), oauthaccesstoken.SubjectEQ(subject), oauthaccesstoken.ClientIDEQ(clientID)).Only(ctx)
	if err == nil {
		if access.ActorType == oauthaccesstoken.ActorTypeService {
			_, err := repo.ent.OAuthAccessToken.UpdateOneID(access.ID).SetRevokedTime(now).Save(ctx)
			return err
		}
		return repo.RevokeSession(ctx, access.TokenSessionID, now)
	}
	if !entmodel.IsNotFound(err) {
		return err
	}
	token, err := repo.ent.OAuthRefreshToken.Query().Where(oauthrefreshtoken.IDEQ(tokenID)).Only(ctx)
	if entmodel.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	session, err := repo.ent.OAuthTokenSession.Get(ctx, token.TokenSessionID)
	if err != nil {
		return err
	}
	if session.ClientID != clientID || session.UserID != subject {
		return nil
	}
	return repo.RevokeSession(ctx, session.ID, now)
}
