SERVORA_CONTEXT := service
include ../../../make/core.mk

SERVORA_KIT_GOWORK ?= $(abspath ../../../../go.work)

.PHONY: test.integration
test.integration:
	@test -n "$$SERVORA_EXAMPLE_SQLITE_DSN" || (echo "SERVORA_EXAMPLE_SQLITE_DSN is required"; exit 1)
	@test -f "$(SERVORA_KIT_GOWORK)" || (echo "Servora-Kit workspace not found: $(SERVORA_KIT_GOWORK)"; exit 1)
	GOWORK="$(SERVORA_KIT_GOWORK)" go test -tags=integration ./internal/service -run '^TestUserReferenceIntegration$$' -count=1

