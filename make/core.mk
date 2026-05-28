CORE_MK_DIR := $(dir $(lastword $(MAKEFILE_LIST)))
REPO_ROOT := $(abspath $(CORE_MK_DIR)/..)
ROOT_DIR := $(REPO_ROOT)/
CURRENT_DIR := $(CURDIR)

SERVORA_CONTEXT ?= root
ENV_FILE ?= .env
ENV_FILE_PATH := $(if $(filter /%,$(ENV_FILE)),$(ENV_FILE),$(REPO_ROOT)/$(ENV_FILE))

ifneq (,$(wildcard $(ENV_FILE_PATH)))
    include $(ENV_FILE_PATH)
    export
endif

SERVICE_MODULES ?=
SERVICE_DIRS := $(sort $(foreach mod,$(SERVICE_MODULES),$(abspath $(REPO_ROOT)/$(mod))))
GO_WORKSPACE_MODULES ?= api/gen $(SERVICE_MODULES)
GO_LINT_MODULES ?= $(SERVICE_MODULES)
GEN_TARGETS ?= openapi wire
LINT_TARGETS ?= lint.go lint.proto
LINT_GOWORK ?= auto
COMPOSE ?= docker compose
COMPOSE_FILES ?= -f docker-compose.yaml
COMPOSE_SERVICES ?=
MICROSERVICES ?=
DOCKER_BUILD_SERVICES ?= $(MICROSERVICES)
CONF ?= ./configs/local/

