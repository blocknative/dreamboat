build:
	go build ./cmd/dreamboat

build-migration:
	go build -o migration-postgres ./cmd/migration/postgres

# Mock testing
mocks: clean-mocks
# This roundabout call to 'go generate' allows us to:
#  - use modules
#  - prevent grep missing (totally fine) from causing nonzero exit 
	@find . -name '*.go' | xargs -I{} grep -l '//go:generate' {} | xargs -I{} -P 10 go generate {}

clean-mocks:
	@find . -name 'mocks.go' | xargs -I{} rm {}
