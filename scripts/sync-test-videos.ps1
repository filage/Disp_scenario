param(
  [ValidateSet("Validate", "Upload", "Download")]
  [string]$Mode = "Validate",
  [string]$VideoDirectory,
  [string]$ManifestPath = "tests/fixtures/videos.json",
  [string]$EnvFile = ".env",
  [switch]$AllowInsecureEndpoint
)

$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$manifestFile = if ([IO.Path]::IsPathRooted($ManifestPath)) {
  [IO.Path]::GetFullPath($ManifestPath)
} else {
  [IO.Path]::GetFullPath((Join-Path $projectRoot $ManifestPath))
}

if (-not (Test-Path -LiteralPath $manifestFile -PathType Leaf)) {
  throw "Fixture manifest does not exist: $manifestFile"
}

$defaultVideoDirectory = if ($Mode -eq "Download") {
  Join-Path $projectRoot "tests/fixtures/videos"
} else {
  Join-Path (Split-Path $projectRoot -Parent) "output/playwright/videos"
}
$videoPath = if ([string]::IsNullOrWhiteSpace($VideoDirectory)) {
  [IO.Path]::GetFullPath($defaultVideoDirectory)
} elseif ([IO.Path]::IsPathRooted($VideoDirectory)) {
  [IO.Path]::GetFullPath($VideoDirectory)
} else {
  [IO.Path]::GetFullPath((Join-Path $projectRoot $VideoDirectory))
}

$manifest = Get-Content -Raw -LiteralPath $manifestFile | ConvertFrom-Json
if ($manifest.version -ne 1 -or @($manifest.videos).Count -eq 0) {
  throw "Unsupported or empty fixture manifest: $manifestFile"
}

function Assert-VideoFiles {
  param([object[]]$Videos, [string]$Directory)

  foreach ($video in $Videos) {
    $file = Join-Path $Directory $video.file
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
      throw "Fixture video does not exist: $file"
    }

    $item = Get-Item -LiteralPath $file
    if ($item.Length -ne [int64]$video.sizeBytes) {
      throw "Fixture size mismatch for $($video.file): expected $($video.sizeBytes), got $($item.Length)"
    }

    $hash = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($hash -ne $video.sha256) {
      throw "Fixture SHA-256 mismatch for $($video.file): expected $($video.sha256), got $hash"
    }
  }
}

function Assert-S3Environment {
  $required = @(
    "FIXTURE_S3_ENDPOINT",
    "FIXTURE_S3_BUCKET",
    "FIXTURE_S3_ACCESS_KEY_ID",
    "FIXTURE_S3_SECRET_ACCESS_KEY"
  )
  $missing = @($required | Where-Object {
    [string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($_, "Process"))
  })
  if ($missing.Count -gt 0) {
    throw "Missing fixture S3 environment variables: $($missing -join ', ')"
  }
  $placeholders = @($required | Where-Object {
    [Environment]::GetEnvironmentVariable($_, "Process") -match '<[^>]+>'
  })
  if ($placeholders.Count -gt 0) {
    throw "Fixture S3 environment variables still contain placeholders: $($placeholders -join ', ')"
  }
  if (-not $AllowInsecureEndpoint -and
      -not $env:FIXTURE_S3_ENDPOINT.StartsWith("https://", [StringComparison]::OrdinalIgnoreCase)) {
    throw "FIXTURE_S3_ENDPOINT must use HTTPS"
  }
}

