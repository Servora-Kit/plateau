package session

import (
	"context"
	"errors"
	"testing"

	security "github.com/Servora-Kit/plateau/security"
)

type testIdentity struct{ userID string }
type localIdentityKey struct{}

func TestAuthenticatorResolvesAndBindsIdentity(t *testing.T) {
	authenticator, err := New("__Host-session", func(_ context.Context, credential string) (testIdentity, error) {
		if credential != "secret" {
			return testIdentity{}, ErrInvalidCredentials
		}
		return testIdentity{userID: "user-1"}, nil
	}, func(identity testIdentity) (security.Actor, error) {
		return security.Actor{Type: security.ActorTypeHuman, ID: identity.userID}, nil
	}, func(ctx context.Context, identity testIdentity) context.Context {
		return context.WithValue(ctx, localIdentityKey{}, identity)
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	trusted, err := authenticator.authenticate(context.Background(), "secret")
	if err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}
	actor, ok := security.ActorFrom(trusted)
	if !ok || actor != (security.Actor{Type: security.ActorTypeHuman, ID: "user-1"}) {
		t.Fatalf("Actor = %+v, present = %t", actor, ok)
	}
	if got := trusted.Value(localIdentityKey{}); got != (testIdentity{userID: "user-1"}) {
		t.Fatalf("local identity = %+v", got)
	}
}

func TestAuthenticatorPreservesResolverClassification(t *testing.T) {
	cause := errors.New("session store offline")
	authenticator, err := New("__Host-session", func(context.Context, string) (testIdentity, error) {
		return testIdentity{}, errors.Join(ErrDependencyUnavailable, cause)
	}, func(identity testIdentity) (security.Actor, error) {
		return security.Actor{Type: security.ActorTypeHuman, ID: identity.userID}, nil
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := authenticator.authenticate(context.Background(), "secret"); !errors.Is(err, ErrDependencyUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("authenticate() error = %v", err)
	}
}
