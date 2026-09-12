package jwt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	jwtpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/jwt/v1"
)

// NewJWKS caches a trusted remote key set until cleanup or context cancellation.
func NewJWKS(ctx context.Context, config *jwtpb.JWKS) (*Verifier, func(), error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("jwt: lifecycle context is nil")
	}
	uri, err := url.Parse(config.GetUri())
	if err != nil || uri.Host == "" || uri.User != nil || uri.Fragment != "" {
		return nil, nil, fmt.Errorf("jwt: invalid JWKS URI")
	}
	loopback := uri.Hostname() == "localhost" || uri.Hostname() == "127.0.0.1" || uri.Hostname() == "::1"
	if uri.Scheme != "https" && (uri.Scheme != "http" || !loopback) {
		return nil, nil, fmt.Errorf("jwt: JWKS requires HTTPS except on loopback")
	}
	lifecycle, cancel := context.WithCancel(ctx)
	source, err := keyfunc.NewDefaultOverrideCtx(lifecycle, []string{uri.String()}, keyfunc.Override{
		HTTPTimeout:               5 * time.Second,
		Client:                    &http.Client{Timeout: 5 * time.Second},
		NoErrorReturnFirstHTTPReq: new(false),
		RateLimitWaitMax:          time.Millisecond,
	})
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("jwt: initialize JWKS: %w", err)
	}
	verifier, err := NewWithKeySource(source.KeyfuncCtx)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return verifier, cancel, nil
}
