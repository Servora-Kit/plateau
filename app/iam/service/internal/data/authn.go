package data

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/iamloginsession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/loginidentifier"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthrefreshtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/predicate"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
	entgomixin "github.com/Servora-Kit/plateau/infra/entgo/mixin"
	crudmapper "github.com/Servora-Kit/servora/core/crud/mapper"
)

type credentialRepository struct{ ent *entmodel.Client }

type sessionRepository struct {
	ent    *entmodel.Client
	mapper *crudmapper.ResourceMapper[*userpb.User, entmodel.User]
}

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
	mapper, err := newUserResourceMapper()
	if err != nil {
		return nil, err
	}
	return &sessionRepository{ent: data.ent, mapper: mapper}, nil
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

func insertLoginSession(ctx context.Context, tx *entmodel.Tx, userID string, now time.Time) (*biz.LoginSession, error) {
	id, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	login, err := tx.IAMLoginSession.Create().SetID(id).SetUserID(userID).SetCreateTime(now).Save(ctx)
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
		_, err = tx.IAMLoginSession.Query().Where(iamloginsession.IDEQ(keepLoginID), iamloginsession.UserIDEQ(userID), iamloginsession.RevokedTimeIsNil()).Only(ctx)
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
		).SetPasswordHash(passwordHash).SetChangedTime(now).Save(ctx)
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
			SetChangedTime(now).
			Save(ctx); err != nil {
			return translateEntError(err)
		}
		return revokeUserSessions(ctx, tx, userID, "", now)
	})
}

func (repo *credentialRepository) CompleteInitialPassword(ctx context.Context, expected biz.PasswordCredential, expectedUserEtag, newHash string, now time.Time) (*biz.LoginSession, error) {
	if expected.UserID == "" || expectedUserEtag == "" || newHash == "" {
		return nil, fmt.Errorf("complete initial password: credential snapshot, expected etag, and new hash are required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	etag, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	var login *biz.LoginSession
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, expected.UserID)
		if err != nil {
			return err
		}
		if current.Status != biz.UserStatusActive {
			return biz.ErrUnauthenticated
		}
		if _, err := ensureVerifiedEmail(ctx, tx, expected.UserID); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrUnauthenticated
			}
			return err
		}
		if current.Etag != expectedUserEtag {
			return biz.ErrAccountInvalidState
		}
		if _, err := verifyPasswordCredentialSnapshot(ctx, tx, expected); err != nil {
			if errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrAccountInvalidState
			}
			return err
		}
		if _, err := requiredActionsFromStrings(current.RequiredActions); err != nil {
			return err
		}
		if !slices.Contains(current.RequiredActions, storedRequiredActionUpdatePassword) {
			return biz.ErrAccountInvalidState
		}
		remaining := make([]string, 0, len(current.RequiredActions))
		for _, action := range current.RequiredActions {
			if action != storedRequiredActionUpdatePassword {
				remaining = append(remaining, action)
			}
		}
		if len(remaining) > 0 {
			return biz.ErrAccountInvalidState
		}
		updated, err := tx.PasswordAuthenticator.Update().Where(
			passwordauthenticator.IDEQ(expected.PasswordAuthenticatorID),
			passwordauthenticator.AuthenticatorIDEQ(expected.AuthenticatorID),
			passwordauthenticator.PasswordHashEQ(expected.PasswordHash),
		).SetPasswordHash(newHash).SetChangedTime(now).Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if updated != 1 {
			return biz.ErrAccountInvalidState
		}
		affected, err := tx.User.Update().Where(
			user.IDEQ(expected.UserID),
			user.DeleteTimeIsNil(),
			user.EtagEQ(expectedUserEtag),
		).SetRequiredActions([]string{}).SetEtag(etag).SetUpdateTime(now).Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if affected != 1 {
			return biz.ErrAccountInvalidState
		}
		if err := revokeUserSessions(ctx, tx, expected.UserID, "", now); err != nil {
			return err
		}
		login, err = insertLoginSession(ctx, tx, expected.UserID, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return login, nil
}

func (repo *sessionRepository) Create(ctx context.Context, expected biz.PasswordCredential, expectedUserEtag string, now time.Time) (*userpb.User, *biz.LoginSession, error) {
	if expected.UserID == "" || expectedUserEtag == "" {
		return nil, nil, fmt.Errorf("create session: credential snapshot and expected user etag are required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	var projected *userpb.User
	var login *biz.LoginSession
	err := inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, expected.UserID)
		if err != nil {
			return err
		}
		if current.Etag != expectedUserEtag {
			return biz.ErrMutationMiss
		}
		if current.Status != biz.UserStatusActive {
			return biz.ErrUserNotActive
		}
		if _, err := ensureVerifiedEmail(ctx, tx, expected.UserID); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrUserNotActive
			}
			return err
		}
		if _, err := verifyPasswordCredentialSnapshot(ctx, tx, expected); err != nil {
			if errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrInvalidCredentials
			}
			return err
		}
		resources, err := projectUsersWithClient(ctx, tx.Client(), repo.mapper, []*entmodel.User{current})
		if err != nil {
			return err
		}
		projected = resources[0]
		if len(projected.GetRequiredActions()) > 0 {
			return nil
		}
		login, err = insertLoginSession(ctx, tx, expected.UserID, now)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return projected, login, nil
}

func (repo *sessionRepository) Find(ctx context.Context, id string) (*biz.LoginSession, error) {
	entity, err := repo.ent.IAMLoginSession.Get(ctx, id)
	if err != nil {
		return nil, translateEntError(err)
	}
	return toSession(entity), nil
}

func (repo *sessionRepository) Revoke(ctx context.Context, id string, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	login, err := repo.ent.IAMLoginSession.Get(ctx, id)
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
		if _, err := tx.IAMLoginSession.Update().Where(iamloginsession.IDEQ(id), iamloginsession.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx); err != nil {
			return translateEntError(err)
		}
		return revokeOAuthSessions(ctx, tx, now, oauthtokensession.IamLoginSessionIDEQ(id))
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
func toSession(entity *entmodel.IAMLoginSession) *biz.LoginSession {
	return &biz.LoginSession{ID: entity.ID, UserID: entity.UserID, AuthTime: entity.CreateTime, RevokedAt: entity.RevokedTime}
}
