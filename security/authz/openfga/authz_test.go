package openfga

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	security "github.com/Servora-Kit/plateau/security"
	fgasdk "github.com/openfga/go-sdk"
	fgaclient "github.com/openfga/go-sdk/client"
)

const (
	testStoreID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	testModelID = "01ARZ3NDEKTSV4RRFFQ69G5FAW"
)

func sdkClient(t *testing.T, handler http.HandlerFunc) (*fgaclient.OpenFgaClient, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		handler(response, request)
	}))
	t.Cleanup(server.Close)
	client, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{
		ApiUrl:               server.URL,
		StoreId:              testStoreID,
		AuthorizationModelId: testModelID,
		RetryParams:          &fgasdk.RetryParams{MaxRetry: 1, MinWaitInMs: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, &calls
}

func directSubject(actor security.Actor) (string, error) {
	return "principal:" + actor.ID, nil
}

func request() Request {
	return Request{
		Actor:        security.Actor{Type: security.ActorTypeHuman, ID: "alice"},
		Action:       "reader",
		ResourceType: "document",
		ResourceID:   "doc-1",
	}
}

func performCheck(ctx context.Context, authorizer *Authorizer, value Request) (bool, error) {
	return authorizer.Check(ctx, value.Actor, value.Action, value.ResourceType, value.ResourceID)
}
func TestNewValidation(t *testing.T) {
	if got, err := New(nil, directSubject); err == nil || got != nil {
		t.Fatalf("authorizer = %v, error = %v", got, err)
	}
	if got, err := New(&fgaclient.OpenFgaClient{}, directSubject); err == nil || got != nil {
		t.Fatalf("zero SDK client authorizer=%v error=%v", got, err)
	}
	client, _ := sdkClient(t, func(http.ResponseWriter, *http.Request) {})
	if got, err := New(client, nil); err == nil || got != nil {
		t.Fatalf("nil subject mapper authorizer=%v error=%v", got, err)
	}
}

func TestDirectRequestValidationDoesNotCallSDK(t *testing.T) {
	client, calls := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(`{"allowed":true}`)) })
	authorizer, err := New(client, directSubject)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []Request{
		{Action: "read", ResourceType: "document", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, ResourceType: "document", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: "read", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: "read", ResourceType: "document"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "alice#member"}, Action: "read", ResourceType: "document", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: "read#member", ResourceType: "document", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: strings.Repeat("a", 51), ResourceType: "document", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: "read", ResourceType: "document@tenant", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: "read", ResourceType: "document", ResourceID: "org:1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: "read", ResourceType: "document", ResourceID: "1#reader"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "*"}, Action: "read", ResourceType: "document", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "alice\x00"}, Action: "read", ResourceType: "document", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: string([]byte{0xff})}, Action: "read", ResourceType: "document", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: "read\x1b", ResourceType: "document", ResourceID: "1"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: "read", ResourceType: "document", ResourceID: "doc\x00"},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: "1"}, Action: "read", ResourceType: "document", ResourceID: string([]byte{0xff})},
		{Actor: security.Actor{Type: security.ActorTypeHuman, ID: strings.Repeat("a", 513)}, Action: "read", ResourceType: "document", ResourceID: "1"},
	}
	for _, checkRequest := range invalid {
		if _, err := performCheck(context.Background(), authorizer, checkRequest); err == nil || !stderrors.Is(err, ErrInvalidInput) {
			t.Fatalf("Check(%#v) error = %v", checkRequest, err)
		}
	}
	if results, err := authorizer.BatchCheck(context.Background(), invalid[:1]); err == nil || results != nil || !stderrors.Is(err, ErrInvalidInput) {
		t.Fatalf("BatchCheck results = %v, error = %v", results, err)
	}
	if ids, err := authorizer.ListAllowed(context.Background(), security.Actor{}, "read", "document"); err == nil || ids != nil || !stderrors.Is(err, ErrInvalidInput) {
		t.Fatalf("ListAllowed ids = %v, error = %v", ids, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("SDK calls = %d", calls.Load())
	}
}

func TestAnonymousIsUnauthenticated(t *testing.T) {
	client, calls := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(`{"allowed":true}`)) })
	authorizer, _ := New(client, directSubject)
	checkRequest := request()
	checkRequest.Actor = security.Actor{Type: security.ActorTypeAnonymous}
	_, err := performCheck(context.Background(), authorizer, checkRequest)
	if !stderrors.Is(err, ErrUnauthenticated) || calls.Load() != 0 {
		t.Fatalf("error=%v calls=%d", err, calls.Load())
	}
}

