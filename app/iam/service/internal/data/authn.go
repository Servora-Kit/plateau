package data

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authflow"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/loginidentifier"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/loginsession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthrefreshtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/predicate"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
	entgomixin "github.com/Servora-Kit/plateau/infra/entgo/mixin"
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
	value, err := tx.User.Query().Where(user.IDEQ(userID), user.DeleteTimeIsNil()).ForUpdate().Only(ctx)
	return value, translateEntError(err)
}

// lockUserIncludingDeleted locks the target User row regardless of soft-delete state.
// Only Undelete and Expunge may call this helper.
func lockUserIncludingDeleted(ctx context.Context, tx *entmodel.Tx, userID string) (*entmodel.User, error) {
	value, err := tx.User.Query().Where(user.IDEQ(userID)).ForUpdate().Only(entgomixin.SkipSoftDelete(ctx))
	return value, translateEntError(err)
}

func ensureVerifiedEmail(ctx context.Context, tx *entmodel.Tx, userID string) (*entmodel.LoginIdentifier, error) {
	identifier, err := tx.LoginIdentifier.Query().Where(
		loginidentifier.UserIDEQ(userID),
		loginidentifier.TypeEQ(biz.LoginIdentifierEmail),
	).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	if identifier.VerifiedTime == nil {
		return nil, biz.ErrMutationMiss
	}
	return identifier, nil
}

func ensureActivePasswordAuthenticator(ctx context.Context, tx *entmodel.Tx, userID string) (*entmodel.Authenticator, *entmodel.PasswordAuthenticator, error) {
	authEntity, err := tx.Authenticator.Query().Where(
		authenticator.UserIDEQ(userID),
		authenticator.TypeEQ(biz.AuthenticatorPassword),
		authenticator.StateEQ(biz.AuthenticatorActive),
		authenticator.RevokedTimeIsNil(),
	).Only(ctx)
	if err != nil {
		return nil, nil, translateEntError(err)
	}
	passEntity, err := tx.PasswordAuthenticator.Query().Where(
		passwordauthenticator.AuthenticatorIDEQ(authEntity.ID),
	).Only(ctx)
	if err != nil {
		return nil, nil, translateEntError(err)
	}
	return authEntity, passEntity, nil
}

func verifyPasswordCredentialSnapshot(ctx context.Context, tx *entmodel.Tx, expected biz.PasswordCredential) (*entmodel.PasswordAuthenticator, error) {
	if expected.UserID == "" || expected.AuthenticatorID == "" || expected.PasswordAuthenticatorID == "" || expected.PasswordHash == "" {
		return nil, biz.ErrMutationMiss
	}
	if _, err := tx.Authenticator.Query().Where(
		authenticator.IDEQ(expected.AuthenticatorID),
		authenticator.UserIDEQ(expected.UserID),
		authenticator.TypeEQ(biz.AuthenticatorPassword),
		authenticator.StateEQ(biz.AuthenticatorActive),
		authenticator.RevokedTimeIsNil(),
	).Only(ctx); err != nil {
		if entmodel.IsNotFound(err) {
			return nil, biz.ErrMutationMiss
		}
		return nil, translateEntError(err)
	}
	passEntity, err := tx.PasswordAuthenticator.Query().Where(
		passwordauthenticator.IDEQ(expected.PasswordAuthenticatorID),
		passwordauthenticator.AuthenticatorIDEQ(expected.AuthenticatorID),
	).Only(ctx)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return nil, biz.ErrMutationMiss
		}
		return nil, translateEntError(err)
	}
	if passEntity.PasswordHash != expected.PasswordHash {
		return nil, biz.ErrMutationMiss
	}
	return passEntity, nil
}

