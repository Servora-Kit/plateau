// Package oidc provides Plateau's shared OIDC Relying Party and
// client_credentials service-token middleware.
package oidc

import "errors"

var (
	// ErrHandshakeInvalid indicates a missing, expired, tampered, or mismatched
	// browser OIDC login handshake or rejected token/claims exchange.
	ErrHandshakeInvalid = errors.New("oidc: authorization handshake is invalid")

	// ErrAccessDenied indicates that the user explicitly cancelled or denied the
	// upstream OIDC authorization request on a verified handshake.
	ErrAccessDenied = errors.New("oidc: authorization access denied")

	// ErrProviderUnavailable indicates that upstream OIDC discovery or token/userinfo
	// transport could not be reached.
	ErrProviderUnavailable = errors.New("oidc: upstream provider is unavailable")

	// ErrTokenSourceUnavailable indicates that acquiring a client_credentials
	// service token failed.
	ErrTokenSourceUnavailable = errors.New("oidc: service token source is unavailable")
)
