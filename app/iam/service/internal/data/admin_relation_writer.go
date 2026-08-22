package data

import (
	"context"
	"fmt"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	fgaclient "github.com/openfga/go-sdk/client"
)

type adminRelationWriter struct{ data *Data }

// NewAdminRelationWriter provides idempotent bootstrap OpenFGA tuple writes.
func NewAdminRelationWriter(data *Data) (biz.AdminRelationWriter, error) {
	if data == nil || data.fga == nil {
		return nil, fmt.Errorf("admin relation writer: OpenFGA client is nil")
	}
	return &adminRelationWriter{data: data}, nil
}

func (writer *adminRelationWriter) EnsurePlatformAdmin(ctx context.Context, userID string) error {
	if ctx == nil {
		return fmt.Errorf("admin relation writer: context is nil")
	}
	if userID == "" {
		return fmt.Errorf("admin relation writer: user ID is empty")
	}
	_, err := writer.data.fga.Write(ctx).
		Body(fgaclient.ClientWriteRequest{Writes: []fgaclient.ClientTupleKey{{
			User: "user:" + userID, Relation: "admin", Object: "plateau:default",
		}}}).
		Options(fgaclient.ClientWriteOptions{Conflict: fgaclient.ClientWriteConflictOptions{
			OnDuplicateWrites: fgaclient.CLIENT_WRITE_REQUEST_ON_DUPLICATE_WRITES_IGNORE,
		}}).
		Execute()
	if err != nil {
		return fmt.Errorf("write platform admin tuple: %w", err)
	}
	return nil
}
