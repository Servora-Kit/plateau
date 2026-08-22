package oidc

import (
	"context"
	"fmt"
	"slices"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/loginidentifier"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/text/language"
)

func (storage *OIDCStorage) activeAccessToken(
	ctx context.Context,
	tokenID, subject string,
) (*entmodel.OAuthAccessToken, error) {
	now := storage.now().UTC()
	token, err := storage.client.OAuthAccessToken.Query().
		Where(
			oauthaccesstoken.IDEQ(tokenID),
			oauthaccesstoken.SubjectEQ(subject),
			oauthaccesstoken.ExpiresTimeGT(now),
			oauthaccesstoken.RevokedTimeIsNil(),
		).
		Only(ctx)
	if entmodel.IsNotFound(err) {
		return nil, storageNotFoundError{cause: fmt.Errorf("OAuth access token is inactive")}
	}
	if err != nil {
		return nil, fmt.Errorf("query OAuth access token: %w", err)
	}
	sessionActive, err := storage.client.OAuthTokenSession.Query().
		Where(oauthtokensession.IDEQ(token.TokenSessionID), oauthtokensession.RevokedTimeIsNil()).
		Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("check OAuth token session: %w", err)
	}
	if !sessionActive {
		return nil, storageNotFoundError{cause: fmt.Errorf("OAuth token session is inactive")}
	}
	return token, nil
}

func (storage *OIDCStorage) populateUserinfo(
	ctx context.Context,
	userinfo *oidc.UserInfo,
	userID, _ string,
	scopes []string,
) error {
	entity, err := storage.client.User.Query().
		Where(user.IDEQ(userID), user.StatusEQ(biz.UserStatusActive)).
		Only(ctx)
	if entmodel.IsNotFound(err) {
		return storageNotFoundError{cause: fmt.Errorf("OIDC subject is not active")}
	}
	if err != nil {
		return fmt.Errorf("query OIDC subject: %w", err)
	}
	userinfo.Subject = entity.ID
	if slices.Contains(scopes, oidc.ScopeProfile) {
		if entity.Name != nil {
			userinfo.Name = *entity.Name
		}
		if entity.GivenName != nil {
			userinfo.GivenName = *entity.GivenName
		}
		if entity.FamilyName != nil {
			userinfo.FamilyName = *entity.FamilyName
		}
		if entity.Nickname != nil {
			userinfo.Nickname = *entity.Nickname
		}
		if entity.PreferredUsername != nil {
			userinfo.PreferredUsername = *entity.PreferredUsername
		}
		if entity.Picture != nil {
			userinfo.Picture = *entity.Picture
		}
		if entity.Locale != nil {
			if tag, parseErr := language.Parse(*entity.Locale); parseErr == nil {
				userinfo.Locale = oidc.NewLocale(tag)
			}
		}
		userinfo.UpdatedAt = oidc.FromTime(entity.UpdateTime)
	}
	if slices.Contains(scopes, oidc.ScopeEmail) {
		email, err := storage.client.LoginIdentifier.Query().
			Where(
				loginidentifier.UserIDEQ(entity.ID),
				loginidentifier.TypeEQ(biz.LoginIdentifierEmail),
			).
			Only(ctx)
		if err != nil {
			return fmt.Errorf("query OIDC subject email: %w", err)
		}
		userinfo.Email = email.DisplayValue
		userinfo.EmailVerified = email.VerifiedTime != nil
	}
	return nil
}
