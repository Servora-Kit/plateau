package data

import (
	"context"
	"fmt"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
)

type initialUserCreator struct{ ent *entmodel.Client }

// NewInitialUserCreator provides the startup-only active User creation path.
func NewInitialUserCreator(data *Data) (biz.InitialUserCreator, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("initial user creator: data is nil")
	}
	return &initialUserCreator{ent: data.ent}, nil
}

func (creator *initialUserCreator) CreateInitialUser(ctx context.Context, userID, email, canonical, passwordHash string, now time.Time) error {
	if userID == "" || email == "" || canonical == "" || passwordHash == "" {
		return fmt.Errorf("initial user creator: required identity fields are missing")
	}
	if now.IsZero() {
		now = time.Now()
	}
	resource := &userpb.User{UserId: userID, Email: &email}
	return createUserAggregate(ctx, creator.ent, resource, passwordHash, canonical, biz.UserStatusActive, &now, now)
}
