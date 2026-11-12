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
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/emailverificationtoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/loginidentifier"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordresettoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
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
	id, err := biz.NewUserID()
	if err != nil {
		return err
	}
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.Status != biz.UserStatusPendingVerification {
			return biz.ErrUserInvalidState
		}
		query := tx.LoginIdentifier.Query().Where(
			loginidentifier.UserIDEQ(userID),
			loginidentifier.TypeEQ(biz.LoginIdentifierEmail),
		)
		if identifierID != "" {
			query = query.Where(loginidentifier.IDEQ(identifierID))
		}
		identifier, err := query.Only(ctx)
		if err != nil {
			if entmodel.IsNotFound(err) {
				return biz.ErrUserInvalidState
			}
			return translateEntError(err)
		}
		if identifier.VerifiedTime != nil {
			return biz.ErrUserInvalidState
		}
		if _, _, err := ensureActivePasswordAuthenticator(ctx, tx, userID); err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrUserInvalidState
			}
			return err
		}
		_, err = tx.EmailVerificationToken.Create().
			SetID(id).
			SetUserID(userID).
			SetLoginIdentifierID(identifier.ID).
			SetTokenHash(tokenHash).
			SetExpiresTime(expires).
			Save(ctx)
		return translateEntError(err)
	})
}

func (repo *verificationTokenRepo) ConsumeAndActivate(ctx context.Context, tokenHash string, now time.Time) (string, error) {
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
		if entmodel.IsNotFound(err) {
			return "", biz.ErrInvalidToken
		}
		return "", translateEntError(err)
	}
	if entity.ConsumedTime != nil {
		return "", biz.ErrInvalidToken
	}
	if !now.Before(entity.ExpiresTime) {
		return "", biz.ErrExpired
	}
	etag, err := biz.NewUserID()
	if err != nil {
		return "", err
	}
	userID := entity.UserID
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrInvalidToken
			}
			return err
		}
		tokenEntity, err := tx.EmailVerificationToken.Get(ctx, entity.ID)
		if entmodel.IsNotFound(err) {
			return biz.ErrInvalidToken
		}
		if err != nil {
			return translateEntError(err)
		}
		if tokenEntity.TokenHash != tokenHash || tokenEntity.UserID != current.ID || tokenEntity.ConsumedTime != nil {
			return biz.ErrInvalidToken
		}
		if !now.Before(tokenEntity.ExpiresTime) {
			return biz.ErrExpired
		}
		if current.Status != biz.UserStatusPendingVerification {
			return biz.ErrInvalidToken
		}
		identifier, err := tx.LoginIdentifier.Query().Where(
			loginidentifier.IDEQ(tokenEntity.LoginIdentifierID),
			loginidentifier.UserIDEQ(current.ID),
			loginidentifier.TypeEQ(biz.LoginIdentifierEmail),
		).Only(ctx)
		if entmodel.IsNotFound(err) {
			return biz.ErrInvalidToken
		}
		if err != nil {
			return translateEntError(err)
		}
		if identifier.VerifiedTime != nil {
			return biz.ErrInvalidToken
		}
		if _, _, err := ensureActivePasswordAuthenticator(ctx, tx, current.ID); err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrInvalidToken
			}
			return err
		}
		affected, err := tx.EmailVerificationToken.Update().
			Where(
				emailverificationtoken.IDEQ(tokenEntity.ID),
				emailverificationtoken.ConsumedTimeIsNil(),
				emailverificationtoken.ExpiresTimeGT(now),
			).
			SetConsumedTime(now).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if affected != 1 {
			return biz.ErrInvalidToken
		}
		if _, err := tx.LoginIdentifier.UpdateOneID(identifier.ID).SetVerifiedTime(now).Save(ctx); err != nil {
			return translateEntError(err)
		}
		affected, err = tx.User.Update().
			Where(user.IDEQ(current.ID), user.DeleteTimeIsNil(), user.StatusEQ(biz.UserStatusPendingVerification)).
			SetStatus(biz.UserStatusActive).
			SetEtag(etag).
			SetUpdateTime(now).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if affected != 1 {
			return biz.ErrInvalidToken
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return userID, nil
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
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.Status != biz.UserStatusActive {
			return biz.ErrUserNotActive
		}
		if _, err := ensureVerifiedEmail(ctx, tx, userID); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrUserNotActive
			}
			return err
		}
		if _, _, err := ensureActivePasswordAuthenticator(ctx, tx, userID); err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrUserNotActive
			}
			return err
		}
		_, err = tx.PasswordResetToken.Create().
			SetID(id).
			SetUserID(userID).
			SetTokenHash(tokenHash).
			SetExpiresTime(expires).
			Save(ctx)
		return translateEntError(err)
	})
}

