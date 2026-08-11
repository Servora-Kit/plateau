import "just/settings.just"

mod service 'just/services.just'
mod web 'just/webs.just'

ROOT_DIR := justfile_directory()
BUF_GO_GEN_TEMPLATE := env("BUF_GO_GEN_TEMPLATE", "buf.go.gen.yaml")
BUF_TS_GEN_TEMPLATE := env("BUF_TS_GEN_TEMPLATE", "buf.typescript.gen.yaml")
API_TS_PACKAGE := env("API_TS_PACKAGE", "@servora-platform/api")
SERVORA_PKG := env("SERVORA_PKG", "github.com/Servora-Kit/servora")
SERVORA_VERSION := env("SERVORA_VERSION", "v0.8.11")
PNPM := env("PNPM", "pnpm")
LINT_GOWORK := env("LINT_GOWORK", "auto")
COMPOSE := env("COMPOSE", "docker compose")
COMPOSE_FILES := env("COMPOSE_FILES", "-f docker-compose.yaml")
COMPOSE_SERVICES := env("COMPOSE_SERVICES", "")
ENV_FILE := env("ENV_FILE", join(ROOT_DIR, ".env"))
OPENFGA_ENV_PREFIX := env("OPENFGA_ENV_PREFIX", "PLATFORM_")
OPENFGA_API_URL := env("FGA_API_URL", env("OPENFGA_API_URL", ""))
OPENFGA_MODEL := env("OPENFGA_MODEL", "manifests/openfga/model/servora.fga")
OPENFGA_TESTS := env("OPENFGA_TESTS", "manifests/openfga/tests/servora.fga.yaml")
OPENFGA_SCRIPT_UNIX := env("OPENFGA_SCRIPT_UNIX", join(ROOT_DIR, "manifests/scripts/openfga.sh"))
OPENFGA_SCRIPT_WINDOWS := env("OPENFGA_SCRIPT_WINDOWS", join(ROOT_DIR, "manifests/scripts/openfga.ps1"))
VERSION_ENV := env("VERSION", "")
VERSION := if VERSION_ENV != "" { VERSION_ENV } else { `git describe --tags --always --dirty` }
GOVERSION := `go version`
DOCKER_TAG_VERSION_RAW := replace_regex(replace_regex(replace_regex(VERSION, "[^A-Za-z0-9_.-]+", "-"), "^[.-]+", ""), "[.-]+$", "")
DOCKER_TAG_VERSION := if DOCKER_TAG_VERSION_RAW == "" { "dev" } else { DOCKER_TAG_VERSION_RAW }

help:
    @just --list --list-submodules

env:
    @echo "ROOT_DIR: {{ ROOT_DIR }}"
    @echo "VERSION: {{ VERSION }}"
    @echo "GOVERSION: {{ GOVERSION }}"
    @echo "ENV_FILE: {{ ENV_FILE }}"
    @echo "LINT_GOWORK: {{ LINT_GOWORK }}"
    @echo "COMPOSE_FILES: {{ COMPOSE_FILES }}"

[unix]
init: plugin cli
    @"{{ PNPM }}" install --frozen-lockfile

[windows]
init: plugin cli
    @& "{{ PNPM }}" install --frozen-lockfile

plugin:
    @echo "==> Installing protoc plugins..."
    @go install google.golang.org/protobuf/cmd/protoc-gen-go@{{ env("PROTOC_GEN_GO_VERSION", "latest") }}
    @go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@{{ env("PROTOC_GEN_GO_GRPC_VERSION", "latest") }}
    @go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v3@{{ env("PROTOC_GEN_GO_HTTP_VERSION", "latest") }}
    @go install {{ SERVORA_PKG }}/cmd/protoc-gen-typescript-http@{{ SERVORA_VERSION }}
    @go install {{ SERVORA_PKG }}/cmd/protoc-gen-go-errors@{{ SERVORA_VERSION }}
    @go install github.com/google/gnostic/cmd/protoc-gen-openapi@{{ env("PROTOC_GEN_OPENAPI_VERSION", "latest") }}
    @go install github.com/envoyproxy/protoc-gen-validate@{{ env("PROTOC_GEN_VALIDATE_VERSION", "latest") }}
    @go install github.com/tx7do/go-wind-toolkit/protoc-gen-go-redact@{{ env("PROTOC_GEN_GO_REDACT_VERSION", "latest") }}
    @go install {{ SERVORA_PKG }}/cmd/protoc-gen-servora-audit@{{ SERVORA_VERSION }}
    @go install {{ SERVORA_PKG }}/cmd/protoc-gen-servora-conf@{{ SERVORA_VERSION }}
    @go install {{ SERVORA_PKG }}/cmd/protoc-gen-servora-crud@{{ SERVORA_VERSION }}
    @go install ./cmd/protoc-gen-servora-authz
    @go install ./cmd/protoc-gen-servora-authn

cli:
    @echo "==> Installing CLI tools..."
    @go install github.com/google/gnostic@{{ env("GNOSTIC_VERSION", "latest") }}
    @go install github.com/bufbuild/buf/cmd/buf@{{ env("BUF_VERSION", "latest") }}
    @go install github.com/golangci/golangci-lint/cmd/golangci-lint@{{ env("GOLANGCI_LINT_VERSION", "latest") }}
    @go install github.com/google/wire/cmd/wire@{{ env("WIRE_VERSION", "latest") }}
    @go install entgo.io/ent/cmd/ent@{{ env("ENT_VERSION", "latest") }}
    @go install github.com/air-verse/air@{{ env("AIR_VERSION", "latest") }}

