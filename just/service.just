import "settings.just"

COMPONENT_DIR := module_directory()
ROOT_DIR := clean(join(COMPONENT_DIR, "../../.."))
SERVICE_NAME := env("SERVICE_NAME", file_name(parent_directory(COMPONENT_DIR)))
SERVICE_MODULE := "app/" + file_name(parent_directory(COMPONENT_DIR)) + "/service"

VERSION_ENV := env("VERSION", "")
VERSION := if VERSION_ENV != "" { VERSION_ENV } else { `git describe --tags --always --dirty` }
GOVERSION := `go version`
CONF := env("CONF", "./configs/local/")
GOFLAGS := env("GOFLAGS", "")
LDFLAGS := env("LDFLAGS", "-X main.Version=" + VERSION + " -X main.Name=" + SERVICE_NAME + ".service")
JUST := just_executable()
OPENAPI_TEMPLATE := join(COMPONENT_DIR, "api/buf.openapi.gen.yaml")
TYPESCRIPT_TEMPLATE := join(COMPONENT_DIR, "api/buf.typescript.gen.yaml")
ENT_GENERATOR := join(COMPONENT_DIR, "internal/data/generate.go")
CMD_DIR := join(COMPONENT_DIR, "cmd")
SERVER_DIR := join(CMD_DIR, "server")

[unix]
help:
    @"{{ JUST }}" --justfile "{{ join(COMPONENT_DIR, "justfile") }}" --list

[windows]
help:
    @& "{{ JUST }}" --justfile "{{ join(COMPONENT_DIR, "justfile") }}" --list

env:
    @echo "COMPONENT_DIR: {{ COMPONENT_DIR }}"
    @echo "ROOT_DIR: {{ ROOT_DIR }}"
    @echo "SERVICE_MODULE: {{ SERVICE_MODULE }}"
    @echo "SERVICE_NAME: {{ SERVICE_NAME }}"
    @echo "VERSION: {{ VERSION }}"
    @echo "GOVERSION: {{ GOVERSION }}"
    @echo "CONF: {{ CONF }}"

[unix]
_root-api-go:
    @"{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" api-go

[windows]
_root-api-go:
    @& "{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" api-go

api: _root-api-go api-ts

[working-directory(ROOT_DIR)]
api-ts:
    @{{ if path_exists(TYPESCRIPT_TEMPLATE) { "buf generate --template \"" + join(SERVICE_MODULE, "api/buf.typescript.gen.yaml") + "\"" } else { "echo No TypeScript API template found for " + SERVICE_NAME + ", skipping" } }}

[working-directory(ROOT_DIR)]
openapi:
    @{{ if path_exists(OPENAPI_TEMPLATE) { "buf generate --template \"" + join(SERVICE_MODULE, "api/buf.openapi.gen.yaml") + "\"" } else { "echo No OpenAPI template found for " + SERVICE_NAME + ", skipping" } }}

[working-directory(COMPONENT_DIR)]
wire:
    @{{ if path_exists(SERVER_DIR) { "go run github.com/google/wire/cmd/wire ./cmd/server" } else { "echo No cmd/server directory found for " + SERVICE_NAME + ", skipping wire" } }}

[working-directory(COMPONENT_DIR)]
gen-ent:
    @{{ if path_exists(ENT_GENERATOR) { "go generate ./internal/data" } else { "echo No Ent generator found for " + SERVICE_NAME + ", skipping" } }}

_gen: openapi wire gen-ent

gen: api _gen

[working-directory(COMPONENT_DIR)]
_build:
    @{{ if path_exists(CMD_DIR) { "go build " + GOFLAGS + " -ldflags \"" + LDFLAGS + "\" -o ./bin/ ./..." } else { "echo No cmd directory found for " + SERVICE_NAME + ", skipping build" } }}

build: gen _build

[working-directory(COMPONENT_DIR)]
run: api openapi
    -@go run {{ GOFLAGS }} -ldflags "{{ LDFLAGS }}" ./cmd/server -conf "{{ CONF }}"

[working-directory(COMPONENT_DIR)]
dev: api openapi
    @air

app: build

[unix]
[working-directory(COMPONENT_DIR)]
_clean-files:
    go clean
    rm -f coverage.out openapi.yaml internal/assets/openapi.yaml
    rm -rf bin tmp

[windows]
[working-directory(COMPONENT_DIR)]
_clean-files:
    go clean
    Remove-Item -Force -ErrorAction SilentlyContinue @("coverage.out", "openapi.yaml", "internal/assets/openapi.yaml")
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue @("bin", "tmp")

clean: _clean-files

[unix]
[working-directory(COMPONENT_DIR)]
lint-go gowork="auto":
    @GOWORK="{{ gowork }}" golangci-lint run

[windows]
[working-directory(COMPONENT_DIR)]
lint-go gowork="auto":
    $env:GOWORK = "{{ gowork }}"; golangci-lint run
