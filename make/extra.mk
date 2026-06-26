BUF_GO_GEN_TEMPLATE ?= buf.go.gen.yaml
BUF_TS_GEN_TEMPLATE ?= buf.typescript.gen.yaml
SERVORA_PKG ?= github.com/Servora-Kit/servora

PROTOC_GEN_GO_VERSION ?= latest
PROTOC_GEN_GO_GRPC_VERSION ?= latest
PROTOC_GEN_GO_HTTP_VERSION ?= v3.0.0-20260621094049-2726761cdd77
PROTOC_GEN_TYPESCRIPT_HTTP_VERSION ?= latest
PROTOC_GEN_GO_ERRORS_VERSION ?= v3.0.0-20260621094049-2726761cdd77
PROTOC_GEN_OPENAPI_VERSION ?= latest
PROTOC_GEN_VALIDATE_VERSION ?= latest
PROTOC_GEN_REDACT_VERSION ?= latest
SERVORA_VERSION ?= latest
KRATOS_VERSION ?= v3.0.0-20260621094049-2726761cdd77
GNOSTIC_VERSION ?= latest
BUF_VERSION ?= latest
GOLANGCI_LINT_VERSION ?= latest
WIRE_VERSION ?= latest
ENT_VERSION ?= latest
AIR_VERSION ?= latest

ifeq ($(SERVORA_CONTEXT),root)
GEN_TARGETS := api $(GEN_TARGETS) ent
API_TARGETS += api-go api-ts

.PHONY: init plugin cli api api-go api-ts ent
.PHONY: openfga.init openfga.model.validate openfga.model.test openfga.model.apply

init: plugin cli ## Install protoc plugins and CLI tools

plugin: ## Install protoc-gen-* plugins
	@echo "==> Installing protoc plugins..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	@go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v3@$(PROTOC_GEN_GO_HTTP_VERSION)
	@go install github.com/go-kratos/protoc-gen-typescript-http@$(PROTOC_GEN_TYPESCRIPT_HTTP_VERSION)
	@go install github.com/go-kratos/kratos/cmd/protoc-gen-go-errors/v3@$(PROTOC_GEN_GO_ERRORS_VERSION)
	@go install github.com/google/gnostic/cmd/protoc-gen-openapi@$(PROTOC_GEN_OPENAPI_VERSION)
	@go install github.com/envoyproxy/protoc-gen-validate@$(PROTOC_GEN_VALIDATE_VERSION)
	@go install github.com/menta2k/protoc-gen-redact/v3@$(PROTOC_GEN_REDACT_VERSION)
	@go install $(SERVORA_PKG)/cmd/protoc-gen-servora-authz@$(SERVORA_VERSION)
	@go install $(SERVORA_PKG)/cmd/protoc-gen-servora-audit@$(SERVORA_VERSION)
	@go install $(SERVORA_PKG)/cmd/protoc-gen-servora-authn@$(SERVORA_VERSION)
	@go install $(SERVORA_PKG)/cmd/protoc-gen-servora-conf@$(SERVORA_VERSION)
	@go install $(SERVORA_PKG)/cmd/protoc-gen-servora-mapper@$(SERVORA_VERSION)
	@echo "✓ Protoc plugins installed"

cli: ## Install CLI tools
	@echo "==> Installing CLI tools..."
	@go install github.com/go-kratos/kratos/cmd/kratos/v3@$(KRATOS_VERSION)
	@go install github.com/google/gnostic@$(GNOSTIC_VERSION)
	@go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@go install github.com/google/wire/cmd/wire@$(WIRE_VERSION)
	@go install entgo.io/ent/cmd/ent@$(ENT_VERSION)
	@go install github.com/air-verse/air@$(AIR_VERSION)
	@go install $(SERVORA_PKG)/cmd/svr@$(SERVORA_VERSION)
	@echo "✓ CLI tools installed"

api: $(API_TARGETS) ## Generate configured protobuf API code
	@echo "✓ API code generated"

api-go: ## Generate protobuf Go code
	@buf generate --template $(BUF_GO_GEN_TEMPLATE)

api-ts: ## Generate TypeScript API code where templates exist
	@if [ -f "$(BUF_TS_GEN_TEMPLATE)" ]; then \
		echo "==> Generating TypeScript via $(BUF_TS_GEN_TEMPLATE)"; \
		buf generate --template "$(BUF_TS_GEN_TEMPLATE)"; \
	fi
	@set -e; for mod in $(SERVICE_MODULES); do \
		tpl="$$mod/api/buf.typescript.gen.yaml"; \
		if [ -f "$$tpl" ]; then \
			echo "==> Generating TypeScript via $$tpl"; \
			buf generate --template "$$tpl"; \
		fi; \
	done

ent: ## Generate Ent code for services that define generators
	$(call run-in-service-dirs,gen.ent)
	@echo "✓ Ent code generated"

OPENFGA_MODEL ?= manifests/openfga/model/servora.fga
OPENFGA_TESTS ?= manifests/openfga/tests/servora.fga.yaml
OPENFGA_ENV_PREFIX ?= OPENFGA_
OPENFGA_API_URL ?= http://localhost:18080

openfga.init: ## Initialize OpenFGA store and model
	@svr openfga init --model $(OPENFGA_MODEL) --env-file $(ENV_FILE_PATH) --env-prefix $(OPENFGA_ENV_PREFIX) --api-url $(OPENFGA_API_URL)

openfga.model.validate: ## Validate OpenFGA model syntax
	@fga model validate --file $(OPENFGA_MODEL) --format fga
	@echo "✓ OpenFGA model valid"

openfga.model.test: openfga.model.validate ## Run OpenFGA model tests
	@fga model test --tests $(OPENFGA_TESTS)
	@echo "✓ OpenFGA model tests passed"

openfga.model.apply: openfga.model.test ## Apply OpenFGA model after validate/test
	@svr openfga model apply --model $(OPENFGA_MODEL) --env-file $(ENV_FILE_PATH) --env-prefix $(OPENFGA_ENV_PREFIX) --api-url $(OPENFGA_API_URL)
endif

ifeq ($(SERVORA_CONTEXT),service)
GEN_TARGETS := api $(GEN_TARGETS) gen.ent
RUN_DEPS := api $(RUN_DEPS)

.PHONY: api gen.ent

api: ## Generate repository Go API and current service API templates
	@$(MAKE) -C $(REPO_ROOT) api-go
	@if [ -f "./api/buf.typescript.gen.yaml" ]; then \
		cd $(REPO_ROOT) && buf generate --template $(SERVICE_MODULE)/api/buf.typescript.gen.yaml; \
	fi

gen.ent: ## Generate Ent code if this service defines a generator
	@if [ -f "./internal/data/generate.go" ]; then \
		go generate ./internal/data; \
	else \
		echo "No Ent generator found for $(SERVICE_NAME), skipping"; \
	fi
endif
