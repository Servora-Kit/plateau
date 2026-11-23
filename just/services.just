import "settings.just"

mod admin '../app/admin/service/justfile'
mod audit '../app/audit/service/justfile'
mod iam '../app/iam/service/justfile'

REGISTRY_DIR := module_directory()
ROOT_DIR := clean(join(REGISTRY_DIR, ".."))
JUST := just_executable()

_gen: admin::_gen audit::_gen iam::_gen
_build: admin::_build audit::_build iam::_build
# Type-level generation includes shared root API once.
gen: _root-api _gen

[unix]
_root-api:
    @"{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" api

[windows]
_root-api:
    @& "{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" api

build: gen _build

lint: admin::lint-go audit::lint-go iam::lint-go

clean: admin::clean audit::clean iam::clean

openapi: admin::openapi audit::openapi iam::openapi
wire: admin::wire audit::wire iam::wire
gen-ent: admin::gen-ent audit::gen-ent iam::gen-ent
