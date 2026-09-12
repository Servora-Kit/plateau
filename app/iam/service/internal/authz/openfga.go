package authz

import (
	"fmt"

	"github.com/Servora-Kit/plateau/security"
	openfgaauthz "github.com/Servora-Kit/plateau/security/authz/openfga"
	fgaclient "github.com/openfga/go-sdk/client"
)

// NewOpenFGAAuthorizer binds verified Actors to the shared identity types.
func NewOpenFGAAuthorizer(client *fgaclient.OpenFgaClient) (*openfgaauthz.Authorizer, error) {
	if client == nil {
		return nil, fmt.Errorf("IAM OpenFGA authorizer: client is nil")
	}
	return openfgaauthz.New(client, subject)
}

func subject(actor security.Actor) (string, error) {
	if !actor.Valid() {
		return "", fmt.Errorf("IAM OpenFGA subject requires a valid Actor")
	}
	switch actor.Type {
	case security.ActorTypeHuman:
		return "user:" + actor.ID, nil
	case security.ActorTypeService:
		return "service:" + actor.ID, nil
	default:
		return "", fmt.Errorf("IAM OpenFGA subject requires an authenticated Actor")
	}
}
