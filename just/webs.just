import "settings.just"

mod admin '../app/admin/web/justfile'
mod iam '../app/iam/web/justfile'

build: admin::build iam::build

lint: admin::lint iam::lint
