package session

import (
	"errors"
	"fmt"
	"net/http"
)

func cookieCredential(rawCookieHeader, name string) (string, error) {
	if rawCookieHeader == "" {
		return "", ErrMissingCredentials
	}
	request := &http.Request{Header: http.Header{"Cookie": []string{rawCookieHeader}}}
	cookie, err := request.Cookie(name)
	if errors.Is(err, http.ErrNoCookie) {
		return "", ErrMissingCredentials
	}
	if err != nil {
		return "", fmt.Errorf("%w: malformed Cookie header", ErrInvalidCredentials)
	}
	if cookie.Value == "" {
		return "", ErrMissingCredentials
	}
	return cookie.Value, nil
}
