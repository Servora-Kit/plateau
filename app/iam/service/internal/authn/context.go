package authn

import (
	"context"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/iam/session/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
)

type identityContextKey struct{}

type identity struct {
	user    *userpb.User
	session *sessionpb.Session
}

func withIdentity(ctx context.Context, value *identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, value)
}

// From returns the IAM User and Session resolved by session authentication.
func From(ctx context.Context) (*userpb.User, *sessionpb.Session, error) {
	if ctx == nil {
		return nil, nil, biz.ErrUnauthenticated
	}
	value, ok := ctx.Value(identityContextKey{}).(*identity)
	if !ok || value == nil || value.user == nil || value.session == nil {
		return nil, nil, biz.ErrUnauthenticated
	}
	return value.user, value.session, nil
}
