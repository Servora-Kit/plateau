package data

import (
	openfgaconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	fgapkg "github.com/Servora-Kit/plateau/infra/openfga"
	fgaclient "github.com/openfga/go-sdk/client"
)

// NewFGAClient constructs IAM's shared OpenFGA SDK client from Plateau configuration.
func NewFGAClient(config *openfgaconfpb.OpenFGA) (*fgaclient.OpenFgaClient, error) {
	return fgapkg.New(config)
}
