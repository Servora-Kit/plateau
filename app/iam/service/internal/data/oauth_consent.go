package data

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	accountpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/account/v1"
	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authflow"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthaccesstoken"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthclient"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthconsent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oauthtokensession"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type oauthConsentRepository struct {
	ent *entmodel.Client
}

// NewOAuthConsentRepository constructs the Ent-backed OAuthConsentRepo adapter.
func NewOAuthConsentRepository(data *Data) (biz.OAuthConsentRepo, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("oauth consent repository: data is nil")
	}
	return &oauthConsentRepository{ent: data.ent}, nil
}

func (repo *oauthConsentRepository) GetConsentView(
	ctx context.Context,
	userID, ownerHash, requestID string,
	now time.Time,
) (*authnpb.ConsentView, error) {
	req, client, err := loadPendingOIDCRequestAndClient(ctx, repo.ent, requestID, false, now.UTC())
	if errors.Is(err, biz.ErrNotFound) {
		return nil, biz.ErrOIDCRequestInvalid
	}
	if err != nil {
		return nil, err
	}
	if client.Trusted || !verifyConsentRequestOwner(req, ownerHash) {
		return nil, biz.ErrOIDCRequestInvalid
	}

	var granted []string
	consent, err := repo.ent.OAuthConsent.Query().
		Where(
			oauthconsent.UserIDEQ(userID),
			oauthconsent.ClientIDEQ(client.ID),
		).
		Only(ctx)
	if err != nil && !entmodel.IsNotFound(err) {
		return nil, translateEntError(err)
	}
	if consent != nil {
		granted = slices.Clone(consent.GrantedScopes)
	}

	return &authnpb.ConsentView{
		RequestId:       req.ID,
		ClientId:        client.ID,
		ClientName:      client.DisplayName,
		LogoUri:         derefString(client.LogoURI),
		RequestedScopes: slices.Clone(req.Scopes),
		GrantedScopes:   granted,
	}, nil
}

func (repo *oauthConsentRepository) ApproveConsent(
	ctx context.Context,
	userID, ownerHash, requestID string,
	now time.Time,
) (string, error) {
	now = now.UTC()
	var clientID string
	err := inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		current, err := lockUser(ctx, tx, userID)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrUnauthenticated
			}
			return err
		}
		if current.Status != biz.UserStatusActive || len(current.RequiredActions) > 0 {
			return biz.ErrUnauthenticated
		}
		if _, err := ensureVerifiedEmail(ctx, tx, userID); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrUnauthenticated
			}
			return err
		}

		req, client, err := loadPendingOIDCRequestAndClient(ctx, tx.Client(), requestID, true, now)
		if errors.Is(err, biz.ErrNotFound) {
			return biz.ErrOIDCRequestInvalid
		}
		if err != nil {
			return err
		}
		if client.Trusted || !verifyConsentRequestOwner(req, ownerHash) {
			return biz.ErrOIDCRequestInvalid
		}
		clientID = client.ID

		existing, err := tx.OAuthConsent.Query().
			Where(
				oauthconsent.UserIDEQ(userID),
				oauthconsent.ClientIDEQ(client.ID),
			).
			Only(ctx)
		if entmodel.IsNotFound(err) {
			consentID, idErr := biz.NewID()
			if idErr != nil {
				return idErr
			}
			if _, err := tx.OAuthConsent.Create().
				SetID(consentID).
				SetUserID(userID).
				SetClientID(client.ID).
				SetGrantedScopes(biz.MergeScopes(nil, req.Scopes)).
				SetCreateTime(now).
				SetUpdateTime(now).
				Save(ctx); err != nil {
				return translateEntError(err)
			}
		} else if err != nil {
			return translateEntError(err)
		} else {
			if _, err := tx.OAuthConsent.UpdateOneID(existing.ID).
				SetGrantedScopes(biz.MergeScopes(existing.GrantedScopes, req.Scopes)).
				SetUpdateTime(now).
				Save(ctx); err != nil {
				return translateEntError(err)
			}
		}

		if _, err := tx.OIDCAuthorizationRequest.UpdateOneID(req.ID).
			SetSubject(userID).
			SetConsentApproved(true).
			Save(ctx); err != nil {
			return translateEntError(err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return clientID, nil
}

func (repo *oauthConsentRepository) DenyConsent(
	ctx context.Context,
	ownerHash, requestID string,
	now time.Time,
) (string, string, string, error) {
	now = now.UTC()
	var (
		redirectURI string
		state       string
		clientID    string
	)
	err := inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		req, client, err := loadPendingOIDCRequestAndClient(ctx, tx.Client(), requestID, true, now)
		if errors.Is(err, biz.ErrNotFound) {
			return biz.ErrOIDCRequestInvalid
		}
		if err != nil {
			return err
		}
		if !verifyConsentRequestOwner(req, ownerHash) {
			return biz.ErrOIDCRequestInvalid
		}

		if _, err := tx.AuthFlow.Update().
			Where(
				authflow.OidcRequestIDEQ(req.ID),
				authflow.StatusEQ(authflow.StatusActive),
			).
			SetStatus(authflow.StatusInvalidated).
			SetUpdateTime(now).
			Save(ctx); err != nil {
			return translateEntError(err)
		}
		if err := tx.OIDCAuthorizationRequest.DeleteOneID(req.ID).Exec(ctx); err != nil {
			return translateEntError(err)
		}
		redirectURI = req.RedirectURI
		state = derefString(req.State)
		clientID = client.ID
		return nil
	})
	if err != nil {
		return "", "", "", err
	}
	return redirectURI, state, clientID, nil
}

