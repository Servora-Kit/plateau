package oidc

import (
	"context"
	"fmt"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcsigningkey"
	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/op"
)

// Signing keys.
func (storage *OIDCStorage) SigningKey(context.Context) (op.SigningKey, error) {
	return storage.signingPrivate, nil
}
func (storage *OIDCStorage) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{jose.RS256}, nil
}
func (storage *OIDCStorage) KeySet(ctx context.Context) ([]op.Key, error) {
	metadata, err := storage.client.OIDCSigningKey.Query().
		Where(
			oidcsigningkey.NotBeforeTimeLTE(storage.now().UTC()),
			oidcsigningkey.ExpiresTimeGT(storage.now().UTC()),
			oidcsigningkey.RevokedTimeIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query OIDC signing key metadata: %w", err)
	}
	if len(metadata) == 0 {
		return nil, storageNotFoundError{cause: fmt.Errorf("active OIDC signing key metadata not found")}
	}
	keys := make([]op.Key, 0, len(metadata))
	for _, keyMetadata := range metadata {
		var jwk jose.JSONWebKey
		if err := jwk.UnmarshalJSON([]byte(keyMetadata.PublicJwk)); err != nil {
			return nil, fmt.Errorf("decode OIDC public JWK %q: %w", keyMetadata.ID, err)
		}
		keys = append(keys, &publicKey{id: keyMetadata.ID, key: jwk.Key})
	}
	return keys, nil
}