api: api-go api-ts

[working-directory(ROOT_DIR)]
api-go:
    @buf generate --template "{{ BUF_GO_GEN_TEMPLATE }}"

[unix]
[working-directory(ROOT_DIR)]
api-ts:
    @buf generate --template "{{ BUF_TS_GEN_TEMPLATE }}"
    @"{{ PNPM }}" --filter {{ API_TS_PACKAGE }} build

[windows]
[working-directory(ROOT_DIR)]
api-ts:
    @buf generate --template "{{ BUF_TS_GEN_TEMPLATE }}"
    @& "{{ PNPM }}" --filter {{ API_TS_PACKAGE }} build

[unix]
[working-directory(ROOT_DIR)]
api-ts-check:
    @"{{ PNPM }}" --filter {{ API_TS_PACKAGE }} typecheck

[windows]
[working-directory(ROOT_DIR)]
api-ts-check:
    @& "{{ PNPM }}" --filter {{ API_TS_PACKAGE }} typecheck

ent: service::gen-ent
openapi: service::openapi
wire: service::wire

gen: api service::_gen

gen-fresh: gen-clean gen

[unix]
_gen-clean-files:
    rm -rf -- "{{ join(ROOT_DIR, "api/gen/go") }}" "{{ join(ROOT_DIR, "api/gen/ts") }}" "{{ join(ROOT_DIR, "api/gen/dist") }}"

[windows]
_gen-clean-files:
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue @("{{ join(ROOT_DIR, "api/gen/go") }}", "{{ join(ROOT_DIR, "api/gen/ts") }}", "{{ join(ROOT_DIR, "api/gen/dist") }}")

gen-clean: _gen-clean-files

[unix]
_clean-api-dist-files:
    rm -rf -- "{{ join(ROOT_DIR, "api/gen/dist") }}"

[windows]
_clean-api-dist-files:
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue "{{ join(ROOT_DIR, "api/gen/dist") }}"

build: gen service::_build web::_build

lint: api-ts-check lint-proto (service::lint LINT_GOWORK)

[working-directory(ROOT_DIR)]
lint-proto:
    @buf lint

clean: _clean-api-dist-files service::clean web::clean

compose-build: (_compose-build-image "example") (_compose-build-image "audit")

_compose-build-image service:
    @docker build --build-arg SERVICE_NAME={{ service }} --build-arg VERSION={{ VERSION }} -t servora-{{ service }}:{{ DOCKER_TAG_VERSION }} .
    @docker tag servora-{{ service }}:{{ DOCKER_TAG_VERSION }} servora-{{ service }}:latest

compose-up:
    @{{ COMPOSE }} {{ COMPOSE_FILES }} up -d {{ COMPOSE_SERVICES }}

compose-stop:
    @{{ COMPOSE }} {{ COMPOSE_FILES }} stop {{ COMPOSE_SERVICES }}

compose-down:
    @{{ COMPOSE }} {{ COMPOSE_FILES }} down --remove-orphans

compose-reset:
    @{{ COMPOSE }} {{ COMPOSE_FILES }} down --remove-orphans --volumes

compose-ps:
    @{{ COMPOSE }} {{ COMPOSE_FILES }} ps {{ COMPOSE_SERVICES }}

compose-logs:
    @{{ COMPOSE }} {{ COMPOSE_FILES }} logs -f {{ COMPOSE_SERVICES }}

openfga-model-validate:
    @fga model validate --file "{{ OPENFGA_MODEL }}" --format fga

openfga-model-test: openfga-model-validate
    @fga model test --tests "{{ OPENFGA_TESTS }}"

[unix]
openfga-init:
    @bash "{{ OPENFGA_SCRIPT_UNIX }}" init --model "{{ OPENFGA_MODEL }}" --env-file "{{ ENV_FILE }}" --env-prefix "{{ OPENFGA_ENV_PREFIX }}" --api-url "{{ OPENFGA_API_URL }}"

[windows]
openfga-init:
    @powershell -NoProfile -ExecutionPolicy Bypass -File "{{ OPENFGA_SCRIPT_WINDOWS }}" init -Model "{{ OPENFGA_MODEL }}" -EnvFile "{{ ENV_FILE }}" -EnvPrefix "{{ OPENFGA_ENV_PREFIX }}" -ApiUrl "{{ OPENFGA_API_URL }}"

[unix]
openfga-model-apply: openfga-model-test
    @bash "{{ OPENFGA_SCRIPT_UNIX }}" apply --model "{{ OPENFGA_MODEL }}" --env-file "{{ ENV_FILE }}" --env-prefix "{{ OPENFGA_ENV_PREFIX }}" --api-url "{{ OPENFGA_API_URL }}"

[windows]
openfga-model-apply: openfga-model-test
    @powershell -NoProfile -ExecutionPolicy Bypass -File "{{ OPENFGA_SCRIPT_WINDOWS }}" apply -Model "{{ OPENFGA_MODEL }}" -EnvFile "{{ ENV_FILE }}" -EnvPrefix "{{ OPENFGA_ENV_PREFIX }}" -ApiUrl "{{ OPENFGA_API_URL }}"