func (repo *oauthConsentRepository) ListAuthorizedApps(
	ctx context.Context,
	userID string,
) ([]*accountpb.AuthorizedApp, error) {
	consents, err := repo.ent.OAuthConsent.Query().
		Where(oauthconsent.UserIDEQ(userID)).
		Order(oauthconsent.ByUpdateTime(entsql.OrderDesc()), oauthconsent.ByID(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	if len(consents) == 0 {
		return []*accountpb.AuthorizedApp{}, nil
	}

	clientIDs := make([]string, len(consents))
	for i, c := range consents {
		clientIDs[i] = c.ClientID
	}
	clients, err := repo.ent.OAuthClient.Query().
		Where(
			oauthclient.IDIn(clientIDs...),
			oauthclient.StatusEQ(biz.ClientStatusActive),
			oauthclient.TrustedEQ(false),
		).
		All(ctx)
	if err != nil {
		return nil, translateEntError(err)
	}
	clientsByID := make(map[string]*entmodel.OAuthClient, len(clients))
	for _, c := range clients {
		clientsByID[c.ID] = c
	}

	apps := make([]*accountpb.AuthorizedApp, 0, len(consents))
	for _, consent := range consents {
		client, ok := clientsByID[consent.ClientID]
		if !ok {
			continue
		}
		apps = append(apps, &accountpb.AuthorizedApp{
			ClientId:      client.ID,
			ClientName:    client.DisplayName,
			Description:   derefString(client.Description),
			LogoUri:       derefString(client.LogoURI),
			GrantedScopes: slices.Clone(consent.GrantedScopes),
			CreateTime:    timestamppb.New(consent.CreateTime.UTC()),
			UpdateTime:    timestamppb.New(consent.UpdateTime.UTC()),
		})
	}
	return apps, nil
}

func (repo *oauthConsentRepository) RevokeAuthorizedApp(
	ctx context.Context,
	userID, clientID string,
	now time.Time,
) error {
	now = now.UTC()
	return inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		if _, err := tx.OAuthConsent.Delete().
			Where(
				oauthconsent.UserIDEQ(userID),
				oauthconsent.ClientIDEQ(clientID),
			).
			Exec(ctx); err != nil {
			return translateEntError(err)
		}
		if err := revokeOAuthSessions(ctx, tx, now, oauthtokensession.UserIDEQ(userID), oauthtokensession.ClientIDEQ(clientID)); err != nil {
			return err
		}
		if _, err := tx.OAuthAccessToken.Update().
			Where(
				oauthaccesstoken.ActorTypeEQ(oauthaccesstoken.ActorTypeHuman),
				oauthaccesstoken.SubjectEQ(userID),
				oauthaccesstoken.ClientIDEQ(clientID),
				oauthaccesstoken.RevokedTimeIsNil(),
			).
			SetRevokedTime(now).
			Save(ctx); err != nil {
			return translateEntError(err)
		}
		return nil
	})
}

func verifyConsentRequestOwner(req *entmodel.OIDCAuthorizationRequest, ownerHash string) bool {
	if ownerHash == "" || req.ConsentOwnerHash == nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(ownerHash), []byte(*req.ConsentOwnerHash)) == 1
}
