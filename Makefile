COMPOSE = docker compose -f deploy/compose.dev.yaml

.PHONY: test test-integration build help migrate serve lint
test:
	$(COMPOSE) run --rm dev go test -race ./...
test-integration:
	$(COMPOSE) up -d --wait postgres
	$(COMPOSE) run --rm dev go test -race -count=1 ./...
build:
	$(COMPOSE) run --rm dev go build -trimpath -o bin/imgnest ./cmd/imgnest
help:
	$(COMPOSE) run --rm dev go run ./cmd/imgnest --help
migrate:
	$(COMPOSE) run --rm dev go run ./cmd/imgnest migrate
serve:
	$(COMPOSE) run --rm --service-ports dev go run ./cmd/imgnest serve
lint:
	$(COMPOSE) run --rm dev sh -c 'go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...'
