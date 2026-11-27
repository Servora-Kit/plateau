package oidc

import (
	"net/http"
	"net/url"
)

type issRedirectWriter struct {
	http.ResponseWriter
	issuer      string
	wroteHeader bool
}

func (w *issRedirectWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *issRedirectWriter) WriteHeader(statusCode int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if statusCode >= http.StatusMultipleChoices && statusCode < http.StatusBadRequest {
			if loc := w.Header().Get("Location"); loc != "" {
				if updated, ok := injectRedirectIssuer(loc, w.issuer); ok {
					w.Header().Set("Location", updated)
				}
			}
		}
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func injectRedirectIssuer(location, issuer string) (string, bool) {
	parsed, err := url.Parse(location)
	if err != nil || !parsed.IsAbs() {
		return "", false
	}
	query := parsed.Query()
	if query.Get("code") == "" && query.Get("error") == "" {
		return "", false
	}
	if !query.Has("iss") {
		query.Set("iss", issuer)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), true
}
