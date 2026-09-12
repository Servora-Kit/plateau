package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOAuthClientCacheAndConcurrentRenewal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var exchanges atomic.Int32
		transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			id, secret, basic := r.BasicAuth()
			if !basic || id != "admin" || secret != serviceSecret {
				t.Error("token request did not use fixed header authentication")
			}
			count := exchanges.Add(1)
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"access_token":"token-%d","token_type":"Bearer","expires_in":12}`, count))), Request: r}, nil
		})
		ctx := context.WithValue(t.Context(), oauth2.HTTPClient, &http.Client{Transport: transport, Timeout: 5 * time.Second})
		config := clientcredentials.Config{ClientID: "admin", ClientSecret: serviceSecret, TokenURL: "https://iam.example/oauth/token", AuthStyle: oauth2.AuthStyleInHeader}
		source := config.TokenSource(ctx)
		first, err := source.Token()
		check(t, err)
		for range 20 {
			token, err := source.Token()
			check(t, err)
			if token != first {
				t.Fatal("valid cache was replaced")
			}
		}
		if exchanges.Load() != 1 {
			t.Fatal("valid token was reacquired")
		}
		// The SDK's default ten-second margin is entered after three fake seconds.
		time.Sleep(3 * time.Second)
		var callers sync.WaitGroup
		for range 32 {
			callers.Go(func() {
				token, err := source.Token()
				if err != nil || token.AccessToken != "token-2" {
					t.Errorf("concurrent renewal token=%v error=%v", token, err)
				}
			})
		}
		callers.Wait()
		if exchanges.Load() != 2 {
			t.Fatalf("concurrent callers exchanged %d tokens", exchanges.Load())
		}
	})
}

func TestOAuthClientFailureStopsBusinessRequest(t *testing.T) {
	for _, failure := range []string{"timeout", "invalid-client"} {
		t.Run(failure, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var exchanges, business atomic.Int32
				transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.Path != "/oauth/token" {
						business.Add(1)
						return nil, fmt.Errorf("unexpected business request")
					}
					exchanges.Add(1)
					if failure == "timeout" {
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					return &http.Response{StatusCode: 401, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":"invalid_client"}`)), Request: r}, nil
				})
				ctx := context.WithValue(t.Context(), oauth2.HTTPClient, &http.Client{Transport: transport, Timeout: time.Second})
				config := clientcredentials.Config{ClientID: "admin", ClientSecret: serviceSecret, TokenURL: "https://iam.example/oauth/token", AuthStyle: oauth2.AuthStyleInHeader}
				client := oauth2.NewClient(ctx, config.TokenSource(ctx))
				if response, err := client.Get("https://business.example/change"); err == nil {
					if err := response.Body.Close(); err != nil {
						t.Error(err)
					}
					t.Fatal("token failure was ignored")
				}
				if exchanges.Load() != 1 || business.Load() != 0 {
					t.Fatalf("token failure was retried or request escaped: exchanges=%d business=%d", exchanges.Load(), business.Load())
				}
			})
		})
	}
}

func TestOAuthClientDoesNotReplayHTTPAuthFailures(t *testing.T) {
	for _, code := range []int{401, 403} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var calls int
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Authorization") != "Bearer access-token" {
					t.Error("HTTP bearer was not attached")
				}
				return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("denied")), Request: r}, nil
			})
			ctx := context.WithValue(t.Context(), oauth2.HTTPClient, &http.Client{Transport: transport})
			client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access-token", TokenType: "Bearer"}))
			response, err := client.Post("https://business.example/change", "application/json", strings.NewReader("{}"))
			check(t, err)
			check(t, response.Body.Close())
			if calls != 1 || response.StatusCode != code {
				t.Fatalf("HTTP request was replayed: %d", calls)
			}
		})
	}
}
