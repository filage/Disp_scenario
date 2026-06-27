-- name: CreateAnalysisRun :one
INSERT INTO analysis_runs (
  organization_id,
  recording_id,
  provider,
  model,
  prompt_version,
  normalization_version,
  grouping_version,
  status,
  created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'QUEUED', $8)
RETURNING *;

-- name: ListAnalysisRuns :many
SELECT *
FROM analysis_runs
WHERE organization_id = $1
  AND ($2::uuid IS NULL OR recording_id = $2)
ORDER BY created_at DESC;

-- name: CreateAnalysisJob :one
INSERT INTO analysis_jobs (
  organization_id,
  recording_id,
  analysis_run_id,
  idempotency_key
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateOutboxEvent :one
INSERT INTO outbox_events (
  aggregate_type,
  aggregate_id,
  event_type,
  payload
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: LockOutboxEvents :many
SELECT *
FROM outbox_events
WHERE published_at IS NULL
  AND available_at <= now()
ORDER BY created_at
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxPublished :exec
UPDATE outbox_events
SET published_at = now(), attempts = attempts + 1, last_error = NULL
WHERE id = $1;

-- name: MarkOutboxFailed :exec
UPDATE outbox_events
SET attempts = attempts + 1,
    last_error = $2,
    available_at = now() + LEAST(interval '5 minutes', make_interval(secs => power(2, LEAST(attempts, 8))::int))
WHERE id = $1;

