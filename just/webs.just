import "settings.just"

mod example '../app/example/web/justfile'
mod test '../app/test/web/justfile'
mod iam '../app/iam/web/justfile'

build: example::build test::build iam::build

lint: example::lint test::lint iam::lint

clean: example::clean test::clean iam::clean
