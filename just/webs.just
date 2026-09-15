import "settings.just"

mod admin '../app/admin/web/justfile'
mod example '../app/example/web/justfile'
mod test '../app/test/web/justfile'
mod iam '../app/iam/web/justfile'

build: admin::build example::build test::build iam::build

lint: admin::lint example::lint test::lint iam::lint
