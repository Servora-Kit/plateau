package oidc

import (
	"context"
	"fmt"
	"time"

	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcsigningkey"
	"github.com/google/uuid"
)

// OIDCStorage adapts IAM persistence to ZITADEL's OpenID Provider interfaces.
type OIDCStorage struct {
	tokens                biz.OAuthRepo
	serviceAccessTokenTTL time.Duration
	client                *entmodel.Client
	now                   func() time.Time
	signingPrivate        *signingKey
	signingPublicJWK      string
}

func NewOIDCStorage(client *entmodel.Client, config *oidcconfpb.OIDC, tokens biz.OAuthRepo) (*OIDCStorage, error) {
	if client == nil {
		return nil, fmt.Errorf("OIDC Ent client is nil")
	}
	if config == nil || tokens == nil {
		return nil, fmt.Errorf("OIDC configuration is nil")
	}
	if err := config.Apply(); err != nil {
		return nil, fmt.Errorf("OIDC storage config: %w", err)
	}
	privateKey, keyID, publicJWK, err := loadSigningKey(config.GetSigningKeyPath())
	if err != nil {
		return nil, err
	}
	ttlConfig := config.GetServiceAccessTokenTtl()
	if err := ttlConfig.CheckValid(); err != nil {
		return nil, err
	}
	ttl := ttlConfig.AsDuration()
	if ttl <= 0 {
		return nil, fmt.Errorf("OIDC service access token TTL must be positive")
	}
	return &OIDCStorage{
		tokens: tokens, serviceAccessTokenTTL: ttl,
		client:           client,
		now:              time.Now,
		signingPrivate:   &signingKey{id: keyID, key: privateKey},
		signingPublicJWK: publicJWK,
	}, nil
}

func (storage *OIDCStorage) Health(ctx context.Context) error {
	now := storage.now().UTC()
	exists, err := storage.client.OIDCSigningKey.Query().
		Where(
			oidcsigningkey.IDEQ(storage.signingPrivate.id),
			oidcsigningkey.NotBeforeTimeLTE(now),
			oidcsigningkey.ExpiresTimeGT(now),
			oidcsigningkey.RevokedTimeIsNil(),
		).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check OIDC signing key metadata: %w", err)
	}
	if !exists {
		return fmt.Errorf("active OIDC signing key metadata not found")
	}
	return nil
}

func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return id.String(), nil
}
