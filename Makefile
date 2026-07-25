SERVORA_CONTEXT := root

PROJECT_NAME := servora-platform
SERVICE_MODULES := app/audit/service app/example/service
GO_WORKSPACE_MODULES := api/gen $(SERVICE_MODULES)
GO_LINT_MODULES ?= $(SERVICE_MODULES)
LINT_GOWORK ?= auto

MICROSERVICES := example audit
COMPOSE_FILES := -f docker-compose.yaml
COMPOSE_SERVICES ?=

ENV_FILE ?= .env
OPENFGA_ENV_PREFIX ?= PLATFORM_
OPENFGA_API_URL ?= http://localhost:18080

include make/core.mk
