package oidc

import (
	"context"
	"errors"
	"fmt"
	"time"

	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/iam/oidc/conf/v1"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcsigningkey"
	"github.com/google/uuid"
)

// OIDCStorage adapts IAM persistence to ZITADEL's OpenID Provider interfaces.
type OIDCStorage struct {
	client           *entmodel.Client
	now              func() time.Time
	signingPrivate   *signingKey
	signingPublicJWK string
}

func NewOIDCStorage(client *entmodel.Client, config *oidcconfpb.OIDC) (*OIDCStorage, error) {
	if client == nil {
		return nil, fmt.Errorf("OIDC Ent client is nil")
	}
	if config == nil {
		return nil, fmt.Errorf("OIDC configuration is nil")
	}
	privateKey, keyID, publicJWK, err := loadSigningKey(config.GetSigningKeyPath())
	if err != nil {
		return nil, err
	}
	return &OIDCStorage{
		client:           client,
		now:              time.Now,
		signingPrivate:   &signingKey{id: keyID, key: privateKey},
		signingPublicJWK: publicJWK,
	}, nil
}

func (storage *OIDCStorage) inTx(ctx context.Context, fn func(*entmodel.Tx) error) error {
	if fn == nil {
		return fmt.Errorf("OIDC transaction function is nil")
	}
	tx, err := storage.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin OIDC transaction: %w", err)
	}
	defer func() {
		if panicValue := recover(); panicValue != nil {
			_ = tx.Rollback()
			panic(panicValue)
		}
	}()
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit OIDC transaction: %w", err)
	}
	return nil
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
