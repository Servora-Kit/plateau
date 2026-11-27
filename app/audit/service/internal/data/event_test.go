package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	auditconfv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/conf/v1"
	auditeventpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/event/v1"
	clickhousepb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/clickhouse/v1"
	securityauditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/biz"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/data/schema"
	pkgch "github.com/Servora-Kit/plateau/infra/clickhouse"
	"github.com/Servora-Kit/plateau/security/audit"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestExtractRetentionDays(t *testing.T) {
	cases := []struct {
		engineFull string
		wantDays   int32
		wantOK     bool
	}{
		{
			engineFull: "ReplacingMergeTree(ingested_at) PARTITION BY toYYYYMM(occurred_at) ORDER BY (occurred_at, event_id) TTL toDateTime(occurred_at) + toIntervalDay(180) SETTINGS index_granularity = 8192",
			wantDays:   180,
			wantOK:     true,
		},
		{
			engineFull: "ReplacingMergeTree(ingested_at) TTL toDateTime(occurred_at) + INTERVAL 365 DAY",
			wantDays:   365,
			wantOK:     true,
		},
		{
			engineFull: "ReplacingMergeTree(ingested_at) ORDER BY event_id",
			wantDays:   0,
			wantOK:     false,
		},
	}

	for _, tc := range cases {
		got, ok := schema.ExtractRetentionDays(tc.engineFull)
		if ok != tc.wantOK || got != tc.wantDays {
			t.Fatalf("ExtractRetentionDays(%q) = (%d, %v), want (%d, %v)", tc.engineFull, got, ok, tc.wantDays, tc.wantOK)
		}
	}
}

