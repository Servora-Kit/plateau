package openfga

import (
	"bytes"
	"context"
	stderrors "errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Servora-Kit/servora-platform/security/authz"
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

func request() authz.CheckRequest {
	return authz.CheckRequest{
		Subject:  "user:alice",
		Action:   "reader",
		Resource: authz.Resource{Type: "document", ID: "doc-1"},
	}
}

func TestNewValidation(t *testing.T) {
	if got, err := New(nil); err == nil || got != nil {
		t.Fatalf("authorizer = %v, error = %v", got, err)
	}
	client, _ := sdkClient(t, func(http.ResponseWriter, *http.Request) {})
	if got, err := New(client, nil); err == nil || got != nil || !strings.Contains(err.Error(), "option[0]") {
		t.Fatalf("authorizer = %v, error = %v", got, err)
	}
	if got, err := New(client, WithLogger(nil)); err == nil || got != nil || !strings.Contains(err.Error(), "option[0]") {
		t.Fatalf("authorizer = %v, error = %v", got, err)
	}
}

func TestDirectRequestValidationDoesNotCallSDK(t *testing.T) {
	client, calls := sdkClient(t, func(http.ResponseWriter, *http.Request) { t.Fatal("SDK called for invalid request") })
	authorizer, err := New(client)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []authz.CheckRequest{
		{Action: "read", Resource: authz.Resource{Type: "document", ID: "1"}},
		{Subject: "user:1", Resource: authz.Resource{Type: "document", ID: "1"}},
		{Subject: "user:1", Action: "read", Resource: authz.Resource{ID: "1"}},
		{Subject: "user:1", Action: "read", Resource: authz.Resource{Type: "document"}},
	}
	for _, request := range invalid {
		if _, err := authorizer.Check(context.Background(), request); err == nil {
			t.Fatalf("Check(%#v) error = nil", request)
		}
	}
	if results, err := authorizer.BatchCheck(context.Background(), invalid[:1]); err == nil || results != nil {
		t.Fatalf("BatchCheck results = %v, error = %v", results, err)
	}
	if ids, err := authorizer.ListAllowed(context.Background(), "", "read", "document"); err == nil || ids != nil {
		t.Fatalf("ListAllowed ids = %v, error = %v", ids, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("SDK calls = %d", calls.Load())
	}
}

func TestCheckMapsRequest(t *testing.T) {
	body := make(chan string, 1)
	client, _ := sdkClient(t, func(response http.ResponseWriter, request *http.Request) {
		data, _ := io.ReadAll(request.Body)
		body <- string(data)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"allowed":true}`))
	})
	authorizer, _ := New(client)
	request := request()
	allowed, err := authorizer.Check(context.Background(), request)
	if err != nil || !allowed {
		t.Fatalf("allowed = %v, error = %v", allowed, err)
	}
	gotBody := <-body
	for _, want := range []string{`"user":"user:alice"`, `"relation":"reader"`, `"object":"document:doc-1"`} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("body missing %s: %s", want, gotBody)
		}
	}
}

func TestCheckProviderErrorClassification(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		unavailable bool
	}{
		{name: "rate limit", status: http.StatusTooManyRequests, unavailable: true},
		{name: "internal", status: http.StatusServiceUnavailable, unavailable: true},
		{name: "authentication", status: http.StatusUnauthorized},
		{name: "validation", status: http.StatusBadRequest},
		{name: "not found", status: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(test.status)
				_, _ = response.Write([]byte(`{"code":"internal_error","message":"provider-secret"}`))
			})
			authorizer, _ := New(client)
			_, err := authorizer.Check(context.Background(), request())
			if err == nil || stderrors.Is(err, authz.ErrUnavailable) != test.unavailable {
				t.Fatalf("error = %v, unavailable = %v", err, stderrors.Is(err, authz.ErrUnavailable))
			}
		})
	}
}

func TestCheckPreservesContextCancellation(t *testing.T) {
	client, _ := sdkClient(t, func(http.ResponseWriter, *http.Request) {})
	authorizer, _ := New(client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := authorizer.Check(ctx, request())
	if !stderrors.Is(err, context.Canceled) || stderrors.Is(err, authz.ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestBatchCheckOrderAndCardinality(t *testing.T) {
	client, calls := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"result":{"1":{"allowed":false},"0":{"allowed":true}}}`))
	})
	authorizer, _ := New(client)
	if empty, err := authorizer.BatchCheck(context.Background(), nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty = %v, error = %v", empty, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("empty batch SDK calls = %d", calls.Load())
	}
	second := request()
	second.Resource.ID = "doc-2"
	results, err := authorizer.BatchCheck(context.Background(), []authz.CheckRequest{request(), second})
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
			authorizer, _ := New(client)
			results, err := authorizer.BatchCheck(context.Background(), []authz.CheckRequest{request()})
			if err == nil || results != nil || stderrors.Is(err, authz.ErrUnavailable) != test.unavailable {
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				_, _ = response.Write([]byte(`{"objects":` + test.objects + `}`))
			})
			authorizer, _ := New(client)
			ids, err := authorizer.ListAllowed(context.Background(), "user:alice", "reader", "document")
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

func TestWithLoggerRecordsSafeProviderFailure(t *testing.T) {
	var output bytes.Buffer
	client, _ := sdkClient(t, func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte(`{"code":"unauthenticated","message":"bad token"}`))
	})
	authorizer, err := New(client, WithLogger(slog.New(slog.NewTextHandler(&output, nil))))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = authorizer.Check(context.Background(), request())
	if !strings.Contains(output.String(), "operation=check") || !strings.Contains(output.String(), "reason=internal") || strings.Contains(output.String(), "bad token") {
		t.Fatalf("log = %q", output.String())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestCheckUnknownTransportFailureIsUnavailable(t *testing.T) {
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
	authorizer, _ := New(client)
	_, err = authorizer.Check(context.Background(), request())
	if !stderrors.Is(err, sentinel) || !stderrors.Is(err, authz.ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
}