GOPATH ?= $(shell go env GOPATH)
GOVERSION ?= $(shell go version)
VERSION ?= $(shell cd $(REPO_ROOT) && git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT ?= $(shell cd $(REPO_ROOT) && git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH ?= $(shell cd $(REPO_ROOT) && git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
DOCKER_TAG_VERSION_RAW := $(shell printf '%s' "$(VERSION)" | sed -E 's/[^[:alnum:]_.-]+/-/g; s/^[.-]+//; s/-+/-/g; s/[.-]+$$//')
DOCKER_TAG_VERSION ?= $(if $(DOCKER_TAG_VERSION_RAW),$(DOCKER_TAG_VERSION_RAW),dev)

ifeq ($(SERVORA_CONTEXT),service)
SERVICE_DIR := $(abspath $(CURDIR))
SERVICE_MODULE := $(patsubst $(REPO_ROOT)/%,%,$(SERVICE_DIR))
SERVICE_NAME ?= $(notdir $(patsubst %/,%,$(dir $(SERVICE_DIR))))
APP_NAME ?= $(subst /,-,$(patsubst app/%,%,$(SERVICE_MODULE)))
LDFLAGS ?= -X main.Version=$(VERSION) -X main.Name=$(SERVICE_NAME).service
GOFLAGS ?=
RUN_DEPS ?= openapi
endif

define run-in-service-dirs
	@set -e; for dir in $(SERVICE_DIRS); do \
		echo "==> $(1) $$dir"; \
		$(MAKE) -C "$$dir" $(1); \
	done
endef

-include $(CORE_MK_DIR)extra.mk

.DEFAULT_GOAL := help

.PHONY: help env
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_.-]+:.*?## / { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

env: ## Print resolved Make environment
	@echo "SERVORA_CONTEXT: $(SERVORA_CONTEXT)"
	@echo "REPO_ROOT: $(REPO_ROOT)"
	@echo "CURRENT_DIR: $(CURRENT_DIR)"
	@echo "ENV_FILE: $(ENV_FILE)"
	@echo "ENV_FILE_PATH: $(ENV_FILE_PATH)"
	@echo "SERVICE_MODULES: $(SERVICE_MODULES)"
	@echo "SERVICE_DIRS: $(SERVICE_DIRS)"
	@echo "GEN_TARGETS: $(GEN_TARGETS)"
	@echo "API_TARGETS: $(API_TARGETS)"
	@echo "LINT_TARGETS: $(LINT_TARGETS)"
	@echo "COMPOSE_FILES: $(COMPOSE_FILES)"
	@echo "COMPOSE_SERVICES: $(COMPOSE_SERVICES)"
	@echo "MICROSERVICES: $(MICROSERVICES)"
	@echo "VERSION: $(VERSION)"
	@echo "GOVERSION: $(GOVERSION)"
ifeq ($(SERVORA_CONTEXT),service)
	@echo "SERVICE_MODULE: $(SERVICE_MODULE)"
	@echo "SERVICE_NAME: $(SERVICE_NAME)"
	@echo "APP_NAME: $(APP_NAME)"
	@echo "RUN_DEPS: $(RUN_DEPS)"
	@echo "CONF: $(CONF)"
endif

ifeq ($(SERVORA_CONTEXT),root)
.PHONY: gen gen.clean gen.fresh openapi wire build clean lint lint.go lint.proto
.PHONY: compose.build compose.up compose.stop compose.down compose.reset compose.ps compose.logs

gen: $(GEN_TARGETS) ## Generate configured project code
	@echo "✓ Code generated"

gen.clean: ## Remove repository generated API code
	@rm -rf api/gen/go
	@echo "✓ Generated API code cleaned"

gen.fresh: gen.clean gen ## Clean generated API code and regenerate

openapi: ## Generate OpenAPI docs for all services
	$(call run-in-service-dirs,openapi)
	@echo "✓ OpenAPI documentation generated"

wire: ## Generate Wire code for all services
	$(call run-in-service-dirs,wire)
	@echo "✓ Wire code generated"

build: gen ## Build all service applications
	$(call run-in-service-dirs,_build)
	@echo "✓ Services built"

clean: ## Clean service build artifacts
	$(call run-in-service-dirs,clean)
	@echo "✓ Service artifacts cleaned"

lint: $(LINT_TARGETS) ## Run configured linters
	@echo "✓ lint complete"

lint.go: ## Run Go lint in configured modules
	@set -e; for mod in $(GO_LINT_MODULES); do \
		echo "==> Linting Go ($$mod, GOWORK=$(LINT_GOWORK))"; \
		(cd "$(REPO_ROOT)/$$mod" && GOWORK=$(LINT_GOWORK) golangci-lint run); \
	done
	@echo "✓ Go lint complete"

lint.proto: ## Run buf lint
	@buf lint
	@echo "✓ Proto lint complete"

compose.build: ## Build service Docker images
	@set -e; for svc in $(DOCKER_BUILD_SERVICES); do \
		echo "==> Building servora-$$svc:$(DOCKER_TAG_VERSION)"; \
		docker build --build-arg SERVICE_NAME=$$svc --build-arg VERSION=$(VERSION) -t servora-$$svc:$(DOCKER_TAG_VERSION) .; \
		docker tag servora-$$svc:$(DOCKER_TAG_VERSION) servora-$$svc:latest; \
	done
	@echo "✓ Docker images built"

compose.up: ## Start compose services
	@$(COMPOSE) $(COMPOSE_FILES) up -d $(COMPOSE_SERVICES)

compose.stop: ## Stop compose services without removing containers
	@$(COMPOSE) $(COMPOSE_FILES) stop $(COMPOSE_SERVICES)

compose.down: ## Remove compose containers and networks, keeping volumes
	@$(COMPOSE) $(COMPOSE_FILES) down --remove-orphans

compose.reset: ## Remove compose containers, networks, and volumes
	@$(COMPOSE) $(COMPOSE_FILES) down --remove-orphans --volumes

compose.ps: ## Show compose service status
	@$(COMPOSE) $(COMPOSE_FILES) ps $(COMPOSE_SERVICES)

compose.logs: ## Tail compose logs
	@$(COMPOSE) $(COMPOSE_FILES) logs -f $(COMPOSE_SERVICES)
endif

ifeq ($(SERVORA_CONTEXT),service)
.PHONY: gen openapi wire build _build clean run dev app lint.go

gen: $(GEN_TARGETS) ## Generate configured service code
	@echo "✓ Service code generated"

openapi: ## Generate current service OpenAPI docs
ifneq (,$(wildcard ./api/buf.openapi.gen.yaml))
	@cd $(REPO_ROOT) && buf generate --template $(SERVICE_MODULE)/api/buf.openapi.gen.yaml
else
	@echo "No OpenAPI template found for $(SERVICE_NAME), skipping"
endif

wire: ## Generate current service Wire code
ifneq (,$(wildcard ./cmd/server))
	@go run github.com/google/wire/cmd/wire ./cmd/server
else
	@echo "No cmd/server directory found for $(SERVICE_NAME), skipping wire"
endif

build: gen _build ## Generate code and build current service

_build:
ifneq (,$(wildcard ./cmd))
	@go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ./bin/ ./...
else
	@echo "No cmd directory found for $(SERVICE_NAME), skipping build"
endif

run: $(RUN_DEPS) ## Run current service with go run
	-@go run $(GOFLAGS) -ldflags "$(LDFLAGS)" ./cmd/server -conf $(CONF)

dev: $(RUN_DEPS) ## Run current service with Air hot reload
	@air

app: build ## Alias for build

clean: ## Clean current service local artifacts
	@go clean
	@rm -f coverage.out openapi.yaml internal/assets/openapi.yaml
	@rm -rf bin tmp
	@echo "✓ Service artifacts cleaned"

lint.go: ## Run Go lint in current service
	@golangci-lint run
endif
