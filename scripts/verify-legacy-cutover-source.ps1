param(
  [string]$LegacyDatabaseUrl = $env:LEGACY_DATABASE_URL,
  [string]$LegacyStorePath = $env:LEGACY_STORE_PATH,
  [string]$LegacyRoot = $env:LEGACY_ROOT,
  [switch]$SkipVideoFileCheck
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
if (-not $LegacyStorePath) {
  $LegacyStorePath = Join-Path $LegacyRoot "server\data\store.json"
}

$trackedCounts = @(
  "videos",
  "jobs",
  "analysisRuns",
  "rawVisionEvents",
  "events",
  "scenarioInstances",
  "scenarioTemplates",
  "reports"
)

$requiredPositiveCounts = @(
  "videos",
  "analysisRuns",
  "rawVisionEvents",
  "events",
  "scenarioInstances",
  "scenarioTemplates",
  "reports"
)

function Assert-PositiveCount([hashtable]$Counts, [string]$Key) {
  $actual = 0
  if ($Counts.ContainsKey($Key)) {
    $actual = [int]$Counts[$Key]
  }
  if ($actual -le 0) {
    throw "Legacy cutover source is incomplete: '$Key' count must be > 0, got $actual."
  }
}

function Assert-ReferencesKnownVideos([array]$Rows, [string]$CollectionName, [hashtable]$VideoIds) {
  $missing = New-Object System.Collections.Generic.List[string]
  foreach ($row in $Rows) {
    $videoId = [string]$row.videoId
    if ($videoId -and -not $VideoIds.ContainsKey($videoId)) {
      $missing.Add($videoId)
    }
  }
  if ($missing.Count -gt 0) {
    $sample = ($missing | Select-Object -Unique -First 5) -join ", "
    throw "$CollectionName references video ids missing from videos: $sample"
  }
}

function Resolve-LegacyFile([string]$FilePath) {
  if (-not $FilePath) {
    return $null
  }
  if ([System.IO.Path]::IsPathRooted($FilePath)) {
    return $FilePath
  }
  return Join-Path $LegacyRoot $FilePath
}

function Test-JsonStore() {
  if (-not (Test-Path $LegacyStorePath)) {
    throw "Legacy store not found: $LegacyStorePath"
  }
  $store = Get-Content -Path $LegacyStorePath -Raw | ConvertFrom-Json
  $counts = @{}
  foreach ($key in $trackedCounts) {
    $value = $store.$key
    $counts[$key] = if ($null -eq $value) { 0 } else { @($value).Count }
  }
  $counts["groundTruth"] = if ($null -eq $store.groundTruth) { 0 } else { @($store.groundTruth).Count }

  foreach ($key in $requiredPositiveCounts) {
    Assert-PositiveCount $counts $key
  }

  $videoIds = @{}
  foreach ($video in @($store.videos)) {
    if (-not $video.id) {
      throw "Legacy videos contains a row without id."
    }
    $videoIds[[string]$video.id] = $true
    if (-not $SkipVideoFileCheck) {
      $path = Resolve-LegacyFile ([string]$video.filePath)
      if (-not $path -or -not (Test-Path $path)) {
        throw "Legacy video file is missing for video '$($video.id)': $path"
      }
    }
  }

  foreach ($collection in @(
    "jobs",
    "analysisRuns",
    "events",
    "scenarioInstances",
    "reports",
    "scenarioGraphs",
    "groundTruth"
  )) {
    Assert-ReferencesKnownVideos @($store.$collection) $collection $videoIds
  }

  return $counts
}

function Test-LegacyDatabase() {
  $query = @"
SELECT json_build_object(
  'videos', (SELECT count(*) FROM "VideoRecording"),
  'jobs', (SELECT count(*) FROM "AnalysisJob"),
  'analysisRuns', (SELECT count(*) FROM "AnalysisRun"),
  'rawVisionEvents', (SELECT count(*) FROM "RawVisionEvent"),
  'events', (SELECT count(*) FROM "ActionEvent"),
  'scenarioInstances', (SELECT count(*) FROM "ScenarioInstance"),
  'scenarioTemplates', (SELECT count(*) FROM "ScenarioTemplate"),
  'reports', (SELECT count(*) FROM "AnalystReport")
)::text;
"@
  $psqlUrl = ConvertTo-PsqlConnectionString $LegacyDatabaseUrl
  $output = $query | docker run --rm -i postgres:16-alpine psql $psqlUrl -t -A
  if ($LASTEXITCODE -ne 0) {
    throw "Unable to inspect legacy database with psql."
  }
  $counts = @{}
  $jsonLines = @($output | Where-Object { $_.Trim().StartsWith("{") })
  if ($jsonLines.Count -eq 0) {
    throw "Unable to inspect legacy database: psql did not return JSON counts."
  }
  $json = $jsonLines[-1] | ConvertFrom-Json
  foreach ($key in $trackedCounts) {
    $counts[$key] = [int]$json.$key
  }
  foreach ($key in $requiredPositiveCounts) {
    Assert-PositiveCount $counts $key
  }
  Assert-LegacyDatabaseVideoFiles $psqlUrl
  return $counts
}

function Assert-LegacyDatabaseVideoFiles([string]$PsqlUrl) {
  if ($SkipVideoFileCheck) {
    return
  }
  $query = @"
SELECT COALESCE(json_agg(json_build_object('id',"id",'filePath',"filePath")),'[]'::json)::text
FROM "VideoRecording";
"@
  $output = $query | docker run --rm -i postgres:16-alpine psql $PsqlUrl -t -A
  if ($LASTEXITCODE -ne 0) {
    throw "Unable to inspect legacy database video files with psql."
  }
  $jsonLines = @($output | Where-Object { $_.Trim().StartsWith("[") })
  if ($jsonLines.Count -eq 0) {
    throw "Unable to inspect legacy database: psql did not return video file JSON."
  }
  $videos = $jsonLines[-1] | ConvertFrom-Json
  foreach ($video in @($videos)) {
    $path = Resolve-LegacyFile ([string]$video.filePath)
    if (-not $path -or -not (Test-Path $path)) {
      throw "Legacy video file is missing for video '$($video.id)': $path"
    }
  }
}

function ConvertTo-PsqlConnectionString([string]$Url) {
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

if ($LegacyDatabaseUrl) {
  $counts = Test-LegacyDatabase
  Write-Output "Legacy database source is sufficient for cutover preflight: $($counts | ConvertTo-Json -Compress)"
} else {
  $counts = Test-JsonStore
  Write-Output "Legacy JSON store source is sufficient for cutover preflight: $($counts | ConvertTo-Json -Compress)"
}