func insertLoginSession(ctx context.Context, tx *entmodel.Tx, userID string, amr []string, now time.Time) (*biz.LoginSession, error) {
	normalizedAMR := biz.NormalizeAMR(amr)
	if len(normalizedAMR) == 0 {
		return nil, fmt.Errorf("insert login session: AMR is required")
	}
	id, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	login, err := tx.LoginSession.Create().
		SetID(id).
		SetUserID(userID).
		SetAmr(normalizedAMR).
		SetCreateTime(now).
		Save(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	return toSession(login), nil
}

func (repo *credentialRepository) FindActivePassword(ctx context.Context, userID string) (*biz.PasswordCredential, error) {
	entity, err := repo.ent.Authenticator.Query().Where(
		authenticator.UserIDEQ(userID),
		authenticator.TypeEQ(biz.AuthenticatorPassword),
		authenticator.StateEQ(biz.AuthenticatorActive),
		authenticator.RevokedTimeIsNil(),
	).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	passwordEntity, err := repo.ent.PasswordAuthenticator.Query().Where(passwordauthenticator.AuthenticatorIDEQ(entity.ID)).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	return &biz.PasswordCredential{
		UserID:                  userID,
		AuthenticatorID:         entity.ID,
		PasswordAuthenticatorID: passwordEntity.ID,
		PasswordHash:            passwordEntity.PasswordHash,
		ChangedTime:             biz.NormalizeTimestamp(passwordEntity.ChangedTime),
	}, nil
}

func (repo *credentialRepository) ReplacePassword(ctx context.Context, userID, authenticatorID, expectedHash, passwordHash, keepLoginID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.Status != biz.UserStatusActive {
			return biz.ErrUserNotActive
		}
		if len(current.RequiredActions) > 0 {
			return biz.ErrRequiredActionPending
		}
		if _, err := ensureVerifiedEmail(ctx, tx, userID); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrUserNotActive
			}
			return err
		}
		_, err = tx.LoginSession.Query().Where(loginsession.IDEQ(keepLoginID), loginsession.UserIDEQ(userID), loginsession.RevokedTimeIsNil()).Only(ctx)
		if entmodel.IsNotFound(err) {
			return biz.ErrSessionRevoked
		}
		if err != nil {
			return translateEntError(err)
		}
		entity, err := tx.Authenticator.Query().Where(
			authenticator.IDEQ(authenticatorID),
			authenticator.UserIDEQ(userID),
			authenticator.TypeEQ(biz.AuthenticatorPassword),
			authenticator.StateEQ(biz.AuthenticatorActive),
			authenticator.RevokedTimeIsNil(),
		).Only(ctx)
		if err != nil {
			return translateEntError(err)
		}
		updated, err := tx.PasswordAuthenticator.Update().Where(
			passwordauthenticator.AuthenticatorIDEQ(entity.ID),
			passwordauthenticator.PasswordHashEQ(expectedHash),
		).SetPasswordHash(passwordHash).SetChangedTime(biz.NormalizeTimestamp(now)).Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if updated != 1 {
			return biz.ErrInvalidCredentials
		}
		return revokeUserSessions(ctx, tx, userID, keepLoginID, now)
	})
}

func (repo *credentialRepository) SetPasswordAndRevokeSessions(ctx context.Context, userID, passwordHash string, now time.Time) error {
	if userID == "" || passwordHash == "" {
		return fmt.Errorf("set password: user ID and password hash are required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		authEntity, err := tx.Authenticator.Query().Where(
			authenticator.UserIDEQ(userID),
			authenticator.TypeEQ(biz.AuthenticatorPassword),
		).Only(ctx)
		if err != nil {
			if entmodel.IsNotFound(err) {
				return biz.ErrUserInvalidState
			}
			return translateEntError(err)
		}
		passEntity, err := tx.PasswordAuthenticator.Query().Where(
			passwordauthenticator.AuthenticatorIDEQ(authEntity.ID),
		).Only(ctx)
		if err != nil {
			if entmodel.IsNotFound(err) {
				return biz.ErrUserInvalidState
			}
			return translateEntError(err)
		}
		if _, err := tx.PasswordAuthenticator.UpdateOneID(passEntity.ID).
			SetPasswordHash(passwordHash).
			SetChangedTime(biz.NormalizeTimestamp(now)).
			Save(ctx); err != nil {
			return translateEntError(err)
		}
		return revokeUserSessions(ctx, tx, userID, "", now)
	})
}

