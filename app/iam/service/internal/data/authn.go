package data

import (
	"context"
	"fmt"
	"time"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/iamloginsession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthrefreshtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/predicate"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type credentialRepository struct{ data *Data }
type sessionRepository struct{ data *Data }
type tokenSessionRepository struct{ data *Data }

func NewTokenSessionRepository(data *Data) (biz.TokenSessionRepo, error) {
	if data == nil {
		return nil, fmt.Errorf("token session repository: data is nil")
	}
	return &tokenSessionRepository{data: data}, nil
}

func NewCredentialRepository(data *Data) (biz.CredentialRepo, error) {
	if data == nil {
		return nil, fmt.Errorf("credential repository: data is nil")
	}
	return &credentialRepository{data: data}, nil
}

func NewSessionRepository(data *Data) (biz.SessionRepo, error) {
	if data == nil {
		return nil, fmt.Errorf("session repository: data is nil")
	}
	return &sessionRepository{data: data}, nil
}

func (repo *tokenSessionRepository) RevokeForLoginSession(ctx context.Context, loginSessionID string, now time.Time) error {
	if loginSessionID == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	return repo.revoke(ctx, now, oauthtokensession.IamLoginSessionIDEQ(loginSessionID))
}

func (repo *tokenSessionRepository) RevokeAllForUser(ctx context.Context, userID string, now time.Time) error {
	if userID == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	return repo.revoke(ctx, now, oauthtokensession.UserIDEQ(userID))
}

func (repo *tokenSessionRepository) revoke(ctx context.Context, now time.Time, predicates ...predicate.OAuthTokenSession) error {
	return repo.data.InTx(ctx, func(tx *entmodel.Tx) error {
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
	})
}

func (repo *credentialRepository) FindActivePassword(ctx context.Context, userID string) (*biz.PasswordCredential, error) {
	entity, err := repo.data.ent.Authenticator.Query().Where(authenticator.UserIDEQ(userID), authenticator.TypeEQ(biz.AuthenticatorPassword), authenticator.StateEQ(biz.AuthenticatorActive), authenticator.RevokedTimeIsNil()).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	passwordEntity, err := repo.data.ent.PasswordAuthenticator.Query().Where(passwordauthenticator.AuthenticatorIDEQ(entity.ID)).Only(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	return &biz.PasswordCredential{UserID: userID, AuthenticatorID: entity.ID, PasswordHash: passwordEntity.PasswordHash}, nil
}

func (repo *credentialRepository) ReplacePassword(ctx context.Context, userID, authenticatorID, passwordHash string, now time.Time) error {
	if userID == "" || authenticatorID == "" || passwordHash == "" {
		return fmt.Errorf("password replacement identifiers and hash are required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	return repo.data.InTx(ctx, func(tx *entmodel.Tx) error {
		authenticatorEntity, err := tx.Authenticator.Query().Where(authenticator.IDEQ(authenticatorID), authenticator.UserIDEQ(userID), authenticator.TypeEQ(biz.AuthenticatorPassword), authenticator.StateEQ(biz.AuthenticatorActive), authenticator.RevokedTimeIsNil()).Only(ctx)
		if err != nil {
			return translateEntError(err)
		}
		passwordEntity, err := tx.PasswordAuthenticator.Query().Where(passwordauthenticator.AuthenticatorIDEQ(authenticatorEntity.ID)).Only(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if _, err := tx.PasswordAuthenticator.UpdateOneID(passwordEntity.ID).SetPasswordHash(passwordHash).Save(ctx); err != nil {
			return translateEntError(err)
		}
		_, err = tx.Authenticator.UpdateOneID(authenticatorEntity.ID).SetLastUsedTime(now).SetUpdateTime(now).Save(ctx)
		return translateEntError(err)
	})
}

func (repo *sessionRepository) Create(ctx context.Context, userID, secretHash string, now time.Time) (*sessionpb.Session, error) {
	id, err := biz.NewUserID()
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	entity, err := repo.data.ent.IAMLoginSession.Create().SetID(id).SetUserID(userID).SetSecretHash(secretHash).SetCreateTime(now).SetLastSeenTime(now).SetIdleExpiresTime(now.Add(biz.LoginSessionIdleTTL)).SetAbsoluteExpiresTime(now.Add(biz.LoginSessionAbsoluteTTL)).Save(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	return toSession(entity), nil
}

func (repo *sessionRepository) FindBySecretHash(ctx context.Context, secretHash string) (*sessionpb.Session, string, bool, error) {
	entity, err := repo.data.ent.IAMLoginSession.Query().Where(iamloginsession.SecretHashEQ(secretHash)).Only(ctx)
	if err != nil {
		return nil, "", false, translateEntError(err)
	}
	return toSession(entity), entity.UserID, entity.RevokedTime != nil, nil
}

func (repo *sessionRepository) Touch(ctx context.Context, id string, now time.Time) (*sessionpb.Session, error) {
	if now.IsZero() {
		now = time.Now()
	}
	current, err := repo.data.ent.IAMLoginSession.Get(ctx, id)
	if err != nil {
		return nil, translateEntError(err)
	}
	idleExpires := now.Add(biz.LoginSessionIdleTTL)
	if idleExpires.After(current.AbsoluteExpiresTime) {
		idleExpires = current.AbsoluteExpiresTime
	}
	updated, err := repo.data.ent.IAMLoginSession.UpdateOneID(id).Where(iamloginsession.RevokedTimeIsNil(), iamloginsession.IdleExpiresTimeGT(now), iamloginsession.AbsoluteExpiresTimeGT(now)).SetLastSeenTime(now).SetIdleExpiresTime(idleExpires).Save(ctx)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return nil, biz.ErrMutationMiss
		}
		return nil, translateEntError(err)
	}
	return toSession(updated), nil
}

func (repo *sessionRepository) Revoke(ctx context.Context, id string, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	_, err := repo.data.ent.IAMLoginSession.UpdateOneID(id).Where(iamloginsession.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx)
	if entmodel.IsNotFound(err) {
		return nil
	}
	return translateEntError(err)
}

func (repo *sessionRepository) RevokeOthersForUser(ctx context.Context, userID, keepSessionID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	_, err := repo.data.ent.IAMLoginSession.Update().Where(iamloginsession.UserIDEQ(userID), iamloginsession.IDNEQ(keepSessionID), iamloginsession.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx)
	return translateEntError(err)
}

func (repo *sessionRepository) RevokeAllForUser(ctx context.Context, userID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	_, err := repo.data.ent.IAMLoginSession.Update().Where(iamloginsession.UserIDEQ(userID), iamloginsession.RevokedTimeIsNil()).SetRevokedTime(now).Save(ctx)
	return translateEntError(err)
}

func toSession(entity *entmodel.IAMLoginSession) *sessionpb.Session {
	return &sessionpb.Session{
		Name: "sessions/" + entity.ID, SessionId: entity.ID,
		CreateTime: timestamppb.New(entity.CreateTime), LastSeenTime: timestamppb.New(entity.LastSeenTime),
		IdleExpiresTime: timestamppb.New(entity.IdleExpiresTime), AbsoluteExpiresTime: timestamppb.New(entity.AbsoluteExpiresTime),
	}
}
