# Makefile for building servora micro service application
# This is a common Makefile template for all services in app/ directory

MKFILE_PATH := $(abspath $(lastword $(MAKEFILE_LIST)))
MKFILE_DIR  := $(dir $(MKFILE_PATH))
ENV_FILE    := $(MKFILE_DIR).env
REPO_ROOT   := $(patsubst %/,%,$(MKFILE_DIR))
ROOT_MAKE   := $(MAKE) -C $(REPO_ROOT)

# load environment variables from .env file if it exists
ifneq (,$(wildcard $(ENV_FILE)))
    include $(ENV_FILE)
    export
endif

GOPATH ?= $(shell go env GOPATH)
# GOVERSION is the current go version, e.g. go1.23.4
GOVERSION ?= $(shell go version | awk '{print $$3;}')

# Ensure GOPATH is set before running build process.
ifeq "$(GOPATH)" ""
  $(error Please set the environment variable GOPATH before running `make`)
endif

DEFAULT_VERSION ?= $(SERVICE_APP_VERSION)
REPO_GIT_DIR := $(REPO_ROOT)/.git

ifeq ($(OS),Windows_NT)
    IS_WINDOWS := TRUE
endif

ifneq (,$(wildcard $(REPO_GIT_DIR)))
	# CUR_TAG is the last git tag plus the delta from the current commit to the tag
	# e.g. v1.5.5-<nr of commits since>-g<current git sha>
	CUR_TAG ?= $(shell git describe --tags --first-parent 2>/dev/null || echo "dev")

	# LAST_TAG is the last git tag
    # e.g. v1.5.5
    LAST_TAG ?= $(shell git describe --match "v*" --abbrev=0 --tags --first-parent 2>/dev/null || echo "v0.0.1")

	# VERSION is the last git tag without the 'v'
	# e.g. 1.5.5
	VERSION ?= $(shell git describe --match "v*" --abbrev=0 --tags --first-parent 2>/dev/null | cut -c 2- || echo "0.0.1")
endif

CUR_TAG  ?= $(DEFAULT_VERSION)
LAST_TAG ?= v$(DEFAULT_VERSION)
VERSION  ?= $(DEFAULT_VERSION)

# GOFLAGS is the flags for the go compiler.
LDFLAGS ?= -X main.Version=$(VERSION) -X main.Name=$(SERVICE_NAME).service
GOFLAGS ?=
BUILD_CMD = @go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ./bin/ ./...

APP_RELATIVE_PATH := $(shell a=`basename $$PWD` && cd .. && b=`basename $$PWD` && echo $$b/$$a)
SERVICE_NAME      := $(shell a=`basename $$PWD` && cd .. && b=`basename $$PWD` && echo $$b)
APP_NAME          := $(shell echo $(APP_RELATIVE_PATH) | sed -En "s/\//-/p")

# Detect service-specific OpenAPI config file
# Format: buf.{service_name}.openapi.gen.yaml
OPENAPI_CONFIG := buf.$(SERVICE_NAME).openapi.gen.yaml
CONF ?= ./configs/local/
RUN_DEPS ?= api openapi

.PHONY: build _build clean gen wire api openapi run dev app help env gen.gorm gen.ent lint.go

# show environment variables
env:
	@echo "GOPATH: $(GOPATH)"
	@echo "GOVERSION: $(GOVERSION)"
	@echo "GOFLAGS: $(GOFLAGS)"
	@echo "LDFLAGS: $(LDFLAGS)"
	@echo "PROJECT_NAME: $(PROJECT_NAME)"
	@echo "SERVICE_APP_VERSION: $(SERVICE_APP_VERSION)"
	@echo "APP_RELATIVE_PATH: $(APP_RELATIVE_PATH)"
	@echo "SERVICE_NAME: $(SERVICE_NAME)"
	@echo "APP_NAME: $(APP_NAME)"
	@echo "CUR_TAG: $(CUR_TAG)"
	@echo "LAST_TAG: $(LAST_TAG)"
	@echo "VERSION: $(VERSION)"
	@echo "OPENAPI_CONFIG: $(OPENAPI_CONFIG)"
	@echo "REPO_ROOT: $(REPO_ROOT)"

# build golang application
build: gen _build

_build:
ifneq ("$(wildcard ./cmd)","")
	$(BUILD_CMD)
else
	@echo "No cmd directory found, skipping build for $(SERVICE_NAME)"
endif

# run application
run: $(RUN_DEPS)
	-@go run $(GOFLAGS) -ldflags "$(LDFLAGS)" ./cmd/server -conf $(CONF)

## dev: run application with Air hot-reload
dev: $(RUN_DEPS)
	@air

# build service app
app: build

# clean build files
clean:
	@go clean
	$(if $(IS_WINDOWS), del "coverage.out", rm -f "coverage.out")
	@rm -f openapi.yaml internal/assets/openapi.yaml

# generate code
gen: api openapi wire gen.ent

# generate GORM GEN PO and DAO code via centralized svr CLI
gen.gorm:
	@echo "Generating GORM DAO/PO..."
	@svr gen gorm $(SERVICE_NAME)

# generate Ent code, if internal/data/generate.go exists
gen.ent:
ifneq ("$(wildcard ./internal/data/generate.go)","")
	@go generate ./internal/data
endif

# generate wire code
wire:
ifneq ("$(wildcard ./cmd/server)","")
	@go run github.com/google/wire/cmd/wire ./cmd/server
else
	@echo "No cmd/server directory found, skipping wire for $(SERVICE_NAME)"
endif

# generate protobuf api code
api:
	@$(ROOT_MAKE) api-go
ifneq (,$(wildcard ./api/buf.typescript.gen.yaml))
	@cd $(REPO_ROOT) && buf generate --template app/$(APP_RELATIVE_PATH)/api/buf.typescript.gen.yaml
endif

# generate protobuf api OpenAPI v3 docs
openapi:
ifneq (,$(wildcard ./api/buf.openapi.gen.yaml))
	@cd $(REPO_ROOT) && buf generate --template app/$(APP_RELATIVE_PATH)/api/buf.openapi.gen.yaml
else
	@echo "No OpenAPI config found for $(SERVICE_NAME), skipping..."
endif

# run golangci-lint in this service module only
lint.go:
	@golangci-lint run

# show help
help:
	@echo ""
	@echo "Usage:"
	@echo " make [target]"
	@echo ""
	@echo "Targets:"
	@awk '/^[a-zA-Z\-_0-9\.]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "%-20s %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help