func (repo *sessionRepository) Find(ctx context.Context, id string) (*biz.LoginSession, error) {
	entity, err := repo.ent.LoginSession.Get(ctx, id)
	if err != nil {
		return nil, translateEntError(err)
	}
	return toSession(entity), nil
}

func (repo *sessionRepository) Revoke(ctx context.Context, id string, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	login, err := repo.ent.LoginSession.Get(ctx, id)
	if entmodel.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return translateEntError(err)
	}
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if _, err := lockUser(ctx, tx, login.UserID); err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return nil
			}
			return err
		}
		if _, err := tx.LoginSession.Update().Where(loginsession.IDEQ(id), loginsession.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx); err != nil {
			return translateEntError(err)
		}
		return revokeOAuthSessions(ctx, tx, now, oauthtokensession.LoginSessionIDEQ(id))
	})
}
func (repo *sessionRepository) RevokeAll(ctx context.Context, userID string, now time.Time) error {
	if userID == "" {
		return fmt.Errorf("revoke all sessions: user ID is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		if err := revokeUserSessions(ctx, tx, userID, "", now); err != nil {
			return err
		}
		_, err := tx.OAuthAccessToken.Update().Where(
			oauthaccesstoken.ActorTypeEQ(oauthaccesstoken.ActorTypeHuman),
			oauthaccesstoken.SubjectEQ(userID),
			oauthaccesstoken.RevokedTimeIsNil(),
		).SetRevokedTime(now).Save(ctx)
		return translateEntError(err)
	})
}

func revokeUserSessions(ctx context.Context, tx *entmodel.Tx, userID, keepID string, now time.Time) error {
	return revokeUserSessionsAndFlows(ctx, tx, userID, keepID, "", now)
}

func revokeUserSessionsAndFlows(ctx context.Context, tx *entmodel.Tx, userID, keepID, exceptFlowID string, now time.Time) error {
	update := tx.LoginSession.Update().Where(loginsession.UserIDEQ(userID), loginsession.RevokedTimeIsNil())
	if keepID != "" {
		update.Where(loginsession.IDNEQ(keepID))
	}
	if _, err := update.SetRevokedTime(now).Save(ctx); err != nil {
		return translateEntError(err)
	}
	if err := invalidateUserAuthFlows(ctx, tx, userID, exceptFlowID, now); err != nil {
		return err
	}
	return revokeOAuthSessions(ctx, tx, now, oauthtokensession.UserIDEQ(userID))
}

func invalidateUserAuthFlows(ctx context.Context, tx *entmodel.Tx, userID, exceptFlowID string, now time.Time) error {
	if userID == "" {
		return nil
	}
	update := tx.AuthFlow.Update().Where(
		authflow.UserIDEQ(userID),
		authflow.StatusEQ(authflow.StatusActive),
	)
	if exceptFlowID != "" {
		update.Where(authflow.IDNEQ(exceptFlowID))
	}
	_, err := update.SetStatus(authflow.StatusInvalidated).SetUpdateTime(now).Save(ctx)
	return translateEntError(err)
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
	if _, err := tx.OAuthAccessToken.Update().Where(
		oauthaccesstoken.ActorTypeEQ(oauthaccesstoken.ActorTypeHuman),
		oauthaccesstoken.TokenSessionIDIn(ids...),
		oauthaccesstoken.RevokedTimeIsNil(),
	).SetRevokedTime(now).Save(ctx); err != nil {
		return translateEntError(err)
	}
	if _, err := tx.OAuthRefreshToken.Update().Where(oauthrefreshtoken.TokenSessionIDIn(ids...), oauthrefreshtoken.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx); err != nil {
		return translateEntError(err)
	}
	return nil
}
func toSession(entity *entmodel.LoginSession) *biz.LoginSession {
	return &biz.LoginSession{
		ID:        entity.ID,
		UserID:    entity.UserID,
		AMR:       slices.Clone(entity.Amr),
		AuthTime:  entity.CreateTime,
		RevokedAt: entity.RevokedTime,
	}
}
