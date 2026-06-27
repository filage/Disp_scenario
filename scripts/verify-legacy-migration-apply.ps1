param(
  [string]$LegacyDatabaseUrl = $env:LEGACY_DATABASE_URL,
  [string]$LegacyRoot = $env:LEGACY_ROOT
)

$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
function Read-EnvFileValue([string]$Path, [string]$Name) {
  if (-not (Test-Path $Path)) {
    return $null
  }
  $value = $null
  Get-Content -Path $Path | ForEach-Object {
    $line = $_.Trim()
    if ($line -and -not $line.StartsWith("#")) {
      $parts = $line.Split("=", 2)
      if ($parts.Count -eq 2 -and $parts[0] -eq $Name) {
        $value = $parts[1].Trim().Trim('"').Trim("'")
      }
    }
  }
  return $value
}
if (-not $LegacyRoot) {
  $LegacyRoot = (Resolve-Path (Join-Path $projectRoot "..\analyst-app")).Path
}
if (-not $LegacyDatabaseUrl) {
  $LegacyDatabaseUrl = Read-EnvFileValue (Join-Path $LegacyRoot ".env") "DATABASE_URL"
}

function ConvertTo-DockerConnectionString([string]$Url) {
  $builder = [System.UriBuilder]::new($Url)
  if ($builder.Host -eq "localhost" -or $builder.Host -eq "127.0.0.1" -or $builder.Host -eq "::1") {
    $builder.Host = "host.docker.internal"
  }
  if (-not $builder.Query) {
    return $builder.Uri.AbsoluteUri
  }
  $kept = New-Object System.Collections.Generic.List[string]
  foreach ($part in $builder.Query.TrimStart("?").Split("&")) {
    if (-not $part) {
      continue
    }
    $name = $part.Split("=", 2)[0]
    if ($name -eq "schema") {
      continue
    }
    $kept.Add($part)
  }
  $builder.Query = ($kept -join "&")
  return $builder.Uri.AbsoluteUri
}

function Wait-Postgres([string]$ContainerName) {
  for ($attempt = 1; $attempt -le 45; $attempt++) {
    docker exec $ContainerName pg_isready -U analyst -d analyst | Out-Null
    if ($LASTEXITCODE -eq 0) {
      return
    }
    Start-Sleep -Seconds 1
  }
  throw "Temporary PostgreSQL did not become ready."
}

function Wait-Minio([string]$NetworkName, [string]$ContainerName) {
  for ($attempt = 1; $attempt -le 45; $attempt++) {
    docker run --rm --network $NetworkName minio/mc:RELEASE.2025-04-16T18-13-26Z `
      alias set temp "http://${ContainerName}:9000" minioadmin minioadmin | Out-Null
    if ($LASTEXITCODE -eq 0) {
      return
    }
    Start-Sleep -Seconds 1
  }
  throw "Temporary MinIO did not become ready."
}

if (-not $LegacyDatabaseUrl) {
  throw "LEGACY_DATABASE_URL is required for full apply verification."
}

$suffix = [guid]::NewGuid().ToString("N").Substring(0, 10)
$network = "legacy-cutover-$suffix"
$postgres = "legacy-cutover-pg-$suffix"
$minio = "legacy-cutover-minio-$suffix"

try {
  docker network create $network | Out-Null

  docker run -d --name $postgres --network $network `
    -e POSTGRES_USER=analyst `
    -e POSTGRES_PASSWORD=analyst `
    -e POSTGRES_DB=analyst `
    postgres:16-alpine | Out-Null

  docker run -d --name $minio --network $network `
    -e MINIO_ROOT_USER=minioadmin `
    -e MINIO_ROOT_PASSWORD=minioadmin `
    minio/minio:RELEASE.2025-04-22T22-12-26Z server /data | Out-Null

  Wait-Postgres $postgres
  Wait-Minio $network $minio

  $targetUrl = "postgres://analyst:analyst@${postgres}:5432/analyst?sslmode=disable"
  docker run --rm --network $network `
    -v "${projectRoot}\backend\migrations:/migrations" `
    migrate/migrate:v4.18.3 `
    -path=/migrations `
    "-database=$targetUrl" `
    up
  if ($LASTEXITCODE -ne 0) {
    throw "Temporary target migrations failed."
  }

  $env:DATABASE_URL = $targetUrl
  $env:LEGACY_DATABASE_URL = ConvertTo-DockerConnectionString $LegacyDatabaseUrl
  $env:LEGACY_ROOT = "/legacy"
  $env:MIGRATION_APPLY = "true"
  $env:S3_ENDPOINT = "http://${minio}:9000"
  $env:S3_PUBLIC_ENDPOINT = "http://${minio}:9000"
  $env:S3_ACCESS_KEY = "minioadmin"
  $env:S3_SECRET_KEY = "minioadmin"
  $env:S3_BUCKET = "analyst-recordings"
  $env:S3_REGION = "us-east-1"
  $env:S3_USE_SSL = "false"

  docker run --rm --network $network `
    -v "${projectRoot}\backend:/workspace" `
    -v "${LegacyRoot}:/legacy" `
    -v analyst-v2-go-mod:/go/pkg/mod `
    -v analyst-v2-go-build:/root/.cache/go-build `
    -w /workspace `
    -e DATABASE_URL `
    -e LEGACY_DATABASE_URL `
    -e LEGACY_ROOT `
    -e MIGRATION_APPLY `
    -e S3_ENDPOINT `
    -e S3_PUBLIC_ENDPOINT `
    -e S3_ACCESS_KEY `
    -e S3_SECRET_KEY `
    -e S3_BUCKET `
    -e S3_REGION `
    -e S3_USE_SSL `
    golang:1.25-alpine go run ./cmd/migrate-legacy
  if ($LASTEXITCODE -ne 0) {
    throw "Legacy apply migration verification failed."
  }
} finally {
  docker rm -f $postgres $minio 2>$null | Out-Null
  docker network rm $network 2>$null | Out-Null
}
