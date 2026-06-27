param(
  [Parameter(Mandatory = $true)][string]$ManifestFile,
  [switch]$ConfirmDataReplacement
)

$ErrorActionPreference = "Stop"

if (-not $ConfirmDataReplacement) {
  throw "Restore replaces the current PostgreSQL database and MinIO volume. Pass -ConfirmDataReplacement explicitly."
}

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$manifestPath = (Resolve-Path $ManifestFile).Path
$backupDirectory = Split-Path -Parent $manifestPath
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
$databaseFile = Join-Path $backupDirectory $manifest.database.file
$minioFile = Join-Path $backupDirectory $manifest.objectStorage.file

foreach ($file in @($databaseFile, $minioFile)) {
  if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
    throw "Backup artifact is missing: $file"
  }
}

if ((Get-FileHash -Algorithm SHA256 -LiteralPath $databaseFile).Hash.ToLowerInvariant() -ne $manifest.database.sha256) {
  throw "PostgreSQL dump checksum mismatch"
}
if ((Get-FileHash -Algorithm SHA256 -LiteralPath $minioFile).Hash.ToLowerInvariant() -ne $manifest.objectStorage.sha256) {
  throw "MinIO archive checksum mismatch"
}

Push-Location $projectRoot
try {
  $runningWriters = @("api", "worker", "frontend") | Where-Object {
    $container = docker compose ps -q $_
    if (-not $container) { return $false }
    (docker inspect $container --format "{{.State.Running}}") -eq "true"
  }
  if ($runningWriters.Count -gt 0) {
    throw "Stop api, worker, and frontend before restore."
  }

  $postgresContainer = docker compose ps -q postgres
  $minioContainer = docker compose ps -q minio
  if (-not $postgresContainer -or -not $minioContainer) {
    throw "postgres and minio containers must exist"
  }
  if ((docker inspect $postgresContainer --format "{{.State.Running}}") -ne "true") {
    throw "postgres must be running during restore"
  }
  if ((docker inspect $minioContainer --format "{{.State.Running}}") -eq "true") {
    throw "minio must be stopped before restore"
  }

  $containerDump = "/tmp/$($manifest.database.file)"
  docker cp $databaseFile "${postgresContainer}:$containerDump"
  if ($LASTEXITCODE -ne 0) { throw "copying PostgreSQL dump failed" }
  docker compose exec -T postgres pg_restore `
    -U analyst -d analyst --clean --if-exists --no-owner `
    --exit-on-error $containerDump
  if ($LASTEXITCODE -ne 0) { throw "PostgreSQL restore failed" }
  docker compose exec -T postgres rm -f $containerDump
  if ($LASTEXITCODE -ne 0) { throw "removing temporary PostgreSQL dump failed" }

  docker run --rm --volumes-from $minioContainer `
    -v "${backupDirectory}:/backup:ro" alpine:3.21 `
    sh -c "find /data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + && tar -xzf '/backup/$($manifest.objectStorage.file)' -C /data"
  if ($LASTEXITCODE -ne 0) { throw "MinIO restore failed" }

  Write-Output "Restore completed and checksums verified."
} finally {
  Pop-Location
}
