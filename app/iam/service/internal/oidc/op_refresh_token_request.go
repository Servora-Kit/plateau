package oidc

import (
	"slices"
	"time"
)

type refreshTokenRequest struct {
	tokenID        string
	tokenSessionID string
	clientID       string
	subject        string
	scopes         []string
	authTime       time.Time
	amr            []string
}

func (request *refreshTokenRequest) GetAMR() []string { return slices.Clone(request.amr) }

func (request *refreshTokenRequest) GetAudience() []string { return []string{request.clientID} }

func (request *refreshTokenRequest) GetAuthTime() time.Time { return request.authTime }

func (request *refreshTokenRequest) GetClientID() string { return request.clientID }

func (request *refreshTokenRequest) GetScopes() []string { return slices.Clone(request.scopes) }

func (request *refreshTokenRequest) GetSubject() string { return request.subject }

func (request *refreshTokenRequest) SetCurrentScopes(scopes []string) {
	request.scopes = slices.Clone(scopes)
}
