# Load .env into every command run by make. Two things to know:
#  1. The leading "-" makes .env optional: no error if it does not exist.
#  2. Make precedence: values in .env OVERRIDE same-named variables exported in
#     the shell. So `LOG_LEVEL=debug make run` does NOT work when .env defines
#     LOG_LEVEL. Pass the value on the make command line instead:
#         make run LOG_LEVEL=debug
#     (command-line variables beat the makefile).
-include .env
export

# A bare `make` must never start a server.
.DEFAULT_GOAL := check

.PHONY: run test check db-up db-down db-psql

# Run the API locally (reads HTTP_ADDR, LOG_LEVEL, SHUTDOWN_TIMEOUT from .env).
run:
	go run ./cmd/api

# Run all tests with the race detector.
test:
	go test -race ./...

# Pre-report checks: formatting (fails and lists files), vet, tests.
check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: these files need formatting:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...
	go test -race ./...

# Start PostgreSQL in the background and wait until it is healthy.
db-up:
	docker compose up -d --wait

# Stop PostgreSQL, keeping the data volume.
db-down:
	docker compose down

# Open a psql session inside the container ($$ so the container's shell expands the variables).
db-psql:
	docker compose exec postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'
