SERVORA_CONTEXT := root

PROJECT_NAME := servora-platform
SERVICE_MODULES := app/audit/service
GO_WORKSPACE_MODULES := api/gen $(SERVICE_MODULES)
GO_LINT_MODULES ?= $(SERVICE_MODULES)
LINT_GOWORK ?= auto

MICROSERVICES := audit
COMPOSE_FILES := -f docker-compose.yaml
COMPOSE_SERVICES ?=

BUF_GO_GEN_TEMPLATE := buf.go.gen.yaml
BUF_TS_GEN_TEMPLATE := buf.typescript.gen.yaml

ENV_FILE ?= .env
OPENFGA_ENV_PREFIX ?= PLATFORM_
OPENFGA_API_URL ?= http://localhost:18080

include make/core.mk
