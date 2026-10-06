COMPOSE = docker compose -f deploy/compose.dev.yaml

.PHONY: test test-integration build release release-legacy release-vben \
	fe-install fe-build fe-build-legacy fe-build-vben fe-gen-api-vben \
	fe-test fe-lint check-legacy-source check-frontend-selection serve help migrate lint
test:
	$(COMPOSE) up -d --wait postgres minio
	$(COMPOSE) run --rm dev go test -race ./...
test-integration:
	$(COMPOSE) up -d --wait postgres minio
	$(COMPOSE) run --rm dev go test -race -count=1 ./...
build:
	$(COMPOSE) run --rm dev go build -trimpath -o bin/imgnest ./cmd/imgnest
fe-install:
	$(COMPOSE) run --rm dev sh -c 'node scripts/check-legacy-source.mjs && cd web && pnpm install --frozen-lockfile && cd .. && node scripts/check-legacy-source.mjs'
fe-build: fe-build-legacy
fe-build-legacy:
	$(COMPOSE) run --rm dev sh -c 'node scripts/check-legacy-source.mjs && cd web && pnpm install --frozen-lockfile && pnpm build && touch dist/.gitkeep && cd .. && node scripts/check-legacy-source.mjs'
fe-build-vben:
	$(COMPOSE) run --rm dev sh -c 'node scripts/check-legacy-source.mjs && cd web-vben && pnpm install --frozen-lockfile && pnpm build && touch dist/.gitkeep && cd .. && node scripts/check-legacy-source.mjs'
fe-gen-api-vben:
	$(COMPOSE) run --rm dev sh -c 'cd web-vben && pnpm gen:api'
check-legacy-source:
	$(COMPOSE) run --rm dev node scripts/check-legacy-source.mjs
check-frontend-selection:
	$(COMPOSE) run --rm dev sh scripts/check-frontend-selection.sh
fe-test:
	$(COMPOSE) run --rm dev sh -c 'cd web && pnpm vitest run'
fe-lint:
	$(COMPOSE) run --rm dev sh -c 'cd web && pnpm typecheck && pnpm lint'
release: release-legacy
release-legacy: fe-build-legacy
	$(COMPOSE) run --rm dev go build -trimpath -o bin/imgnest-legacy ./cmd/imgnest
release-vben: fe-build-vben
	$(COMPOSE) run --rm dev go build -tags vben -trimpath -o bin/imgnest-vben ./cmd/imgnest
help:
	$(COMPOSE) run --rm dev go run ./cmd/imgnest --help
migrate:
	$(COMPOSE) run --rm dev go run ./cmd/imgnest migrate
serve:
	$(COMPOSE) run --rm --service-ports dev go run ./cmd/imgnest serve
lint:
	$(COMPOSE) run --rm dev sh -c 'go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...'
