param(
  [string]$WorkspaceRoot = "..",
  [string]$ExcludedRecordingPrefix = "20260512_081010_1x92893"
)

$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$workspacePath = if ([IO.Path]::IsPathRooted($WorkspaceRoot)) {
  [IO.Path]::GetFullPath($WorkspaceRoot)
} else {
  [IO.Path]::GetFullPath((Join-Path $projectRoot $WorkspaceRoot))
}

if (-not (Test-Path -LiteralPath $workspacePath -PathType Container)) {
  throw "Workspace root does not exist: $workspacePath"
}

$sampleVideos = Get-ChildItem -LiteralPath $workspacePath -File |
  Where-Object { $_.Extension -in @(".mp4", ".webm") } |
  Sort-Object Name

$allowedVideos = @($sampleVideos | Where-Object {
  -not $_.BaseName.StartsWith($ExcludedRecordingPrefix, [StringComparison]::Ordinal)
})
$excludedVideos = @($sampleVideos | Where-Object {
  $_.BaseName.StartsWith($ExcludedRecordingPrefix, [StringComparison]::Ordinal)
})

if ($allowedVideos.Count -eq 0) {
  throw "No allowed sample videos found in $workspacePath"
}

Push-Location $projectRoot
try {
  $rows = docker compose exec -T postgres psql -U analyst -d analyst -At -F "`t" `
    -c "SELECT original_name,status FROM recordings ORDER BY original_name"
  if ($LASTEXITCODE -ne 0) { throw "Cannot read recordings from PostgreSQL" }
} finally {
  Pop-Location
}

$recordingStatus = @{}
foreach ($row in $rows) {
  if ([string]::IsNullOrWhiteSpace($row)) { continue }
  $parts = $row -split "`t", 2
  if ($parts.Count -eq 2) {
    $recordingStatus[$parts[0]] = $parts[1]
  }
}

$missing = @()
$notAnalyzed = @()
foreach ($video in $allowedVideos) {
  if (-not $recordingStatus.ContainsKey($video.Name)) {
    $missing += $video.Name
    continue
  }
  if ($recordingStatus[$video.Name] -ne "ANALYZED") {
    $notAnalyzed += "$($video.Name): $($recordingStatus[$video.Name])"
  }
}

if ($missing.Count -gt 0) {
  throw "Allowed sample videos are missing in recordings: $($missing -join '; ')"
}
if ($notAnalyzed.Count -gt 0) {
  throw "Allowed sample videos are not ANALYZED: $($notAnalyzed -join '; ')"
}

Write-Output "Allowed sample videos analyzed: $($allowedVideos.Count)"
if ($excludedVideos.Count -gt 0) {
  Write-Output "Excluded reserved videos ignored: $($excludedVideos.Name -join ', ')"
}
