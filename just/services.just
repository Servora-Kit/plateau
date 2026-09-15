import "settings.just"

mod audit '../app/audit/service/justfile'
mod example '../app/example/service/justfile'
mod iam '../app/iam/service/justfile'

REGISTRY_DIR := module_directory()
ROOT_DIR := clean(join(REGISTRY_DIR, ".."))
JUST := just_executable()

_gen: audit::_gen example::_gen iam::_gen
_build: audit::_build example::_build iam::_build
# Type-level generation includes shared root API once.
gen: _root-api _gen

[unix]
_root-api:
    @"{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" api

[windows]
_root-api:
    @& "{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" api

build: gen _build

lint: audit::lint-go example::lint-go iam::lint-go

clean: audit::clean example::clean iam::clean

openapi: audit::openapi example::openapi iam::openapi
wire: audit::wire example::wire iam::wire
gen-ent: audit::gen-ent example::gen-ent iam::gen-ent
