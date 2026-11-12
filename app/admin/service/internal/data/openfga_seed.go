package data

import (
	"context"
	"fmt"

	"github.com/Servora-Kit/plateau/app/admin/service/internal/biz"
	fgaclient "github.com/openfga/go-sdk/client"
)

type adminSeedRepo struct {
	fga *fgaclient.OpenFgaClient
}

// NewAdminSeedRepo constructs biz.AdminSeedRepo using the official OpenFGA SDK client.
func NewAdminSeedRepo(data *Data) (biz.AdminSeedRepo, error) {
	if data == nil || data.fga == nil {
		return nil, fmt.Errorf("admin seed repo: OpenFGA client is nil")
	}
	return &adminSeedRepo{fga: data.fga}, nil
}

func (r *adminSeedRepo) EnsureAdminTuple(ctx context.Context, userID string) error {
	subject := "user:" + userID
	object := biz.AdminResourceType + ":" + biz.AdminResourceID

	checkResp, err := r.fga.Check(ctx).Body(fgaclient.ClientCheckRequest{
		User:     subject,
		Relation: biz.RelationAdmin,
		Object:   object,
	}).Execute()
	if err != nil {
		return fmt.Errorf("check OpenFGA tuple: %w", err)
	}
	if checkResp.GetAllowed() {
		return nil
	}

	_, writeErr := r.fga.Write(ctx).Body(fgaclient.ClientWriteRequest{
		Writes: []fgaclient.ClientTupleKey{
			{
				User:     subject,
				Relation: biz.RelationAdmin,
				Object:   object,
			},
		},
	}).Execute()
	if writeErr != nil {
		// If a concurrent replica wrote the same tuple first, re-check before failing.
		recheck, recheckErr := r.fga.Check(ctx).Body(fgaclient.ClientCheckRequest{
			User:     subject,
			Relation: biz.RelationAdmin,
			Object:   object,
		}).Execute()
		if recheckErr == nil && recheck.GetAllowed() {
			return nil
		}
		return fmt.Errorf("write OpenFGA admin tuple: %w", writeErr)
	}
	return nil
}
