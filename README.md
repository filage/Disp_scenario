# DispScenario Analyst

DispScenario Analyst is an application for analyzing scenario video recordings. It combines video upload and storage, automated analysis, event normalization, scenario grouping, QA workflows, reporting, and observability in one local or production-like stack.

## Stack

- Frontend: Next.js, React, TypeScript.
- Backend: Go API and worker.
- Data: PostgreSQL, Redis/Asynq, S3-compatible storage with MinIO.
- Observability: Prometheus, Grafana, Loki, Alloy.
- Tooling: Docker Compose, OpenAPI code generation, sqlc, Playwright, Vitest.

## Architecture

```text
Browser -> Next.js -> Go API -> PostgreSQL
                    |  |-> signed upload/playback -> S3/MinIO
                    |  |-> transactional outbox -> Redis/Asynq
                    |
                    +-> Go worker
                         |-> ffprobe/ffmpeg
                         |-> Gemini provider
                         +-> deterministic analysis pipeline
```

PostgreSQL stores recording metadata, analysis runs, normalized events, scenarios, QA decisions, and reports. Redis is used for background job delivery. S3/MinIO stores video files and evidence frames. The API and worker are separate processes built from the same Go module.

## Features

- Video recording ingestion with signed upload and playback URLs.
- Gemini-based video analysis with structured event extraction.
- Deterministic normalization of events, actions, boundaries, and metrics.
- Scenario grouping, timeline views, QA review, and report generation.
- Operational dashboards, metrics, logs, backup, restore, and smoke checks.
- Role-aware API authentication with local auth-disabled mode and OIDC support.

## Quick Start

Requirements: Docker Desktop with Compose v2 and at least 6 GB of free memory.

```powershell
Copy-Item .env.example .env
docker compose up -d --build
docker compose ps
```

Services:

- frontend: `http://localhost:3000`
- API health: `http://localhost:8787/health`
- MinIO console: `http://localhost:9001`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3001` (`admin` / `analyst`)

`GEMINI_API_KEY` is required for the API and worker. The model is configured with `GEMINI_MODEL`.

## Development

Common commands are available through `Makefile`:

```powershell
make generate
make test
make lint
make security
make verify-no-js
make verify-plan-e2e-coverage
```

OpenAPI generation produces TypeScript client types and Go strict server types. SQL queries generate the `backend/internal/database/db` package.

Frontend commands can be run from `frontend`:

```powershell
npm install
npm run dev
npm run lint
npm run test
npm run test:e2e
```

Backend commands can be run from `backend`:

```powershell
go test ./...
go test -race ./...
```

Source `.js`, `.jsx`, `.cjs`, and `.mjs` files are intentionally not used. Compiled artifacts, local outputs, dependencies, backups, and environment files are not committed.

## Full-Stack E2E

The full-stack E2E flow verifies upload to S3, job scheduling through Redis/Asynq, real Gemini analysis, timeline output, QA fragments, reports, exports, Loki correlation logs, cleanup, and unsupported upload handling.

```powershell
make test-e2e-full
```

CI can provide the real-video fixture through `E2E_REAL_VIDEO_URL`.

## Authentication

Local Compose uses `AUTH_DISABLED=true` and creates a principal with `admin`, `analyst`, and `viewer` roles.

For OIDC:

```dotenv
AUTH_DISABLED=false
OIDC_ISSUER=https://identity.example.com/
OIDC_CLIENT_ID=dispscenario-analyst
```

The Go API validates JWT signature, issuer, audience, and expiry through OIDC discovery/JWKS. The `roles` or `role` claim must contain `viewer`, `analyst`, or `admin`.

## Backup and Restore

```powershell
./scripts/backup.ps1
./scripts/verify-backup-restore.ps1 -ManifestFile ./backups/manifest-<timestamp>.json
./scripts/restore.ps1 `
  -ManifestFile ./backups/manifest-<timestamp>.json `
  -ConfirmDataReplacement
```

Backup creates a PostgreSQL dump, MinIO volume archive, and manifest with SHA-256 checksums and object/table counters. Restore requires explicit confirmation, validates checksums, and replaces the current PostgreSQL database and MinIO volume.
