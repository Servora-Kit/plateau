package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	oidcv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/proto"
)

// NewServiceTokenMiddleware constructs a Kratos client middleware that attaches
// OAuth 2.1 client_credentials Bearer tokens using a process-shared ReuseTokenSource,
// while honoring per-request context cancellation immediately.
func NewServiceTokenMiddleware(config *oidcv1.OIDCClient, opts ...Option) (middleware.Middleware, func(), error) {
	if config == nil {
		return nil, nil, fmt.Errorf("oidc: config is nil")
	}
	cfg := proto.Clone(config).(*oidcv1.OIDCClient)
	if err := cfg.Apply(); err != nil {
		return nil, nil, fmt.Errorf("oidc: config: %w", err)
	}
	issuerURL, err := parseEndpointURL(cfg.GetIssuer())
	if err != nil {
		return nil, nil, fmt.Errorf("oidc: invalid issuer: %w", err)
	}
	if err := cfg.GetHttpTimeout().CheckValid(); err != nil || cfg.GetHttpTimeout().AsDuration() <= 0 {
		return nil, nil, fmt.Errorf("oidc: http_timeout must be a positive duration")
	}

	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	timeout := cfg.GetHttpTimeout().AsDuration()
	httpClient := resolveHTTPClient(o.httpClient, http.Client{Timeout: timeout})

	appCtx, cancel := context.WithCancel(context.Background())
	tokenCtx := context.WithValue(appCtx, oauth2.HTTPClient, httpClient)

	cc := clientcredentials.Config{
		ClientID:     cfg.GetClientId(),
		ClientSecret: cfg.GetClientSecret(),
		TokenURL:     strings.TrimRight(issuerURL.String(), "/") + "/oauth/token",
		AuthStyle:    oauth2.AuthStyleInHeader,
	}
	source := cc.TokenSource(tokenCtx)

	var sf singleflight.Group

	mw := func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if ctx == nil {
				return nil, fmt.Errorf("%w: context is nil", ErrTokenSourceUnavailable)
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			tr, ok := transport.FromClientContext(ctx)
			if !ok || tr == nil {
				return nil, fmt.Errorf("%w: client transport is missing", ErrTokenSourceUnavailable)
			}

			ch := sf.DoChan("token", func() (any, error) {
				tok, err := source.Token()
				if err != nil {
					return nil, sanitizeTokenError(err)
				}
				if tok == nil || tok.AccessToken == "" {
					return nil, fmt.Errorf("%w: empty access token", ErrTokenSourceUnavailable)
				}
				return tok.AccessToken, nil
			})

			var accessToken string
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case res := <-ch:
				if res.Err != nil {
					if ctxErr := ctx.Err(); ctxErr != nil {
						return nil, ctxErr
					}
					return nil, res.Err
				}
				accessToken = res.Val.(string)
			}

			tr.RequestHeader().Set("Authorization", "Bearer "+accessToken)
			return handler(ctx, req)
		}
	}

	return mw, cancel, nil
}

func sanitizeTokenError(err error) error {
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) && retrieveErr.Response != nil {
		return fmt.Errorf("%w: token endpoint returned status %d", ErrTokenSourceUnavailable, retrieveErr.Response.StatusCode)
	}
	return fmt.Errorf("%w: token request failed", ErrTokenSourceUnavailable)
}
