import "settings.just"

mod admin '../app/admin/service/justfile'
mod audit '../app/audit/service/justfile'
mod example '../app/example/service/justfile'
mod iam '../app/iam/service/justfile'
mod test '../app/test/service/justfile'

REGISTRY_DIR := module_directory()
ROOT_DIR := clean(join(REGISTRY_DIR, ".."))
JUST := just_executable()

_gen: admin::_gen audit::_gen example::_gen iam::_gen test::_gen
_build: admin::_build audit::_build example::_build iam::_build test::_build
# Type-level generation includes shared root API once.
gen: _root-api _gen

[unix]
_root-api:
    @"{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" api

[windows]
_root-api:
    @& "{{ JUST }}" --justfile "{{ join(ROOT_DIR, "justfile") }}" api

build: gen _build

lint: admin::lint-go audit::lint-go example::lint-go iam::lint-go test::lint-go

clean: admin::clean audit::clean example::clean iam::clean test::clean

openapi: admin::openapi audit::openapi example::openapi iam::openapi test::openapi
wire: admin::wire audit::wire example::wire iam::wire test::wire
gen-ent: admin::gen-ent audit::gen-ent example::gen-ent iam::gen-ent test::gen-ent
