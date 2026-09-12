package data

import (
	"context"
	"fmt"
	"time"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/iamloginsession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthrefreshtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/predicate"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
)

type credentialRepository struct{ ent *entmodel.Client }
type sessionRepository struct{ ent *entmodel.Client }

func NewCredentialRepository(data *Data) (biz.CredentialRepo, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("credential repository: data is nil")
	}
	return &credentialRepository{ent: data.ent}, nil
}
func NewSessionRepository(data *Data) (biz.SessionRepo, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("session repository: data is nil")
	}
	return &sessionRepository{ent: data.ent}, nil
}

// lockUser 为身份变更、登录与 OAuth 提交提供同一数据库串行化边界。
func lockUser(ctx context.Context, tx *entmodel.Tx, userID string) (*entmodel.User, error) {
	value, err := tx.User.Query().Where(user.IDEQ(userID)).ForUpdate().Only(ctx)
	return value, translateEntError(err)
}

func (repo *credentialRepository) FindActivePassword(ctx context.Context, userID string) (*biz.PasswordCredential, error) {
	entity, err := repo.ent.Authenticator.Query().Where(authenticator.UserIDEQ(userID), authenticator.TypeEQ(biz.AuthenticatorPassword), authenticator.StateEQ(biz.AuthenticatorActive), authenticator.RevokedTimeIsNil()).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	passwordEntity, err := repo.ent.PasswordAuthenticator.Query().Where(passwordauthenticator.AuthenticatorIDEQ(entity.ID)).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	return &biz.PasswordCredential{UserID: userID, AuthenticatorID: entity.ID, PasswordHash: passwordEntity.PasswordHash}, nil
}

func (repo *credentialRepository) ReplacePassword(ctx context.Context, userID, authenticatorID, expectedHash, passwordHash, keepLoginID string, now time.Time) error {
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.Status != biz.UserStatusActive {
			return biz.ErrUserNotActive
		}
		_, err = tx.IAMLoginSession.Query().Where(iamloginsession.IDEQ(keepLoginID), iamloginsession.UserIDEQ(userID), iamloginsession.RevokedTimeIsNil()).Only(ctx)
		if entmodel.IsNotFound(err) {
			return biz.ErrSessionRevoked
		}
		if err != nil {
			return translateEntError(err)
		}
		entity, err := tx.Authenticator.Query().Where(authenticator.IDEQ(authenticatorID), authenticator.UserIDEQ(userID), authenticator.TypeEQ(biz.AuthenticatorPassword), authenticator.StateEQ(biz.AuthenticatorActive), authenticator.RevokedTimeIsNil()).Only(ctx)
		if err != nil {
			return translateEntError(err)
		}
		updated, err := tx.PasswordAuthenticator.Update().Where(passwordauthenticator.AuthenticatorIDEQ(entity.ID), passwordauthenticator.PasswordHashEQ(expectedHash)).SetPasswordHash(passwordHash).SetChangedTime(now).Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if updated != 1 {
			return biz.ErrInvalidCredentials
		}
		return revokeUserSessions(ctx, tx, userID, keepLoginID, now)
	})
}

func (repo *sessionRepository) Create(ctx context.Context, userID string, now time.Time) (*biz.LoginSession, error) {
	id, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	var login *entmodel.IAMLoginSession
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.Status != biz.UserStatusActive {
			return biz.ErrUserNotActive
		}
		login, err = tx.IAMLoginSession.Create().SetID(id).SetUserID(userID).SetCreateTime(now).Save(ctx)
		return translateEntError(err)
	})
	if err != nil {
		return nil, err
	}
	return toSession(login), nil
}

func (repo *sessionRepository) Find(ctx context.Context, id string) (*biz.LoginSession, error) {
	entity, err := repo.ent.IAMLoginSession.Get(ctx, id)
	if err != nil {
		return nil, translateEntError(err)
	}
	return toSession(entity), nil
}

func (repo *sessionRepository) Revoke(ctx context.Context, id string, now time.Time) error {
	login, err := repo.ent.IAMLoginSession.Get(ctx, id)
	if entmodel.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return translateEntError(err)
	}
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if _, err := lockUser(ctx, tx, login.UserID); err != nil {
			return err
		}
		if _, err := tx.IAMLoginSession.Update().Where(iamloginsession.IDEQ(id), iamloginsession.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx); err != nil {
			return translateEntError(err)
		}
		return revokeOAuthSessions(ctx, tx, now, oauthtokensession.IamLoginSessionIDEQ(id))
	})
}

func revokeUserSessions(ctx context.Context, tx *entmodel.Tx, userID, keepID string, now time.Time) error {
	update := tx.IAMLoginSession.Update().Where(iamloginsession.UserIDEQ(userID), iamloginsession.RevokedTimeIsNil())
	if keepID != "" {
		update.Where(iamloginsession.IDNEQ(keepID))
	}
	if _, err := update.SetRevokedTime(now).Save(ctx); err != nil {
		return translateEntError(err)
	}
	return revokeOAuthSessions(ctx, tx, now, oauthtokensession.UserIDEQ(userID))
}

func revokeOAuthSessions(ctx context.Context, tx *entmodel.Tx, now time.Time, predicates ...predicate.OAuthTokenSession) error {
	ids, err := tx.OAuthTokenSession.Query().Where(predicates...).IDs(ctx)
	if err != nil {
		return translateEntError(err)
	}
	if len(ids) == 0 {
		return nil
	}
	if _, err := tx.OAuthTokenSession.Update().Where(oauthtokensession.IDIn(ids...), oauthtokensession.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx); err != nil {
		return translateEntError(err)
	}
	if _, err := tx.OAuthAccessToken.Update().Where(oauthaccesstoken.TokenSessionIDIn(ids...), oauthaccesstoken.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx); err != nil {
		return translateEntError(err)
	}
	if _, err := tx.OAuthRefreshToken.Update().Where(oauthrefreshtoken.TokenSessionIDIn(ids...), oauthrefreshtoken.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx); err != nil {
		return translateEntError(err)
	}
	return nil
}

func toSession(entity *entmodel.IAMLoginSession) *biz.LoginSession {
	return &biz.LoginSession{ID: entity.ID, UserID: entity.UserID, AuthTime: entity.CreateTime, RevokedAt: entity.RevokedTime}
}
