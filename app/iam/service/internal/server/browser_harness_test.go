package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

// Opt-in harness for the existing IAM Web; never installed in the application.
func TestBrowserHarness(t *testing.T) {
	if os.Getenv("IAM_TEST_BROWSER_HARNESS") != "1" {
		t.Skip("manual browser harness")
	}
	f := newServerFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:10000")
	check(t, err)
	done := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.Handle("/", f.http)
	mux.HandleFunc("POST /__test/stop", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(204)
		select {
		case done <- struct{}{}:
		default:
		}
	})
	host := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	t.Cleanup(func() { _ = host.Close() })
	go func() { _ = host.Serve(listener) }()
	t.Log("IAM browser harness ready at http://127.0.0.1:10000; fixture login alice@example.com")
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
		t.Error("browser harness was not stopped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	check(t, host.Shutdown(ctx))
}
