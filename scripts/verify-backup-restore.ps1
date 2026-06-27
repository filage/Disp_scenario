param(
  [Parameter(Mandatory = $true)][string]$ManifestFile
)

$ErrorActionPreference = "Stop"
$manifestPath = (Resolve-Path $ManifestFile).Path
$backupDirectory = Split-Path -Parent $manifestPath
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
$databaseFile = Join-Path $backupDirectory $manifest.database.file
$minioFile = Join-Path $backupDirectory $manifest.objectStorage.file
$suffix = [Guid]::NewGuid().ToString("N").Substring(0, 10)
$postgresContainer = "analyst-restore-pg-$suffix"
$minioContainer = "analyst-restore-minio-$suffix"
$postgresVolume = "analyst-restore-pg-$suffix"
$minioVolume = "analyst-restore-minio-$suffix"
$network = "analyst-restore-$suffix"

function Assert-LastExitCode([string]$message) {
  if ($LASTEXITCODE -ne 0) { throw $message }
}

if ((Get-FileHash -Algorithm SHA256 -LiteralPath $databaseFile).Hash.ToLowerInvariant() -ne $manifest.database.sha256) {
  throw "PostgreSQL dump checksum mismatch"
}
if ((Get-FileHash -Algorithm SHA256 -LiteralPath $minioFile).Hash.ToLowerInvariant() -ne $manifest.objectStorage.sha256) {
  throw "MinIO archive checksum mismatch"
}

try {
  docker network create $network | Out-Null
  Assert-LastExitCode "creating verification network failed"
  docker volume create $postgresVolume | Out-Null
  Assert-LastExitCode "creating PostgreSQL verification volume failed"
  docker volume create $minioVolume | Out-Null
  Assert-LastExitCode "creating MinIO verification volume failed"

  docker run -d --name $postgresContainer --network $network `
    -e POSTGRES_USER=analyst -e POSTGRES_PASSWORD=analyst -e POSTGRES_DB=analyst `
    -v "${postgresVolume}:/var/lib/postgresql/data" postgres:16-alpine | Out-Null
  Assert-LastExitCode "starting verification PostgreSQL failed"

  $ready = $false
  foreach ($attempt in 1..60) {
    docker exec $postgresContainer pg_isready -U analyst -d analyst *> $null
    if ($LASTEXITCODE -eq 0) {
      $ready = $true
      break
    }
    Start-Sleep -Seconds 1
  }
  if (-not $ready) { throw "verification PostgreSQL did not become ready" }

  docker cp $databaseFile "${postgresContainer}:/tmp/restore.dump"
  Assert-LastExitCode "copying dump to verification PostgreSQL failed"
  docker exec $postgresContainer pg_restore `
    -U analyst -d analyst --clean --if-exists --no-owner `
    --exit-on-error /tmp/restore.dump
  Assert-LastExitCode "verification PostgreSQL restore failed"

  foreach ($property in $manifest.database.tables.PSObject.Properties) {
    $actual = docker exec $postgresContainer `
      psql -U analyst -d analyst -Atc "SELECT count(*) FROM $($property.Name)"
    Assert-LastExitCode "count query failed for $($property.Name)"
    if ([int64]($actual | Select-Object -Last 1) -ne [int64]$property.Value) {
      throw "row count mismatch for $($property.Name)"
    }
  }

  docker run --rm -v "${minioVolume}:/data" `
    -v "${backupDirectory}:/backup:ro" alpine:3.21 `
    tar -xzf "/backup/$($manifest.objectStorage.file)" -C /data
  Assert-LastExitCode "extracting verification MinIO archive failed"

  docker run -d --name $minioContainer --network $network `
    -e MINIO_ROOT_USER=analyst -e MINIO_ROOT_PASSWORD=analyst-secret `
    -v "${minioVolume}:/data" minio/minio:RELEASE.2025-04-22T22-12-26Z `
    server /data | Out-Null
  Assert-LastExitCode "starting verification MinIO failed"

  $minioReady = $false
  foreach ($attempt in 1..60) {
    docker run --rm --network $network `
      --entrypoint /bin/sh minio/mc:RELEASE.2025-04-16T18-13-26Z `
      -c "mc alias set local http://$minioContainer`:9000 analyst analyst-secret >/dev/null" *> $null
    if ($LASTEXITCODE -eq 0) {
      $minioReady = $true
      break
    }
    Start-Sleep -Seconds 1
  }
  if (-not $minioReady) { throw "verification MinIO did not become ready" }

  $objects = docker run --rm --network $network `
    --entrypoint /bin/sh minio/mc:RELEASE.2025-04-16T18-13-26Z `
    -c "set -eu; mc alias set local http://$minioContainer`:9000 analyst analyst-secret >/dev/null; mc ls --recursive --json local/$($manifest.objectStorage.bucket) | wc -l"
  Assert-LastExitCode "counting restored MinIO objects failed"
  if ([int64]($objects | Select-Object -Last 1) -ne [int64]$manifest.objectStorage.objects) {
    throw "restored MinIO object count mismatch"
  }

  Write-Output "Backup restore verification passed."
  Write-Output "PostgreSQL tables: $(@($manifest.database.tables.PSObject.Properties).Count)"
  Write-Output "MinIO objects: $($manifest.objectStorage.objects)"
} finally {
  docker rm -f $postgresContainer $minioContainer *> $null
  docker volume rm $postgresVolume $minioVolume *> $null
  docker network rm $network *> $null
}
