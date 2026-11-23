import "just/settings.just"

mod service 'just/services.just'
mod web 'just/webs.just'

ROOT_DIR := justfile_directory()
JUST := just_executable()
BUF_GO_GEN_TEMPLATE := env("BUF_GO_GEN_TEMPLATE", "buf.go.gen.yaml")
BUF_TS_GEN_TEMPLATE := env("BUF_TS_GEN_TEMPLATE", "buf.typescript.gen.yaml")
BUF_BREAKING_AGAINST := env("BUF_BREAKING_AGAINST", ".git#ref=origin/main")
API_TS_PACKAGE := env("API_TS_PACKAGE", "@plateau/api")
PNPM := env("PNPM", "pnpm")
COMPOSE := env("COMPOSE", "docker compose")
COMPOSE_FILES := env("COMPOSE_FILES", "-f docker-compose.yaml")
COMPOSE_SERVICES := env("COMPOSE_SERVICES", "")
ENV_FILE := env("ENV_FILE", join(ROOT_DIR, ".env"))
OPENFGA_API_URL := env("FGA_API_URL", "")
OPENFGA_MODEL := env("OPENFGA_MODEL", "manifests/openfga/fga.mod")
OPENFGA_TESTS := env("OPENFGA_TESTS", "manifests/openfga/*.fga.yaml")
OPENFGA_SCRIPT_UNIX := env("OPENFGA_SCRIPT_UNIX", join(ROOT_DIR, "manifests/scripts/openfga/openfga.sh"))
OPENFGA_SCRIPT_WINDOWS := env("OPENFGA_SCRIPT_WINDOWS", join(ROOT_DIR, "manifests/scripts/openfga/openfga.ps1"))
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
    @echo "COMPOSE_FILES: {{ COMPOSE_FILES }}"

[unix]
init:
    @mise install
    @prek install
    @"{{ PNPM }}" install --frozen-lockfile

[windows]
init:
    @mise install
    @prek install
    @& "{{ PNPM }}" install --frozen-lockfile

api: api-go api-ts

[working-directory(ROOT_DIR)]
api-go:
    @buf generate --template "{{ BUF_GO_GEN_TEMPLATE }}"

[unix]
[working-directory(ROOT_DIR)]
api-ts:
    @buf generate --template "{{ BUF_TS_GEN_TEMPLATE }}"

[windows]
[working-directory(ROOT_DIR)]
api-ts:
    @buf generate --template "{{ BUF_TS_GEN_TEMPLATE }}"

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
[working-directory(ROOT_DIR)]
gen-check:
    #!/usr/bin/env bash
    set -euo pipefail
    GOWORK=off "{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" gen
    paths=(
        'api/gen/go'
        'api/gen/ts'
        ':(glob)**/wire_gen.go'
        ':(glob)app/*/service/internal/data/ent'
        ':(glob)app/*/service/**/openapi.yaml'
    )
    status=0
    if ! git diff --exit-code --stat -- "${paths[@]}"; then
        echo "Generated files are out of date (tracked diffs above)." >&2
        status=1
    fi
    untracked=$(git ls-files --others --exclude-standard -- "${paths[@]}")
    if [ -n "$untracked" ]; then
        echo "Untracked generated files detected:" >&2
        printf '%s\n' "$untracked" >&2
        status=1
    fi
    exit "$status"

[script("powershell.exe", "-NoLogo", "-NoProfile", "-Command")]
[windows]
[working-directory(ROOT_DIR)]
gen-check:
    $ErrorActionPreference = "Stop"
    $env:GOWORK = "off"
    & "{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" gen
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $paths = @(
        "api/gen/go",
        "api/gen/ts",
        ":(glob)**/wire_gen.go",
        ":(glob)app/*/service/internal/data/ent",
        ":(glob)app/*/service/**/openapi.yaml"
    )
    $failed = $false
    git diff --exit-code --stat -- @paths
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Generated files are out of date (tracked diffs above)."
        $failed = $true
    }
    $untracked = git ls-files --others --exclude-standard -- @paths
    if ($untracked) {
        Write-Error ("Untracked generated files detected:`n" + ($untracked -join "`n"))
        $failed = $true
    }
    if ($failed) { exit 1 }

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

build: gen service::_build web::build

lint: api-ts-check lint-proto lint-go

[working-directory(ROOT_DIR)]
test *args:
    @go test -race -count=1 {{ args }} ./...

[working-directory(ROOT_DIR)]
lint-go *args:
    @golangci-lint run {{ args }} ./...

[working-directory(ROOT_DIR)]
lint-proto *args:
    @buf lint {{ args }}

[working-directory(ROOT_DIR)]
fmt-proto-check *args:
    @buf format -d --exit-code {{ args }}

[working-directory(ROOT_DIR)]
breaking *args:
    @buf breaking --against "{{ BUF_BREAKING_AGAINST }}" {{ args }}

clean: _clean-api-dist-files service::clean

compose-build: (_compose-build-image "audit")

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
    @fga model validate --file "{{ OPENFGA_MODEL }}"

openfga-model-test: openfga-model-validate
    @fga model test --tests "{{ OPENFGA_TESTS }}"

[unix]
openfga-init: openfga-model-test
    @bash "{{ OPENFGA_SCRIPT_UNIX }}" init --model "{{ OPENFGA_MODEL }}" --env-file "{{ ENV_FILE }}" --api-url "{{ OPENFGA_API_URL }}"

[windows]
openfga-init: openfga-model-test
    @powershell -NoProfile -ExecutionPolicy Bypass -File "{{ OPENFGA_SCRIPT_WINDOWS }}" init -Model "{{ OPENFGA_MODEL }}" -EnvFile "{{ ENV_FILE }}" -ApiUrl "{{ OPENFGA_API_URL }}"

[unix]
openfga-model-apply: openfga-model-test
    @bash "{{ OPENFGA_SCRIPT_UNIX }}" apply --model "{{ OPENFGA_MODEL }}" --env-file "{{ ENV_FILE }}" --api-url "{{ OPENFGA_API_URL }}"

[windows]
openfga-model-apply: openfga-model-test
    @powershell -NoProfile -ExecutionPolicy Bypass -File "{{ OPENFGA_SCRIPT_WINDOWS }}" apply -Model "{{ OPENFGA_MODEL }}" -EnvFile "{{ ENV_FILE }}" -ApiUrl "{{ OPENFGA_API_URL }}"
