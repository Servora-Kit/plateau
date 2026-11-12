package session

import (
	"database/sql"
	"testing"
	"time"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestNewPostgresValidationAndCleanup(t *testing.T) {
	if _, _, err := NewPostgres(config(), nil); err == nil {
		t.Fatal("expected nil db to fail")
	}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	badCfg := &sessionpb.Session{
		Lifetime: durationpb.New(-time.Second),
		Cookie:   &sessionpb.Cookie{Name: proto.String("__Host-test")},
	}
	if _, _, err := NewPostgres(badCfg, db); err == nil {
		t.Fatal("expected invalid session config to fail")
	}

	manager, cleanup, err := NewPostgres(config(), db)
	if err != nil {
		t.Fatalf("NewPostgres() unexpected error: %v", err)
	}
	if manager == nil || cleanup == nil {
		t.Fatal("NewPostgres() returned nil manager or cleanup")
	}
	cleanup()
}
