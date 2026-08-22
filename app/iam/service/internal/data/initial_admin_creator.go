package data

import (
	"context"
	"fmt"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
)

type initialAdminCreator struct{ data *Data }

// NewInitialAdminCreator provides the startup-only active User creation path.
func NewInitialAdminCreator(data *Data) (biz.InitialAdminCreator, error) {
	if data == nil {
		return nil, fmt.Errorf("initial admin creator: data is nil")
	}
	return &initialAdminCreator{data: data}, nil
}

func (creator *initialAdminCreator) CreateInitialAdmin(ctx context.Context, userID, email, canonical, passwordHash string, now time.Time) error {
	if userID == "" || email == "" || canonical == "" || passwordHash == "" {
		return fmt.Errorf("initial admin creator: required identity fields are missing")
	}
	if now.IsZero() {
		now = time.Now()
	}
	resource := &userpb.User{UserId: userID, Email: &email}
	return createUserAggregate(ctx, creator.data, resource, passwordHash, canonical, biz.UserStatusActive, &now, now)
}
