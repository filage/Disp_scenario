GO_MOD_CACHE_VOLUME?=analyst-v2-go-mod
GO_BUILD_CACHE_VOLUME?=analyst-v2-go-build
TESTCONTAINERS_HOST_OVERRIDE?=host.docker.internal

DOCKER_GO=docker run --rm -v "$(CURDIR)/backend:/workspace" -v "$(GO_MOD_CACHE_VOLUME):/go/pkg/mod" -v "$(GO_BUILD_CACHE_VOLUME):/root/.cache/go-build" -w /workspace golang:1.25-alpine
DOCKER_GO_TESTCONTAINERS=docker run --rm -v "$(CURDIR)/backend:/workspace" -v /var/run/docker.sock:/var/run/docker.sock -v "$(GO_MOD_CACHE_VOLUME):/go/pkg/mod" -v "$(GO_BUILD_CACHE_VOLUME):/root/.cache/go-build" -e TESTCONTAINERS_HOST_OVERRIDE="$(TESTCONTAINERS_HOST_OVERRIDE)" -w /workspace golang:1.25-alpine

.PHONY: setup dev up down build test test-unit test-integration test-e2e test-e2e-full lint format generate migrate-up migrate-down migrate-legacy verify-legacy-cutover-source verify-legacy-migration-apply verify-plan-e2e-coverage sqlc openapi verify-openapi-contract verify-no-js verify-sample-videos security backup verify-backup test-redis-recovery test-s3-failure test-postgres-failure test-performance-smoke

setup:
	cd frontend && npm install

dev:
	docker compose up --build

up:
	docker compose up -d --build

down:
	docker compose down

build:
	cd frontend && npm run build
	$(DOCKER_GO) go build ./...

test:
	cd frontend && npm test
	$(DOCKER_GO) go test ./...

test-unit:
	$(DOCKER_GO) go test ./internal/domain/...

test-integration:
	$(DOCKER_GO_TESTCONTAINERS) go test -tags=integration ./internal/...

test-e2e:
	cd frontend && npm run test:e2e

test-e2e-full:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-e2e-full.ps1

lint:
	cd frontend && npm run lint
	$(DOCKER_GO) go vet ./...
	$(DOCKER_GO) go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run
	$(DOCKER_GO) go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...

format:
	cd frontend && npm run format
	$(DOCKER_GO) gofmt -w .

generate: sqlc openapi

migrate-up:
	docker compose run --rm migrate up

migrate-down:
	docker compose run --rm migrate down 1

migrate-legacy:
	$(DOCKER_GO) go run ./cmd/migrate-legacy

verify-legacy-cutover-source:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/verify-legacy-cutover-source.ps1

verify-legacy-migration-apply:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/verify-legacy-migration-apply.ps1

verify-plan-e2e-coverage:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/verify-plan-e2e-coverage.ps1

sqlc:
	docker run --rm -v "$(CURDIR)/backend:/src" -w /src sqlc/sqlc:1.29.0 generate

openapi:
	cd frontend && npm run generate:api
	docker run --rm -v "$(CURDIR):/repo" -w /repo/backend golang:1.25-alpine go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.1 --config oapi-codegen.yaml ../api/openapi.yaml

verify-openapi-contract:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/verify-openapi-contract.ps1

verify-no-js:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/verify-no-js.ps1

verify-sample-videos:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/verify-sample-videos.ps1

security:
	$(DOCKER_GO) go run golang.org/x/vuln/cmd/govulncheck@v1.4.0 ./...

backup:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/backup.ps1

verify-backup:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/verify-backup-restore.ps1 -ManifestFile "$(MANIFEST)"

test-redis-recovery:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-redis-recovery.ps1

test-s3-failure:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-s3-failure.ps1

test-postgres-failure:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-postgres-failure.ps1

test-performance-smoke:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-performance-smoke.ps1
