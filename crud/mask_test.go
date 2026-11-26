package crud_test

import (
	"slices"
	"testing"

	crudpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/crud/v1"
	"github.com/Servora-Kit/plateau/crud"
	examplev1 "github.com/Servora-Kit/plateau/crud/testdata/examplepb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestNormalizeWriteMaskBuildsImplicitPresenceMask(t *testing.T) {
	t.Parallel()

	plan := userResourcePlan()
	mask, err := plan.NormalizeWriteMask(&examplev1.User{
		DisplayName: new("Alice"),
		Nickname:    new(""),
	}, nil)
	if err != nil {
		t.Fatalf("NormalizeWriteMask: %v", err)
	}
	if !mask.Implicit() {
		t.Fatal("mask is not marked implicit")
	}
	if got, want := mask.Paths(), []string{"display_name", "nickname"}; !slices.Equal(got, want) {
		t.Fatalf("Paths() = %v, want %v", got, want)
	}
}

func TestNormalizeWriteMaskCanonicalizesExplicitPaths(t *testing.T) {
	t.Parallel()

	plan := userResourcePlan()
	mask, err := plan.NormalizeWriteMask(
		&examplev1.User{},
		&fieldmaskpb.FieldMask{Paths: []string{"nickname", "display_name", "nickname"}},
	)
	if err != nil {
		t.Fatalf("NormalizeWriteMask: %v", err)
	}
	if got, want := mask.Paths(), []string{"display_name", "nickname"}; !slices.Equal(got, want) {
		t.Fatalf("Paths() = %v, want %v", got, want)
	}
}

func TestNormalizeWriteMaskAcceptsExclusiveWildcard(t *testing.T) {
	t.Parallel()

	plan := userResourcePlan()
	mask, err := plan.NormalizeWriteMask(&examplev1.User{}, &fieldmaskpb.FieldMask{Paths: []string{"*"}})
	if err != nil {
		t.Fatalf("NormalizeWriteMask: %v", err)
	}
	if !mask.Wildcard() {
		t.Fatal("mask is not marked wildcard")
	}
}

func TestNormalizeWriteMaskRejectsInvalidPaths(t *testing.T) {
	t.Parallel()

	plan := userResourcePlan()
	for _, paths := range [][]string{
		{"missing"},
		{"*", "display_name"},
		{"displayName"},
		{" display_name"},
		{"nickname.value"},
		{""},
	} {
		_, err := plan.NormalizeWriteMask(&examplev1.User{}, &fieldmaskpb.FieldMask{Paths: paths})
		if !crudpb.IsCrudErrorReasonInvalidFieldMask(err) {
			t.Fatalf("NormalizeWriteMask(%v) error = %v, want INVALID_FIELD_MASK", paths, err)
		}
	}
}

func userResourcePlan() *crud.ResourcePlan[*examplev1.User] {
	descriptor := examplev1.UserCRUDDescriptor()
	return crud.MustBuildResourcePlan[*examplev1.User](descriptor)
}
