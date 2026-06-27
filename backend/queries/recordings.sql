-- name: ListRecordings :many
SELECT *
FROM recordings
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: GetRecording :one
SELECT *
FROM recordings
WHERE id = $1 AND organization_id = $2;

-- name: CreateRecording :one
INSERT INTO recordings (
  organization_id,
  original_name,
  mime_type,
  size_bytes,
  status,
  object_key,
  created_by
) VALUES ($1, $2, $3, $4, 'PENDING_UPLOAD', $5, $6)
RETURNING *;

-- name: CompleteRecordingUpload :one
UPDATE recordings
SET status = 'UPLOADED', updated_at = now(), updated_by = $3
WHERE id = $1 AND organization_id = $2
RETURNING *;

-- name: UpdateRecordingStatus :exec
UPDATE recordings
SET status = $3, updated_at = now()
WHERE id = $1 AND organization_id = $2;

-- name: DeleteRecording :one
DELETE FROM recordings
WHERE id = $1 AND organization_id = $2
RETURNING *;

