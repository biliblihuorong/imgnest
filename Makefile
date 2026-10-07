COMPOSE = docker compose -f deploy/compose.dev.yaml

.PHONY: test test-integration build release fe-install fe-build fe-gen-api 	fe-test fe-lint serve help migrate lint
test:
	$(COMPOSE) up -d --wait postgres minio
	$(COMPOSE) run --rm dev go test -race ./...
test-integration:
	$(COMPOSE) up -d --wait postgres minio
	$(COMPOSE) run --rm dev go test -race -count=1 ./...
build:
	$(COMPOSE) run --rm dev go build -trimpath -o bin/imgnest ./cmd/imgnest
fe-install:
	$(COMPOSE) run --rm dev sh -c 'cd web-vben && pnpm install --frozen-lockfile'
fe-build:
	$(COMPOSE) run --rm dev sh -c 'cd web-vben && pnpm install --frozen-lockfile && pnpm build && touch dist/.gitkeep'
fe-gen-api:
	$(COMPOSE) run --rm dev sh -c 'cd web-vben && pnpm gen:api'
fe-test:
	$(COMPOSE) run --rm dev sh -c 'cd web-vben && pnpm test'
fe-lint:
	$(COMPOSE) run --rm dev sh -c 'cd web-vben && pnpm typecheck && pnpm lint'
release: fe-build
	$(COMPOSE) run --rm dev go build -trimpath -o bin/imgnest ./cmd/imgnest
help:
	$(COMPOSE) run --rm dev go run ./cmd/imgnest --help
migrate:
	$(COMPOSE) run --rm dev go run ./cmd/imgnest migrate
serve:
	$(COMPOSE) run --rm --service-ports dev go run ./cmd/imgnest serve
lint:
	$(COMPOSE) run --rm dev sh -c 'go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...'
