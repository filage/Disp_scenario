param(
  [string]$OutputDirectory = ".\backups",
  [switch]$AllowOnline
)

$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$outputPath = if ([IO.Path]::IsPathRooted($OutputDirectory)) {
  [IO.Path]::GetFullPath($OutputDirectory)
} else {
  [IO.Path]::GetFullPath((Join-Path $projectRoot $OutputDirectory))
}

$tables = @(
  "organizations",
  "recordings",
  "analysis_runs",
  "analysis_jobs",
  "job_attempts",
  "outbox_events",
  "raw_vision_events",
  "action_events",
  "scenario_templates",
  "scenario_instances",
  "automation_candidates",
  "data_quality_issues",
  "analyst_reports",
  "scenario_graphs",
  "known_scenarios",
  "boundary_rules",
  "ground_truth_events",
  "settings_audit_log"
)

Push-Location $projectRoot
try {
  if (-not $AllowOnline) {
    $writers = @("api", "worker") | Where-Object {
      $container = docker compose ps -q $_
      $LASTEXITCODE -eq 0 -and $container
    }
    if ($writers.Count -gt 0) {
      throw "Stop api and worker before backup, or pass -AllowOnline after accepting a non-atomic snapshot."
    }
  }

  $postgresContainer = docker compose ps -q postgres
  $minioContainer = docker compose ps -q minio
  if (-not $postgresContainer -or -not $minioContainer) {
    throw "postgres and minio containers must exist"
  }

  New-Item -ItemType Directory -Force -Path $outputPath | Out-Null
  $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
  $databaseName = "postgres-$stamp.dump"
  $minioName = "minio-$stamp.tar.gz"
  $manifestName = "manifest-$stamp.json"
  $databaseFile = Join-Path $outputPath $databaseName
  $minioFile = Join-Path $outputPath $minioName
  $manifestFile = Join-Path $outputPath $manifestName
  $containerDump = "/tmp/$databaseName"

  docker compose exec -T postgres pg_dump `
    -U analyst -d analyst --format=custom --compress=9 `
    --file=$containerDump
  if ($LASTEXITCODE -ne 0) { throw "pg_dump failed" }

  docker cp "${postgresContainer}:$containerDump" $databaseFile
  if ($LASTEXITCODE -ne 0) { throw "copying PostgreSQL dump failed" }
  docker compose exec -T postgres rm -f $containerDump
  if ($LASTEXITCODE -ne 0) { throw "removing temporary PostgreSQL dump failed" }

  docker run --rm --volumes-from $minioContainer `
    -v "${outputPath}:/backup" alpine:3.21 `
    tar -czf "/backup/$minioName" -C /data .
  if ($LASTEXITCODE -ne 0) { throw "MinIO volume archive failed" }

  $tableCounts = [ordered]@{}
  foreach ($table in $tables) {
    $value = docker compose exec -T postgres `
      psql -U analyst -d analyst -Atc "SELECT count(*) FROM $table"
    if ($LASTEXITCODE -ne 0) { throw "count query failed for $table" }
    $tableCounts[$table] = [int64]($value | Select-Object -Last 1)
  }

  $network = docker inspect $minioContainer `
    --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{end}}'
  if ($LASTEXITCODE -ne 0 -or -not $network) {
    throw "cannot resolve MinIO Docker network"
  }
  $objectCount = docker run --rm --network $network `
    --entrypoint /bin/sh minio/mc:RELEASE.2025-04-16T18-13-26Z `
    -c "set -eu; mc alias set local http://minio:9000 analyst analyst-secret >/dev/null; mc ls --recursive --json local/analyst-recordings | wc -l"
  if ($LASTEXITCODE -ne 0) { throw "counting MinIO objects failed" }
  $objectCount = [int64]($objectCount | Select-Object -Last 1)
  if ($tableCounts.recordings -gt 0 -and $objectCount -le 0) {
    throw "MinIO object count is zero while recordings exist"
  }

  $manifest = [ordered]@{
    version = 1
    createdAt = (Get-Date).ToUniversalTime().ToString("o")
    database = [ordered]@{
      file = $databaseName
      sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $databaseFile).Hash.ToLowerInvariant()
      tables = $tableCounts
    }
    objectStorage = [ordered]@{
      file = $minioName
      sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $minioFile).Hash.ToLowerInvariant()
      bucket = "analyst-recordings"
      objects = $objectCount
    }
  }
  $manifest | ConvertTo-Json -Depth 6 |
    Set-Content -Encoding utf8 -LiteralPath $manifestFile

  Write-Output "Database: $databaseFile"
  Write-Output "MinIO: $minioFile"
  Write-Output "Manifest: $manifestFile"
} finally {
  Pop-Location
}
