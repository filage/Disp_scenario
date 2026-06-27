param(
  [string]$ApiUrl = "http://localhost:8787"
)

$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$excludedRecording = "20260512_081010_1x92893"
$correlationID = "redis-recovery-$([Guid]::NewGuid().ToString('N'))"
$recordingID = $null
$runID = $null
$jobID = $null

function Query-Scalar([string]$sql) {
  $value = docker compose exec -T postgres `
    psql -U analyst -d analyst -Atc $sql
  if ($LASTEXITCODE -ne 0) { throw "PostgreSQL query failed" }
  return ($value | Select-Object -Last 1)
}

Push-Location $projectRoot
try {
  $recordingID = Query-Scalar @"
SELECT id
FROM recordings
WHERE status = 'ANALYZED'
  AND original_name NOT LIKE '$excludedRecording%'
ORDER BY duration_sec NULLS LAST, created_at
LIMIT 1
"@
  if (-not $recordingID) { throw "No eligible analyzed recording found" }

  docker compose stop worker redis
  if ($LASTEXITCODE -ne 0) { throw "Stopping Redis/worker failed" }

  $run = Invoke-RestMethod `
    -Method Post `
    -Uri "$ApiUrl/v1/recordings/$recordingID/analysis-runs" `
    -Headers @{"X-Correlation-ID" = $correlationID}
  $runID = $run.id
  if (-not $runID) { throw "Analysis run was not created while Redis was unavailable" }

  $jobID = Query-Scalar "SELECT id FROM analysis_jobs WHERE analysis_run_id = '$runID'"
  $failedPublicationObserved = $false
  foreach ($attempt in 1..20) {
    $attempts = [int](Query-Scalar "SELECT attempts FROM outbox_events WHERE aggregate_id = '$jobID'")
    if ($attempts -gt 0) {
      $failedPublicationObserved = $true
      break
    }
    Start-Sleep -Seconds 1
  }
  if (-not $failedPublicationObserved) {
    throw "Outbox did not record a failed Redis publication"
  }

  $healthFailure = $false
  try {
    Invoke-WebRequest -UseBasicParsing "$ApiUrl/health" | Out-Null
  } catch {
    if ($_.Exception.Response.StatusCode.value__ -eq 503) {
      $healthFailure = $true
    }
  }
  if (-not $healthFailure) { throw "Health endpoint did not report Redis outage" }

  docker compose start redis
  if ($LASTEXITCODE -ne 0) { throw "Starting Redis failed" }

  $published = $false
  foreach ($attempt in 1..40) {
    $publishedAt = Query-Scalar "SELECT COALESCE(published_at::text, '') FROM outbox_events WHERE aggregate_id = '$jobID'"
    if ($publishedAt) {
      $published = $true
      break
    }
    Start-Sleep -Seconds 1
  }
  if (-not $published) { throw "Outbox event was not published after Redis recovery" }

  $storedCorrelation = Query-Scalar "SELECT COALESCE(correlation_id, '') FROM analysis_jobs WHERE id = '$jobID'"
  if ($storedCorrelation -ne $correlationID) {
    throw "Correlation ID was not persisted on the business job"
  }

  docker compose exec -T postgres psql -U analyst -d analyst -v ON_ERROR_STOP=1 -c @"
BEGIN;
DELETE FROM outbox_events WHERE aggregate_id = '$jobID';
DELETE FROM analysis_jobs WHERE id = '$jobID';
DELETE FROM analysis_runs WHERE id = '$runID';
UPDATE recordings SET status = 'ANALYZED', updated_at = now() WHERE id = '$recordingID';
COMMIT;
"@
  if ($LASTEXITCODE -ne 0) { throw "Cleaning recovery test rows failed" }
  $jobID = $null
  $runID = $null

  docker compose start worker
  if ($LASTEXITCODE -ne 0) { throw "Starting worker failed" }

  Write-Output "Redis outage and outbox recovery verification passed."
  Write-Output "Correlation ID: $correlationID"
  Write-Output "Recording ID: $recordingID"
} finally {
  $cleanupErrorActionPreference = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  try {
    if ($runID -or $jobID) {
      docker compose exec -T postgres psql -U analyst -d analyst -v ON_ERROR_STOP=1 -c @"
BEGIN;
DELETE FROM outbox_events WHERE aggregate_id = NULLIF('$jobID', '')::uuid;
DELETE FROM analysis_jobs WHERE id = NULLIF('$jobID', '')::uuid;
DELETE FROM analysis_runs WHERE id = NULLIF('$runID', '')::uuid;
UPDATE recordings SET status = 'ANALYZED', updated_at = now() WHERE id = NULLIF('$recordingID', '')::uuid;
COMMIT;
"@ *> $null
    }
    docker compose start redis worker *> $null
  } finally {
    $ErrorActionPreference = $cleanupErrorActionPreference
    Pop-Location
  }
}
