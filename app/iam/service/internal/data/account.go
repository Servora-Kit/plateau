package data

import (
	"context"
	"fmt"
	"time"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/emailverificationtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/loginidentifier"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordresettoken"
)

type verificationTokenRepo struct{ ent *entmodel.Client }

// NewVerificationTokenRepo provides one-time email verification token operations.
func NewVerificationTokenRepo(data *Data) (biz.VerificationTokenRepo, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("verification token repository: data is nil")
	}
	return &verificationTokenRepo{ent: data.ent}, nil
}

func (repo *verificationTokenRepo) Create(ctx context.Context, userID, identifierID, tokenHash string, expires time.Time) error {
	if userID == "" || tokenHash == "" {
		return fmt.Errorf("verification token: user and token hash are required")
	}
	if identifierID == "" {
		identifier, err := repo.ent.LoginIdentifier.Query().
			Where(loginidentifier.UserIDEQ(userID), loginidentifier.TypeEQ(biz.LoginIdentifierEmail)).
			Only(ctx)
		if err != nil {
			return translateEntError(err)
		}
		identifierID = identifier.ID
	}
	id, err := biz.NewUserID()
	if err != nil {
		return err
	}
	_, err = repo.ent.EmailVerificationToken.Create().
		SetID(id).
		SetUserID(userID).
		SetLoginIdentifierID(identifierID).
		SetTokenHash(tokenHash).
		SetExpiresTime(expires).
		Save(ctx)
	return translateEntError(err)
}

func (repo *verificationTokenRepo) Consume(ctx context.Context, tokenHash string, now time.Time) (string, error) {
	if tokenHash == "" {
		return "", fmt.Errorf("verification token hash is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	entity, err := repo.ent.EmailVerificationToken.Query().
		Where(emailverificationtoken.TokenHashEQ(tokenHash)).
		Only(ctx)
	if err != nil {
		return "", translateEntError(err)
	}
	if entity.ConsumedTime != nil {
		return "", biz.ErrMutationMiss
	}
	if !now.Before(entity.ExpiresTime) {
		return "", biz.ErrExpired
	}
	updated, err := repo.ent.EmailVerificationToken.UpdateOneID(entity.ID).
		Where(emailverificationtoken.ConsumedTimeIsNil(), emailverificationtoken.ExpiresTimeGT(now)).
		SetConsumedTime(now).
		Save(ctx)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return "", biz.ErrMutationMiss
		}
		return "", translateEntError(err)
	}
	return updated.UserID, nil
}

type passwordResetTokenRepo struct{ ent *entmodel.Client }

// NewPasswordResetTokenRepository provides atomic reset-token consumption and password replacement.
func NewPasswordResetTokenRepository(data *Data) (biz.PasswordResetTokenRepo, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("password reset token repository: data is nil")
	}
	return &passwordResetTokenRepo{ent: data.ent}, nil
}

func (repo *passwordResetTokenRepo) Create(ctx context.Context, userID, tokenHash string, expires time.Time) error {
	if userID == "" || tokenHash == "" {
		return fmt.Errorf("password reset token: user and token hash are required")
	}
	id, err := biz.NewUserID()
	if err != nil {
		return err
	}
	_, err = repo.ent.PasswordResetToken.Create().SetID(id).SetUserID(userID).SetTokenHash(tokenHash).SetExpiresTime(expires).Save(ctx)
	return translateEntError(err)
}

func (repo *passwordResetTokenRepo) ConsumeAndReplacePassword(ctx context.Context, tokenHash, passwordHash string, now time.Time) (string, error) {
	if tokenHash == "" || passwordHash == "" {
		return "", fmt.Errorf("password reset token hash and password hash are required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	entity, err := repo.ent.PasswordResetToken.Query().Where(passwordresettoken.TokenHashEQ(tokenHash)).Only(ctx)
	if err != nil {
		return "", translateEntError(err)
	}
	if entity.ConsumedTime != nil {
		return "", biz.ErrMutationMiss
	}
	if !now.Before(entity.ExpiresTime) {
		return "", biz.ErrExpired
	}
	userID := entity.UserID
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.Status != biz.UserStatusActive {
			return biz.ErrUserNotActive
		}
		entity, err = tx.PasswordResetToken.Get(ctx, entity.ID)
		if err != nil {
			return translateEntError(err)
		}
		if entity.ConsumedTime != nil {
			return biz.ErrMutationMiss
		}
		if !now.Before(entity.ExpiresTime) {
			return biz.ErrExpired
		}
		authenticatorEntity, err := tx.Authenticator.Query().Where(
			authenticator.UserIDEQ(userID),
			authenticator.TypeEQ(biz.AuthenticatorPassword),
			authenticator.StateEQ(biz.AuthenticatorActive),
			authenticator.RevokedTimeIsNil(),
		).Only(ctx)
		if err != nil {
			return translateEntError(err)
		}
		passwordEntity, err := tx.PasswordAuthenticator.Query().Where(passwordauthenticator.AuthenticatorIDEQ(authenticatorEntity.ID)).Only(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if _, err := tx.PasswordAuthenticator.UpdateOneID(passwordEntity.ID).SetPasswordHash(passwordHash).SetChangedTime(now).Save(ctx); err != nil {
			return translateEntError(err)
		}
		updated, err := tx.PasswordResetToken.UpdateOneID(entity.ID).Where(passwordresettoken.ConsumedTimeIsNil(), passwordresettoken.ExpiresTimeGT(now)).SetConsumedTime(now).Save(ctx)
		if err != nil {
			if entmodel.IsNotFound(err) {
				return biz.ErrMutationMiss
			}
			return translateEntError(err)
		}
		if updated == nil {
			return biz.ErrMutationMiss
		}
		return revokeUserSessions(ctx, tx, userID, "", now)
	})
	if err != nil {
		return "", err
	}
	return userID, nil
}
