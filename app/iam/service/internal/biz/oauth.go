package biz

import (
	"context"
	"errors"
	"time"
)

var ErrOAuthGrantInvalid = errors.New("OAuth grant is inactive or inconsistent")

// OAuthGrant is the verified protocol input rechecked within the issuance transaction.
type OAuthGrant struct {
	CodeID, RefreshTokenID, TokenSessionID string
	Subject, ClientID, LoginID             string
	Scopes, Audiences, AMR                 []string
	AuthTime                               time.Time
	Service                                bool
}

// OAuthTokenIssue contains application-generated IDs, hash and issuance deadlines.
type OAuthTokenIssue struct {
	AccessID, RefreshID, RefreshHash            string
	IssuedAt, AccessExpiresAt, RefreshExpiresAt time.Time
}

// OAuthRepo serializes user issuance and revocation with identity changes.
type OAuthRepo interface {
	Issue(context.Context, OAuthGrant, OAuthTokenIssue) error
	RevokeSession(context.Context, string, time.Time) error
	RevokeClient(context.Context, string, string, time.Time) error
	RevokeToken(context.Context, string, string, string, time.Time) error
}