func TestNewClickHouseConnFailFast(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, _, err := NewClickHouseConn(nil, logger); err == nil {
		t.Fatal("NewClickHouseConn(nil) succeeded, want error")
	}
	if _, _, err := NewClickHouseConn(&clickhousepb.ClickHouse{}, logger); err == nil {
		t.Fatal("NewClickHouseConn(empty addrs) succeeded, want error")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	deadAddr := ln.Addr().String()
	_ = ln.Close()

	if _, _, err := NewClickHouseConn(&clickhousepb.ClickHouse{
		Addrs:       []string{deadAddr},
		Database:    "audit",
		DialTimeout: durationpb.New(300 * time.Millisecond),
	}, logger); err == nil {
		t.Fatal("NewClickHouseConn(unreachable) succeeded, want error")
	}
}

func TestClickHouseSchemaTTLDeduplicationAndQueries(t *testing.T) {
	addr := os.Getenv("AUDIT_TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:19000"
	}
	probe, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Skipf("skipping ClickHouse integration test (%s not reachable: %v)", addr, err)
	}
	_ = probe.Close()

	user := envOrDefault("CLICKHOUSE_USER", "clickhouse")
	pass := envOrDefault("CLICKHOUSE_PASSWORD", "clickhouse")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	testDB := "audit_it_" + hex.EncodeToString(suffix[:])

	adminConn, err := pkgch.NewConnOptional(context.Background(), &clickhousepb.ClickHouse{
		Addrs:       []string{addr},
		Database:    "default",
		Username:    user,
		Password:    pass,
		DialTimeout: durationpb.New(5 * time.Second),
	})
	if err != nil || adminConn == nil {
		t.Skipf("skipping ClickHouse integration test (auth/ping unavailable: %v)", err)
	}
	t.Cleanup(func() {
		_ = adminConn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+testDB)
		_ = adminConn.Close()
	})

	if err := adminConn.Exec(context.Background(), "CREATE DATABASE "+testDB); err != nil {
		t.Fatalf("CREATE DATABASE %s: %v", testDB, err)
	}

	conn, closeConn, err := NewClickHouseConn(&clickhousepb.ClickHouse{
		Addrs:       []string{addr},
		Database:    testDB,
		Username:    user,
		Password:    pass,
		DialTimeout: durationpb.New(5 * time.Second),
	}, logger)
	if err != nil {
		t.Fatalf("NewClickHouseConn(%s): %v", testDB, err)
	}
	t.Cleanup(closeConn)

	retention180 := int32(180)
	d, err := NewData(conn, &auditconfv1.Audit{RetentionDays: &retention180}, logger)
	if err != nil {
		t.Fatalf("NewData(180): %v", err)
	}

	var engineFull string
	if err := conn.QueryRow(context.Background(), "SELECT engine_full FROM system.tables WHERE database = ? AND name = ?", testDB, schema.TableEvents).Scan(&engineFull); err != nil {
		t.Fatalf("query system.tables: %v", err)
	}
	if days, ok := schema.ExtractRetentionDays(engineFull); !ok || days != 180 {
		t.Fatalf("initial retention days = (%d, %v) from %q, want 180", days, ok, engineFull)
	}

	if err := schema.EnsureEventsTable(context.Background(), conn, 365); err != nil {
		t.Fatalf("EnsureEventsTable(365): %v", err)
	}
	if err := conn.QueryRow(context.Background(), "SELECT engine_full FROM system.tables WHERE database = ? AND name = ?", testDB, schema.TableEvents).Scan(&engineFull); err != nil {
		t.Fatalf("query system.tables after MODIFY TTL: %v", err)
	}
	if days, ok := schema.ExtractRetentionDays(engineFull); !ok || days != 365 {
		t.Fatalf("updated retention days = (%d, %v) from %q, want 365", days, ok, engineFull)
	}

	repo, err := NewEventRepo(d)
	if err != nil {
		t.Fatalf("NewEventRepo: %v", err)
	}

	octTime := time.Date(2026, 10, 31, 23, 59, 58, 123456000, time.UTC)
	novTime1 := time.Date(2026, 11, 1, 0, 0, 1, 654321000, time.UTC)
	novTime2 := time.Date(2026, 11, 1, 0, 0, 2, 100000000, time.UTC)
	sharedTrace := "4bf92f3577b34da6a3ce929d0e0e4736"

	ev1 := &auditeventpb.Event{
		EventId:      "ev-oct-iam",
		EventType:    audit.DefaultRPCEventType,
		Category:     securityauditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE,
		OccurredTime: timestamppb.New(octTime),
		Service:      "iam.service",
		Operation:    "/plateau.iam.user.v1.UserService/DisableUser",
		ActorType:    "service",
		ActorId:      "admin",
		ResourceType: "iam.plateau.dev/User",
		ResourceName: "users/u-100",
		Success:      true,
		TraceId:      sharedTrace,
		RequestId:    "req-1",
		DetailJson:   `{"name":"users/u-100"}`,
	}
	ev2 := &auditeventpb.Event{
		EventId:      "ev-nov-admin",
		EventType:    audit.DefaultRPCEventType,
		Category:     securityauditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE,
		OccurredTime: timestamppb.New(novTime1),
		Service:      "admin.service",
		Operation:    "/plateau.admin.user.v1.UserService/DisableUser",
		ActorType:    "human",
		ActorId:      "admin-1",
		ResourceType: "iam.plateau.dev/User",
		ResourceName: "users/u-100",
		Success:      true,
		TraceId:      sharedTrace,
		RequestId:    "req-1",
		ClientIp:     "127.0.0.1",
		UserAgent:    "Mozilla/5.0",
		DetailJson:   `{"name":"users/u-100"}`,
	}
	ev3 := &auditeventpb.Event{
		EventId:      "ev-nov-denied",
		EventType:    audit.AuthzDeniedEventType,
		Category:     securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ,
		OccurredTime: timestamppb.New(novTime2),
		Service:      "admin.service",
		Operation:    "/plateau.admin.user.v1.UserService/ExpungeUser",
		ActorType:    "human",
		ActorId:      "guest-9",
		ResourceType: "admin",
		ResourceName: "admin/global",
		Success:      false,
		ErrorCode:    "SECURITY_ERROR_REASON_PERMISSION_DENIED",
		ErrorMessage: "permission denied",
		TraceId:      "11111111111111111111111111111111",
		DetailJson:   "{}",
	}

	if err := repo.SaveBatch(context.Background(), []*auditeventpb.Event{ev1, ev2, ev3}); err != nil {
		t.Fatalf("SaveBatch(1): %v", err)
	}
	if err := repo.SaveBatch(context.Background(), []*auditeventpb.Event{ev2}); err != nil {
		t.Fatalf("SaveBatch(duplicate retry): %v", err)
	}

	gotSingle, err := repo.GetByID(context.Background(), "ev-nov-admin")
	if err != nil {
		t.Fatalf("GetByID(ev-nov-admin): %v", err)
	}
	if gotSingle.GetName() != "events/ev-nov-admin" ||
		gotSingle.GetActorId() != "admin-1" ||
		!gotSingle.GetOccurredTime().AsTime().Equal(novTime1) ||
		gotSingle.GetClientIp() != "127.0.0.1" {
		t.Fatalf("unexpected GetByID result: %+v", gotSingle)
	}
	if _, err := repo.GetByID(context.Background(), "non-existent"); !errors.Is(err, biz.ErrEventNotFound) {
		t.Fatalf("GetByID(non-existent) error = %v, want ErrEventNotFound", err)
	}

	uc, err := biz.NewEventUsecase(repo, logger)
	if err != nil {
		t.Fatalf("NewEventUsecase: %v", err)
	}
	traceEvents, nextToken, err := uc.ListEvents(context.Background(), biz.ListEventsQuery{
		TraceID: sharedTrace,
	})
	if err != nil {
		t.Fatalf("ListEvents(trace_id): %v", err)
	}
	if len(traceEvents) != 2 || nextToken != "" || traceEvents[0].GetEventId() != "ev-nov-admin" || traceEvents[1].GetEventId() != "ev-oct-iam" {
		t.Fatalf("unexpected trace_id events (dedup/order failed): %+v", traceEvents)
	}

	p1, tok, err := uc.ListEvents(context.Background(), biz.ListEventsQuery{PageSize: 2})
	if err != nil || len(p1) != 2 || tok == "" {
		t.Fatalf("page1 = (%+v, %q, %v)", p1, tok, err)
	}
	p2, tok2, err := uc.ListEvents(context.Background(), biz.ListEventsQuery{PageSize: 2, PageToken: tok})
	if err != nil || len(p2) != 1 || tok2 != "" || p2[0].GetEventId() != "ev-oct-iam" {
		t.Fatalf("page2 = (%+v, %q, %v)", p2, tok2, err)
	}

	onlyFail := false
	failedEvents, _, err := uc.ListEvents(context.Background(), biz.ListEventsQuery{
		Category: securityauditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ,
		Success:  &onlyFail,
	})
	if err != nil || len(failedEvents) != 1 || failedEvents[0].GetEventId() != "ev-nov-denied" {
		t.Fatalf("failedEvents = (%+v, %v)", failedEvents, err)
	}

	services, err := uc.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices(): %v", err)
	}
	if len(services) != 2 || services[0] != "admin.service" || services[1] != "iam.service" {
		t.Fatalf("ListServices() = %v, want [admin.service iam.service]", services)
	}
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return fmt.Sprint(v)
	}
	return fallback
}