func TestCheckMapsActorRequest(t *testing.T) {
	body := make(chan string, 1)
	client, _ := sdkClient(t, func(response http.ResponseWriter, request *http.Request) {
		data, _ := io.ReadAll(request.Body)
		body <- string(data)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"allowed":true}`))
	})
	authorizer, _ := New(client, directSubject)
	allowed, err := performCheck(context.Background(), authorizer, request())
	if err != nil || !allowed {
		t.Fatalf("allowed = %v, error = %v", allowed, err)
	}
	gotBody := <-body
	for _, want := range []string{`"user":"principal:alice"`, `"relation":"reader"`, `"object":"document:doc-1"`} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("body missing %s: %s", want, gotBody)
		}
	}
}

func TestCheckPreservesProviderErrorTypes(t *testing.T) {
	tests := []struct {
		name   string
		status int
		check  func(error) bool
	}{
		{name: "rate limit", status: http.StatusTooManyRequests, check: func(err error) bool {
			var target fgasdk.FgaApiRateLimitExceededError
			return stderrors.As(err, &target)
		}},
		{name: "internal", status: http.StatusServiceUnavailable, check: func(err error) bool {
			var target fgasdk.FgaApiInternalError
			return stderrors.As(err, &target)
		}},
		{name: "authentication", status: http.StatusUnauthorized, check: func(err error) bool {
			var target fgasdk.FgaApiAuthenticationError
			return stderrors.As(err, &target)
		}},
		{name: "validation", status: http.StatusBadRequest, check: func(err error) bool {
			var target fgasdk.FgaApiValidationError
			return stderrors.As(err, &target)
		}},
		{name: "not found", status: http.StatusNotFound, check: func(err error) bool {
			var target fgasdk.FgaApiNotFoundError
			return stderrors.As(err, &target)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(test.status)
				_, _ = response.Write([]byte(`{"code":"internal_error","message":"provider-secret"}`))
			})
			authorizer, _ := New(client, directSubject)
			_, err := performCheck(context.Background(), authorizer, request())
			if err == nil || !test.check(err) {
				t.Fatalf("unexpected error classification: %v", err)
			}
		})
	}
}

func TestCheckPreservesContextCancellation(t *testing.T) {
	client, _ := sdkClient(t, func(http.ResponseWriter, *http.Request) {})
	authorizer, _ := New(client, directSubject)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := performCheck(ctx, authorizer, request())
	if !stderrors.Is(err, context.Canceled) || stderrors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestPublicMethodsRejectNilContextWithoutCallingSDK(t *testing.T) {
	client, calls := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(`{"allowed":true}`)) })
	authorizer, _ := New(client, directSubject)
	checkRequest := request()
	if _, err := performCheck(nil, authorizer, checkRequest); !stderrors.Is(err, ErrInvalidInput) {
		t.Fatalf("Check error=%v", err)
	}
	if results, err := authorizer.BatchCheck(nil, []Request{checkRequest}); !stderrors.Is(err, ErrInvalidInput) || results != nil {
		t.Fatalf("BatchCheck results=%v error=%v", results, err)
	}
	if ids, err := authorizer.ListAllowed(nil, checkRequest.Actor, checkRequest.Action, checkRequest.ResourceType); !stderrors.Is(err, ErrInvalidInput) || ids != nil {
		t.Fatalf("ListAllowed ids=%v error=%v", ids, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("SDK calls=%d", calls.Load())
	}
}

func TestBatchCheckOrderAndCardinality(t *testing.T) {
	client, calls := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"result":{"1":{"allowed":false},"0":{"allowed":true}}}`))
	})
	authorizer, _ := New(client, directSubject)
	if empty, err := authorizer.BatchCheck(context.Background(), nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty = %v, error = %v", empty, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("empty batch SDK calls = %d", calls.Load())
	}
	second := request()
	second.ResourceID = "doc-2"
	results, err := authorizer.BatchCheck(context.Background(), []Request{request(), second})
	if err != nil || len(results) != 2 || !results[0] || results[1] {
		t.Fatalf("results = %v, error = %v", results, err)
	}
}

func TestBatchCheckRejectsItemAndProtocolErrors(t *testing.T) {
	tests := []struct {
		name        string
		response    string
		unavailable bool
	}{
		{name: "internal item", response: `{"result":{"0":{"error":{"internal_error":"unavailable","message":"secret"}}}}`, unavailable: true},
		{name: "input item", response: `{"result":{"0":{"error":{"input_error":"invalid_check_input","message":"secret"}}}}`},
		{name: "missing correlation", response: `{"result":{"unexpected":{"allowed":true}}}`},
		{name: "missing allowed", response: `{"result":{"0":{}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				_, _ = response.Write([]byte(test.response))
			})
			authorizer, _ := New(client, directSubject)
			results, err := authorizer.BatchCheck(context.Background(), []Request{request()})
			if err == nil || results != nil || stderrors.Is(err, ErrUnavailable) != test.unavailable {
				t.Fatalf("results = %v, error = %v", results, err)
			}
		})
	}
}

func TestListAllowedRequiresExactPrefix(t *testing.T) {
	tests := []struct {
		name     string
		objects  string
		want     []string
		wantFail bool
	}{
		{name: "valid", objects: `["document:1","document:2"]`, want: []string{"1", "2"}},
		{name: "wrong type", objects: `["folder:1"]`, wantFail: true},
		{name: "empty id", objects: `["document:"]`, wantFail: true},
		{name: "colon id", objects: `["document:org:1"]`, wantFail: true},
		{name: "userset id", objects: `["document:1#reader"]`, wantFail: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				_, _ = response.Write([]byte(`{"objects":` + test.objects + `}`))
			})
			authorizer, _ := New(client, directSubject)
			ids, err := authorizer.ListAllowed(context.Background(), request().Actor, "reader", "document")
			if test.wantFail {
				if err == nil || ids != nil {
					t.Fatalf("ids = %v, error = %v", ids, err)
				}
				return
			}
			if err != nil || strings.Join(ids, ",") != strings.Join(test.want, ",") {
				t.Fatalf("ids = %v, error = %v", ids, err)
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestCheckPreservesTransportFailure(t *testing.T) {
	sentinel := stderrors.New("transport failed")
	client, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{
		ApiUrl:               "http://openfga.invalid",
		StoreId:              testStoreID,
		AuthorizationModelId: testModelID,
		HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, sentinel
		})},
		RetryParams: &fgasdk.RetryParams{MaxRetry: 1, MinWaitInMs: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	authorizer, _ := New(client, directSubject)
	_, err = performCheck(context.Background(), authorizer, request())
	if !stderrors.Is(err, sentinel) {
		t.Fatalf("error = %v", err)
	}
}