func (repo *passwordResetTokenRepo) Peek(ctx context.Context, tokenHash string, now time.Time) (*biz.PasswordResetProof, error) {
	if tokenHash == "" {
		return nil, fmt.Errorf("password reset token hash is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	entity, err := repo.ent.PasswordResetToken.Query().
		Where(passwordresettoken.TokenHashEQ(tokenHash)).
		Only(ctx)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return nil, biz.ErrInvalidToken
		}
		return nil, translateEntError(err)
	}
	if entity.ConsumedTime != nil {
		return nil, biz.ErrInvalidToken
	}
	if !now.Before(entity.ExpiresTime) {
		return nil, biz.ErrExpired
	}
	userEntity, err := repo.ent.User.Get(ctx, entity.UserID)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return nil, biz.ErrInvalidToken
		}
		return nil, translateEntError(err)
	}
	if userEntity.DeleteTime != nil || userEntity.Status != biz.UserStatusActive {
		return nil, biz.ErrInvalidToken
	}
	identifier, err := repo.ent.LoginIdentifier.Query().Where(
		loginidentifier.UserIDEQ(userEntity.ID),
		loginidentifier.TypeEQ(biz.LoginIdentifierEmail),
	).Only(ctx)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return nil, biz.ErrInvalidToken
		}
		return nil, translateEntError(err)
	}
	if identifier.VerifiedTime == nil {
		return nil, biz.ErrInvalidToken
	}
	authEntity, err := repo.ent.Authenticator.Query().Where(
		authenticator.UserIDEQ(userEntity.ID),
		authenticator.TypeEQ(biz.AuthenticatorPassword),
		authenticator.StateEQ(biz.AuthenticatorActive),
		authenticator.RevokedTimeIsNil(),
	).Only(ctx)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return nil, biz.ErrInvalidToken
		}
		return nil, translateEntError(err)
	}
	passEntity, err := repo.ent.PasswordAuthenticator.Query().Where(
		passwordauthenticator.AuthenticatorIDEQ(authEntity.ID),
	).Only(ctx)
	if err != nil {
		if entmodel.IsNotFound(err) {
			return nil, biz.ErrInvalidToken
		}
		return nil, translateEntError(err)
	}
	if _, err := requiredActionsFromStrings(userEntity.RequiredActions); err != nil {
		return nil, err
	}
	return &biz.PasswordResetProof{
		TokenID:   entity.ID,
		TokenHash: tokenHash,
		UserEtag:  userEntity.Etag,
		Credential: biz.PasswordCredential{
			UserID:                  userEntity.ID,
			AuthenticatorID:         authEntity.ID,
			PasswordAuthenticatorID: passEntity.ID,
			PasswordHash:            passEntity.PasswordHash,
		},
		RequiresPasswordChange: slices.Contains(userEntity.RequiredActions, storedRequiredActionUpdatePassword),
	}, nil
}

func (repo *passwordResetTokenRepo) ConsumeAndReplacePassword(ctx context.Context, proof biz.PasswordResetProof, passwordHash string, now time.Time) (string, error) {
	if proof.TokenID == "" || proof.TokenHash == "" || proof.UserEtag == "" || proof.Credential.UserID == "" || passwordHash == "" {
		return "", fmt.Errorf("password reset proof and password hash are required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	etag, err := biz.NewUserID()
	if err != nil {
		return "", err
	}
	userID := proof.Credential.UserID
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrInvalidToken
			}
			return err
		}
		tokenEntity, err := tx.PasswordResetToken.Get(ctx, proof.TokenID)
		if entmodel.IsNotFound(err) {
			return biz.ErrInvalidToken
		}
		if err != nil {
			return translateEntError(err)
		}
		if tokenEntity.TokenHash != proof.TokenHash || tokenEntity.UserID != userID || tokenEntity.ConsumedTime != nil {
			return biz.ErrInvalidToken
		}
		if !now.Before(tokenEntity.ExpiresTime) {
			return biz.ErrExpired
		}
		if current.Status != biz.UserStatusActive {
			return biz.ErrInvalidToken
		}
		if _, err := ensureVerifiedEmail(ctx, tx, userID); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrInvalidToken
			}
			return err
		}
		if _, err := requiredActionsFromStrings(current.RequiredActions); err != nil {
			return err
		}
		if current.Etag != proof.UserEtag {
			return biz.ErrAccountInvalidState
		}
		if _, err := verifyPasswordCredentialSnapshot(ctx, tx, proof.Credential); err != nil {
			if errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrAccountInvalidState
			}
			return err
		}
		hasUpdatePassword := slices.Contains(current.RequiredActions, storedRequiredActionUpdatePassword)
		if hasUpdatePassword != proof.RequiresPasswordChange {
			return biz.ErrAccountInvalidState
		}
		consumed, err := tx.PasswordResetToken.Update().
			Where(
				passwordresettoken.IDEQ(tokenEntity.ID),
				passwordresettoken.ConsumedTimeIsNil(),
				passwordresettoken.ExpiresTimeGT(now),
			).
			SetConsumedTime(now).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if consumed != 1 {
			return biz.ErrInvalidToken
		}
		updated, err := tx.PasswordAuthenticator.Update().
			Where(
				passwordauthenticator.IDEQ(proof.Credential.PasswordAuthenticatorID),
				passwordauthenticator.AuthenticatorIDEQ(proof.Credential.AuthenticatorID),
				passwordauthenticator.PasswordHashEQ(proof.Credential.PasswordHash),
			).
			SetPasswordHash(passwordHash).
			SetChangedTime(now).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if updated != 1 {
			return biz.ErrAccountInvalidState
		}
		if hasUpdatePassword {
			remaining := make([]string, 0, len(current.RequiredActions))
			for _, action := range current.RequiredActions {
				if action != storedRequiredActionUpdatePassword {
					remaining = append(remaining, action)
				}
			}
			affected, err := tx.User.Update().
				Where(user.IDEQ(userID), user.DeleteTimeIsNil(), user.EtagEQ(proof.UserEtag)).
				SetRequiredActions(remaining).
				SetEtag(etag).
				SetUpdateTime(now).
				Save(ctx)
			if err != nil {
				return translateEntError(err)
			}
			if affected != 1 {
				return biz.ErrAccountInvalidState
			}
		}
		return revokeUserSessions(ctx, tx, userID, "", now)
	})
	if err != nil {
		return "", err
	}
	return userID, nil
}
