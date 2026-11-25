import "settings.just"

mod admin '../app/admin/web/justfile'
mod iam '../app/iam/web/justfile'

install: admin::install

build: admin::build iam::build

lint: admin::lint iam::lint

test: admin::test iam::test
