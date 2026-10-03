//go:build integration

package data_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Servora-Kit/plateau/app/example/service/internal/data"
	entmodel "github.com/Servora-Kit/plateau/app/example/service/internal/data/ent"
	entuser "github.com/Servora-Kit/plateau/app/example/service/internal/data/ent/user"
	entgomixin "github.com/Servora-Kit/plateau/infra/entgo/mixin"
	corev1 "github.com/Servora-Kit/servora/api/gen/go/servora/core/v1"
)

func TestSoftDeleteMixinIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SERVORA_EXAMPLE_SQLITE_DSN"))
	if dsn == "" {
		t.Fatal("SERVORA_EXAMPLE_SQLITE_DSN is required for the explicit integration test")
	}
	driver, err := data.NewEntDriver(&corev1.Data{Database: &corev1.Data_Database{
		Driver: "sqlite",
		Source: dsn,
	}})
	if err != nil {
		t.Fatalf("NewEntDriver: %v", err)
	}
	database, err := data.NewDBClient(driver)
	if err != nil {
		_ = driver.Close()
		t.Fatalf("NewDBClient: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})
	ctx := context.Background()
	tx, err := database.Tx(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil {
			t.Errorf("rollback transaction: %v", err)
		}
	})
	client := tx.Client()

	for _, operation := range []string{"delete", "delete_one"} {
		t.Run(operation, func(t *testing.T) {
			row, err := client.User.Create().SetTenantID("mixin-test").SetResourceID(operation).
				SetEmail(operation + "@example.com").SetEtag("initial").Save(ctx)
			if err != nil {
				t.Fatalf("create user: %v", err)
			}
			actorContext := entgomixin.WithDeletedBy(ctx, "principals/tester")
			if operation == "delete" {
				count, err := client.User.Delete().Where(entuser.IDEQ(row.ID)).Exec(actorContext)
				if err != nil || count != 1 {
					t.Fatalf("delete user = (%d, %v), want (1, nil)", count, err)
				}
			} else if err := client.User.DeleteOneID(row.ID).Exec(actorContext); err != nil {
				t.Fatalf("delete one user: %v", err)
			}
			if _, err := client.User.Get(ctx, row.ID); !entmodel.IsNotFound(err) {
				t.Fatalf("default read after soft delete = %v, want not found", err)
			}
			bypass := entgomixin.SkipSoftDelete(ctx)
			tombstone, err := client.User.Get(bypass, row.ID)
			if err != nil {
				t.Fatalf("read tombstone: %v", err)
			}
			if tombstone.DeleteTime == nil || tombstone.DeletedBy == nil || *tombstone.DeletedBy != "principals/tester" {
				t.Fatalf("tombstone fields = delete_time:%v deleted_by:%v", tombstone.DeleteTime, tombstone.DeletedBy)
			}
			if tombstone.PurgeTime != nil {
				t.Fatalf("mixin assigned application-owned purge time: %v", tombstone.PurgeTime)
			}
			if _, err := client.User.UpdateOneID(row.ID).ClearDeleteTime().ClearDeletedBy().Save(bypass); err != nil {
				t.Fatalf("restore user: %v", err)
			}
			restored, err := client.User.Get(ctx, row.ID)
			if err != nil || restored.DeleteTime != nil || restored.DeletedBy != nil {
				t.Fatalf("restored user = (%v, %v)", restored, err)
			}
			if err := client.User.DeleteOneID(row.ID).Exec(bypass); err != nil {
				t.Fatalf("hard delete user: %v", err)
			}
			if _, err := client.User.Get(bypass, row.ID); !entmodel.IsNotFound(err) {
				t.Fatalf("bypass read after hard delete = %v, want not found", err)
			}
		})
	}
}
