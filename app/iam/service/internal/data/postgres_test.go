package data

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// 每个测试创建独立 Schema；只删除本测试创建的对象。
func newPostgresTestClient(t *testing.T) (*entmodel.Client, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("IAM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("IAM_TEST_POSTGRES_DSN is required for PostgreSQL integration tests")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin := stdlib.OpenDB(*config)
	schema := "test_" + uuid.NewString()[:8]
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client, cleanup, err := NewDBClient(driver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return client, db
}
