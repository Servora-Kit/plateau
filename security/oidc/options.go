package oidc

import "net/http"

// Option customizes RelyingParty or ServiceTokenMiddleware construction.
type Option func(*options)

type options struct {
	httpClient *http.Client
}

// WithHTTPClient overrides the underlying HTTP client transport; if Timeout is
// zero on the supplied client, the configured http_timeout is applied on a shallow copy.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		o.httpClient = client
	}
}

func resolveHTTPClient(custom *http.Client, timeout http.Client) *http.Client {
	if custom == nil {
		return &timeout
	}
	cloned := *custom
	if cloned.Timeout <= 0 {
		cloned.Timeout = timeout.Timeout
	}
	return &cloned
}
