package data

import (
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

func TestProjectEventLegacySecurityUsesGenericProjection(t *testing.T) {
	event := cloudevents.NewEvent()
	event.SetType("servora.authz.denied.v1")
	event.SetSubject("resource:legacy")
	event.SetExtension(extAuthID, "legacy-user")
	event.SetExtension(extAuthType, "legacy-scheme")
	event.SetExtension(extErrorMessage, "legacy error")

	got := projectEvent(&event)
	want := eventProjection{
		actorID:      "legacy-user",
		actorType:    "legacy-scheme",
		targetID:     "resource:legacy",
		errorMessage: "legacy error",
	}
	if got != want {
		t.Fatalf("projection = %#v, want %#v", got, want)
	}
}

func TestProjectEventUnknownCloudEventUsesGenericProjection(t *testing.T) {
	event := cloudevents.NewEvent()
	event.SetType("platform.future.event.v1")
	event.SetSubject("resource:1")
	event.SetExtension(extAuthID, "user-1")

	got := projectEvent(&event)
	want := eventProjection{actorID: "user-1", targetID: "resource:1"}
	if got != want {
		t.Fatalf("projection = %#v, want %#v", got, want)
	}
}

func TestProjectEventNil(t *testing.T) {
	if got := projectEvent(nil); got != (eventProjection{}) {
		t.Fatalf("projection = %#v, want empty projection", got)
	}
}

func TestSuccessFromCETypeUsesGenericErrorMessage(t *testing.T) {
	for _, tt := range []struct {
		name   string
		errMsg string
		want   bool
	}{
		{name: "no error", want: true},
		{name: "error", errMsg: "denied", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := successFromCEType("servora.authz.denied.v1", tt.errMsg); got != tt.want {
				t.Fatalf("success = %v, want %v", got, tt.want)
			}
		})
	}
}
