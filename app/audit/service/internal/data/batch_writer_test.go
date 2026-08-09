package data

import (
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	authnauditpb "github.com/Servora-Kit/servora/api/gen/go/servora/authn/audit/v1"
	authzauditpb "github.com/Servora-Kit/servora/api/gen/go/servora/authz/audit/v1"
	obsaudit "github.com/Servora-Kit/servora/obs/audit"
	"google.golang.org/protobuf/proto"
)

func TestProjectEventTypedSecurityPayloads(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		payload  proto.Message
		subject  string
		want     eventProjection
	}{
		{
			name:     "authn success",
			typeName: "servora.authn.success.v1",
			payload:  &authnauditpb.AuthnSuccess{Scheme: "oidc", Subject: "user:alice"},
			want:     eventProjection{actorID: "user:alice", actorType: "oidc"},
		},
		{
			name:     "authn failure",
			typeName: "servora.authn.failure.v1",
			payload:  &authnauditpb.AuthnFailure{Reason: authnauditpb.AuthnFailureReason_AUTHN_FAILURE_REASON_UNAVAILABLE, Code: 503},
			want:     eventProjection{errorCode: "AUTHN_FAILURE_REASON_UNAVAILABLE"},
		},
		{
			name:     "authz denied",
			typeName: "servora.authz.denied.v1",
			payload: &authzauditpb.AuthzDecision{
				Decision:     authzauditpb.AuthzDecision_DECISION_DENIED,
				Subject:      "user:alice",
				Action:       "read",
				ResourceType: "document",
				ResourceId:   "doc-1",
				Reason:       authzauditpb.AuthzDecision_REASON_DENIED,
				Code:         403,
			},
			subject: "document:doc-1",
			want:    eventProjection{actorID: "user:alice", targetType: "document", targetID: "doc-1", errorCode: "REASON_DENIED"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := cloudevents.NewEvent()
			event.SetType(tt.typeName)
			event.SetSubject(tt.subject)
			if err := obsaudit.SetProtoData(&event, tt.payload); err != nil {
				t.Fatal(err)
			}
			if got := projectEvent(&event); got != tt.want {
				t.Fatalf("projection = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestProjectEventMalformedTypedPayloadDoesNotUseLegacyExtensions(t *testing.T) {
	event := cloudevents.NewEvent()
	event.SetType("servora.authz.denied.v1")
	event.SetSubject("resource:legacy")
	event.SetExtension(extAuthID, "legacy-user")
	event.SetExtension(extAuthType, "legacy-scheme")
	event.SetExtension(extErrorMessage, "legacy error")
	event.SetDataContentType("application/protobuf")
	if err := event.SetData(cloudevents.ApplicationJSON, []byte("not-protobuf")); err != nil {
		t.Fatal(err)
	}

	if got := projectEvent(&event); got != (eventProjection{}) {
		t.Fatalf("projection = %#v, want empty projection", got)
	}
}

func TestProjectEventLegacyExtensionsFallback(t *testing.T) {
	event := cloudevents.NewEvent()
	event.SetType("servora.audit.rpc.v1")
	event.SetSubject("resource:1")
	event.SetExtension(extAuthID, "legacy-user")
	event.SetExtension(extAuthType, "legacy-scheme")
	event.SetExtension(extErrorMessage, "legacy error")

	got := projectEvent(&event)
	want := eventProjection{
		actorID:      "legacy-user",
		actorType:    "legacy-scheme",
		targetID:     "resource:1",
		errorMessage: "legacy error",
	}
	if got != want {
		t.Fatalf("projection = %#v, want %#v", got, want)
	}
}