function Import-FixtureEnvironment {
  param([string]$Path)

  $file = if ([IO.Path]::IsPathRooted($Path)) {
    [IO.Path]::GetFullPath($Path)
  } else {
    [IO.Path]::GetFullPath((Join-Path $projectRoot $Path))
  }
  if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
    return
  }

  $allowed = @(
    "FIXTURE_S3_ENDPOINT",
    "FIXTURE_S3_BUCKET",
    "FIXTURE_S3_ACCESS_KEY_ID",
    "FIXTURE_S3_SECRET_ACCESS_KEY"
  )
  foreach ($line in Get-Content -LiteralPath $file) {
    if ($line -notmatch '^\s*(?:\$env:)?(FIXTURE_S3_[A-Z_]+)\s*=\s*(.*?)\s*$') {
      continue
    }
    $name = $Matches[1]
    if ($name -notin $allowed -or
        -not [string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name, "Process"))) {
      continue
    }
    $value = $Matches[2].Trim()
    if ($value.Length -ge 2 -and
        (($value.StartsWith('"') -and $value.EndsWith('"')) -or
         ($value.StartsWith("'") -and $value.EndsWith("'")))) {
      $value = $value.Substring(1, $value.Length - 2)
    }
    [Environment]::SetEnvironmentVariable($name, $value, "Process")
  }
}

function Invoke-MinioClient {
  param(
    [ValidateSet("Upload", "Download")]
    [string]$Operation,
    [object]$Video,
    [string]$Directory
  )

  $containerCommand = if ($Operation -eq "Upload") {
    'set -eu; mc alias set fixtures "$FIXTURE_S3_ENDPOINT" "$FIXTURE_S3_ACCESS_KEY_ID" "$FIXTURE_S3_SECRET_ACCESS_KEY" --api S3v4 >/dev/null; mc cp "/fixtures/$FIXTURE_FILE" "fixtures/$FIXTURE_S3_BUCKET/$FIXTURE_OBJECT_KEY"; mc stat "fixtures/$FIXTURE_S3_BUCKET/$FIXTURE_OBJECT_KEY" >/dev/null'
  } else {
    'set -eu; mc alias set fixtures "$FIXTURE_S3_ENDPOINT" "$FIXTURE_S3_ACCESS_KEY_ID" "$FIXTURE_S3_SECRET_ACCESS_KEY" --api S3v4 >/dev/null; mc cp "fixtures/$FIXTURE_S3_BUCKET/$FIXTURE_OBJECT_KEY" "/fixtures/$FIXTURE_FILE"'
  }

  $dockerArguments = @(
    "run", "--rm",
    "-e", "FIXTURE_S3_ENDPOINT",
    "-e", "FIXTURE_S3_BUCKET",
    "-e", "FIXTURE_S3_ACCESS_KEY_ID",
    "-e", "FIXTURE_S3_SECRET_ACCESS_KEY",
    "-e", "FIXTURE_FILE=$($Video.file)",
    "-e", "FIXTURE_OBJECT_KEY=$($Video.objectKey)",
    "-v", "${Directory}:/fixtures",
    "--entrypoint", "/bin/sh",
    "minio/mc:RELEASE.2025-04-16T18-13-26Z",
    "-c", $containerCommand
  )

  & docker @dockerArguments
  if ($LASTEXITCODE -ne 0) {
    throw "$Operation failed for $($Video.file)"
  }
}

$videos = @($manifest.videos)
foreach ($video in $videos) {
  if (-not $video.objectKey.StartsWith("$($manifest.prefix)/", [StringComparison]::Ordinal)) {
    throw "Object key is outside manifest prefix: $($video.objectKey)"
  }
}

if ($Mode -eq "Validate") {
  Assert-VideoFiles -Videos $videos -Directory $videoPath
  Write-Output "Validated fixture videos: $($videos.Count)"
  exit 0
}

Import-FixtureEnvironment -Path $EnvFile
Assert-S3Environment
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
  throw "Docker is required to synchronize fixture videos"
}

if ($Mode -eq "Upload") {
  Assert-VideoFiles -Videos $videos -Directory $videoPath
  foreach ($video in $videos) {
    Invoke-MinioClient -Operation Upload -Video $video -Directory $videoPath
  }
  Write-Output "Uploaded fixture videos: $($videos.Count)"
  exit 0
}

New-Item -ItemType Directory -Force -Path $videoPath | Out-Null
foreach ($video in $videos) {
  Invoke-MinioClient -Operation Download -Video $video -Directory $videoPath
}
Assert-VideoFiles -Videos $videos -Directory $videoPath
Write-Output "Downloaded and validated fixture videos: $($videos.Count)"
