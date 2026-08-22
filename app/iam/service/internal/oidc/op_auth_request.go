package oidc

import (
	"slices"
	"time"

	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

type authorizationRequest struct {
	entity              *entmodel.OIDCAuthorizationRequest
	authorizationCodeID string
}

func (request *authorizationRequest) GetID() string { return request.entity.ID }

func (request *authorizationRequest) GetACR() string { return "" }

func (request *authorizationRequest) GetAMR() []string {
	if !request.Done() {
		return nil
	}
	return []string{passwordAMR}
}

func (request *authorizationRequest) GetAudience() []string {
	return []string{request.entity.ClientID}
}

func (request *authorizationRequest) GetAuthTime() time.Time {
	if request.entity.AuthTime == nil {
		return time.Time{}
	}
	return *request.entity.AuthTime
}

func (request *authorizationRequest) GetClientID() string { return request.entity.ClientID }

func (request *authorizationRequest) GetCodeChallenge() *oidc.CodeChallenge {
	if request.entity.PkceChallenge == "" {
		return nil
	}
	return &oidc.CodeChallenge{
		Challenge: request.entity.PkceChallenge,
		Method:    oidc.CodeChallengeMethod(request.entity.PkceChallengeMethod),
	}
}

func (request *authorizationRequest) GetNonce() string {
	if request.entity.Nonce == nil {
		return ""
	}
	return *request.entity.Nonce
}

func (request *authorizationRequest) GetRedirectURI() string { return request.entity.RedirectURI }

func (request *authorizationRequest) GetResponseType() oidc.ResponseType {
	return oidc.ResponseType(request.entity.ResponseType)
}

func (request *authorizationRequest) GetResponseMode() oidc.ResponseMode {
	return oidc.ResponseMode(request.entity.ResponseMode)
}

func (request *authorizationRequest) GetScopes() []string {
	return slices.Clone(request.entity.Scopes)
}

func (request *authorizationRequest) GetState() string {
	if request.entity.State == nil {
		return ""
	}
	return *request.entity.State
}

func (request *authorizationRequest) GetSubject() string {
	if request.entity.Subject == nil {
		return ""
	}
	return *request.entity.Subject
}

func (request *authorizationRequest) Done() bool {
	return request.entity.Done && request.entity.Subject != nil && request.entity.AuthTime != nil
}
