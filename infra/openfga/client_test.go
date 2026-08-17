package openfga

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	openfgaconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/openfga/v1"
	fgaclient "github.com/openfga/go-sdk/client"
	"google.golang.org/protobuf/proto"
)

const (
	testStoreID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	testModelID = "01ARZ3NDEKTSV4RRFFQ69G5FAW"
)

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  *openfgaconfpb.OpenFGA
	}{
		{name: "nil"},
		{name: "missing api url", cfg: &openfgaconfpb.OpenFGA{StoreId: testStoreID}},
		{name: "missing store", cfg: &openfgaconfpb.OpenFGA{ApiUrl: "http://127.0.0.1"}},
		{name: "token over HTTP", cfg: &openfgaconfpb.OpenFGA{ApiUrl: "http://openfga.example", StoreId: testStoreID, ApiToken: "secret-token"}},
		{name: "token without hostname", cfg: &openfgaconfpb.OpenFGA{ApiUrl: "https://:443", StoreId: testStoreID, ApiToken: "secret-token"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := New(test.cfg)
			if err == nil || client != nil || strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("client=%v error=%v", client, err)
			}
		})
	}
}

func TestNewMapsConfigWithoutMutationOrConstructorNetwork(t *testing.T) {
	var requests atomic.Int32
	captured := make(chan string, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(request.Body)
		captured <- request.Header.Get("Authorization") + "\n" + request.URL.Path + "\n" + string(body)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"allowed":true}`))
	}))
	defer server.Close()

	previousDefault := http.DefaultClient
	http.DefaultClient = server.Client()
	defer func() { http.DefaultClient = previousDefault }()

	config := &openfgaconfpb.OpenFGA{ApiUrl: server.URL, StoreId: testStoreID, ModelId: testModelID, ApiToken: "secret-token"}
	before := proto.Clone(config)
	client, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 || !proto.Equal(config, before) {
		t.Fatalf("constructor requests=%d config mutated=%v", requests.Load(), !proto.Equal(config, before))
	}

	response, err := client.Check(context.Background()).Body(fgaclient.ClientCheckRequest{User: "user:alice", Relation: "reader", Object: "document:doc-1"}).Execute()
	if err != nil || !response.GetAllowed() {
		t.Fatalf("allowed=%v error=%v", response.GetAllowed(), err)
	}
	request := strings.Split(<-captured, "\n")
	if request[0] != "Bearer secret-token" || !strings.Contains(request[1], "/stores/"+testStoreID+"/check") || !strings.Contains(request[2], testModelID) {
		t.Fatalf("request=%q", request)
	}
}

func TestNewDisablesRedirectsWhenTokenConfigured(t *testing.T) {
	var plaintextHits atomic.Int32
	plaintext := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { plaintextHits.Add(1) }))
	defer plaintext.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, plaintext.URL, http.StatusTemporaryRedirect)
	}))
	defer secure.Close()

	previousDefault := http.DefaultClient
	http.DefaultClient = secure.Client()
	defer func() { http.DefaultClient = previousDefault }()

	client, err := New(&openfgaconfpb.OpenFGA{ApiUrl: secure.URL, StoreId: testStoreID, ApiToken: "secret-token"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Check(context.Background()).Body(fgaclient.ClientCheckRequest{User: "user:alice", Relation: "reader", Object: "document:doc-1"}).Execute()
	if err == nil || plaintextHits.Load() != 0 || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error=%v plaintext hits=%d", err, plaintextHits.Load())
	}
}

func TestNewLeavesEmptyModelUnset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("constructor performed network request") }))
	defer server.Close()
	client, err := New(&openfgaconfpb.OpenFGA{ApiUrl: server.URL, StoreId: testStoreID})
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := client.GetAuthorizationModelId()
	if err != nil || modelID != "" {
		t.Fatalf("modelID=%q error=%v", modelID, err)
	}
}
