BUF_GO_GEN_TEMPLATE ?= buf.go.gen.yaml
BUF_TS_GEN_TEMPLATE ?= buf.typescript.gen.yaml
PNPM ?= pnpm
API_TS_PACKAGE ?= @plateau/api
SERVORA_PKG ?= github.com/Servora-Kit/servora

PROTOC_GEN_GO_VERSION ?= latest
PROTOC_GEN_GO_GRPC_VERSION ?= latest
PROTOC_GEN_GO_HTTP_VERSION ?= latest
PROTOC_GEN_OPENAPI_VERSION ?= latest
PROTOC_GEN_VALIDATE_VERSION ?= latest
PROTOC_GEN_GO_REDACT_VERSION ?= latest
SERVORA_VERSION ?= v0.8.8
GNOSTIC_VERSION ?= latest
BUF_VERSION ?= latest
GOLANGCI_LINT_VERSION ?= latest
WIRE_VERSION ?= latest
ENT_VERSION ?= latest
AIR_VERSION ?= latest

ifeq ($(SERVORA_CONTEXT),root)
GEN_TARGETS := api $(GEN_TARGETS) ent
API_TARGETS += api-go api-ts

.PHONY: init plugin cli api api-go api-ts api-ts.check ent
.PHONY: openfga.init openfga.model.validate openfga.model.test openfga.model.apply

init: plugin cli ## Install protoc plugins, CLI tools, and pnpm workspace dependencies
	@$(PNPM) install --frozen-lockfile

plugin: ## Install protoc-gen-* plugins
	@echo "==> Installing protoc plugins..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	@go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v3@$(PROTOC_GEN_GO_HTTP_VERSION)
	@go install $(SERVORA_PKG)/cmd/protoc-gen-typescript-http@$(SERVORA_VERSION)
	@go install $(SERVORA_PKG)/cmd/protoc-gen-go-errors@$(SERVORA_VERSION)
	@go install github.com/google/gnostic/cmd/protoc-gen-openapi@$(PROTOC_GEN_OPENAPI_VERSION)
	@go install github.com/envoyproxy/protoc-gen-validate@$(PROTOC_GEN_VALIDATE_VERSION)
	@go install $(SERVORA_PKG)/cmd/protoc-gen-redact@$(SERVORA_VERSION)
	@go install ./cmd/protoc-gen-plateau-authz
	@go install $(SERVORA_PKG)/cmd/protoc-gen-servora-audit@$(SERVORA_VERSION)
	@go install ./cmd/protoc-gen-plateau-authn
	@go install $(SERVORA_PKG)/cmd/protoc-gen-servora-conf@$(SERVORA_VERSION)
	@go install $(SERVORA_PKG)/cmd/protoc-gen-servora-crud@$(SERVORA_VERSION)
	@echo "✓ Protoc plugins installed"

cli: ## Install CLI tools
	@echo "==> Installing CLI tools..."
	@go install github.com/google/gnostic@$(GNOSTIC_VERSION)
	@go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@go install github.com/google/wire/cmd/wire@$(WIRE_VERSION)
	@go install entgo.io/ent/cmd/ent@$(ENT_VERSION)
	@go install github.com/air-verse/air@$(AIR_VERSION)
	@echo "✓ CLI tools installed"

api: $(API_TARGETS) ## Generate configured protobuf API code
	@echo "✓ API code generated"

api-go: ## Generate protobuf Go code
	@buf generate --template $(BUF_GO_GEN_TEMPLATE)

api-ts: ## Generate and build the shared TypeScript HTTP API package
	@buf generate --template $(BUF_TS_GEN_TEMPLATE)
	@$(PNPM) --filter $(API_TS_PACKAGE) build

api-ts.check: ## Type-check the shared TypeScript HTTP API package without emitting files
	@$(PNPM) --filter $(API_TS_PACKAGE) typecheck

ent: ## Generate Ent code for services that define generators
	$(call run-in-service-dirs,gen.ent)
	@echo "✓ Ent code generated"

OPENFGA_MODEL ?= manifests/openfga/model/servora.fga
OPENFGA_TESTS ?= manifests/openfga/tests/servora.fga.yaml
OPENFGA_ENV_PREFIX ?= PLATFORM_
OPENFGA_API_URL ?=

ifeq ($(OS),Windows_NT)
OPENFGA_INIT_CMD = powershell -NoProfile -ExecutionPolicy Bypass -File "$(REPO_ROOT)/manifests/scripts/openfga.ps1" init -Model "$(OPENFGA_MODEL)" -EnvFile "$(ENV_FILE_PATH)" -EnvPrefix "$(OPENFGA_ENV_PREFIX)" -ApiUrl "$(OPENFGA_API_URL)"
OPENFGA_APPLY_CMD = powershell -NoProfile -ExecutionPolicy Bypass -File "$(REPO_ROOT)/manifests/scripts/openfga.ps1" apply -Model "$(OPENFGA_MODEL)" -EnvFile "$(ENV_FILE_PATH)" -EnvPrefix "$(OPENFGA_ENV_PREFIX)" -ApiUrl "$(OPENFGA_API_URL)"
else
OPENFGA_INIT_CMD = bash "$(REPO_ROOT)/manifests/scripts/openfga.sh" init --model "$(OPENFGA_MODEL)" --env-file "$(ENV_FILE_PATH)" --env-prefix "$(OPENFGA_ENV_PREFIX)" --api-url "$(OPENFGA_API_URL)"
OPENFGA_APPLY_CMD = bash "$(REPO_ROOT)/manifests/scripts/openfga.sh" apply --model "$(OPENFGA_MODEL)" --env-file "$(ENV_FILE_PATH)" --env-prefix "$(OPENFGA_ENV_PREFIX)" --api-url "$(OPENFGA_API_URL)"
endif

openfga.init: ## Initialize OpenFGA store and model
	@$(OPENFGA_INIT_CMD)

openfga.model.validate: ## Validate OpenFGA model syntax
	@fga model validate --file $(OPENFGA_MODEL) --format fga
	@echo "✓ OpenFGA model valid"

openfga.model.test: openfga.model.validate ## Run OpenFGA model tests
	@fga model test --tests $(OPENFGA_TESTS)
	@echo "✓ OpenFGA model tests passed"

openfga.model.apply: openfga.model.test ## Apply OpenFGA model after validate/test
	@$(OPENFGA_APPLY_CMD)
endif

ifeq ($(SERVORA_CONTEXT),service)
GEN_TARGETS := api $(GEN_TARGETS) gen.ent
RUN_DEPS := api $(RUN_DEPS)

.PHONY: api api-ts gen.ent

api: ## Generate repository Go API and current service TypeScript API
	@$(MAKE) -C $(REPO_ROOT) api-go
	@$(MAKE) api-ts

api-ts: ## Generate current service TypeScript API code
ifneq (,$(wildcard ./api/buf.typescript.gen.yaml))
	@cd $(REPO_ROOT) && buf generate --template $(SERVICE_MODULE)/api/buf.typescript.gen.yaml
else
	@echo "No TypeScript API template found for $(SERVICE_NAME), skipping"
endif

gen.ent: ## Generate Ent code if this service defines a generator
	@if [ -f "./internal/data/generate.go" ]; then \
		go generate ./internal/data; \
	else \
		echo "No Ent generator found for $(SERVICE_NAME), skipping"; \
	fi
endif
