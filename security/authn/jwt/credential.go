package jwt

import "strings"

func bearerToken(authorization string) (string, error) {
	if authorization == "" {
		return "", ErrMissingCredentials
	}
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", ErrMalformedCredentials
	}
	return parts[1], nil
}
