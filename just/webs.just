import "settings.just"

mod example '../app/example/web/justfile'
mod test '../app/test/web/justfile'

_build: example::_build test::_build

build: example::build test::build

lint: example::lint test::lint

clean: example::clean test::clean
