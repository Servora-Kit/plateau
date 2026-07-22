SERVORA_CONTEXT := service
include ../../../make/core.mk

.PHONY: test.integration
test.integration:
	@test -n "$$SERVORA_EXAMPLE_SQLITE_DSN" || (echo "SERVORA_EXAMPLE_SQLITE_DSN is required"; exit 1)
	go test -tags=integration ./internal/service -run '^TestUserReferenceIntegration$$' -count=1

