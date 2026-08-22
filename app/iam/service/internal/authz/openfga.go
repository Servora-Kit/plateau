package authz

import (
	"fmt"

	"github.com/Servora-Kit/plateau/security"
	openfgaauthz "github.com/Servora-Kit/plateau/security/authz/openfga"
	fgaclient "github.com/openfga/go-sdk/client"
)

// NewOpenFGAAuthorizer binds IAM human Actors to the IAM user type in Plateau's OpenFGA model.
func NewOpenFGAAuthorizer(client *fgaclient.OpenFgaClient) (*openfgaauthz.Authorizer, error) {
	if client == nil {
		return nil, fmt.Errorf("IAM OpenFGA authorizer: client is nil")
	}
	return openfgaauthz.New(client, subject)
}

func subject(actor security.Actor) (string, error) {
	if actor.Type != security.ActorTypeHuman || actor.ID == "" {
		return "", fmt.Errorf("IAM OpenFGA subject requires a human Actor")
	}
	return "user:" + actor.ID, nil
}
