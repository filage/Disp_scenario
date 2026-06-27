param(
  [string]$ApiUrl = "http://localhost:8787",
  [int]$WarmupRequests = 2,
  [int]$MeasuredRequests = 10,
  [int]$HealthP95Ms = 1000,
  [int]$MetricsP95Ms = 1500,
  [int]$ApiP95Ms = 2500
)

$ErrorActionPreference = "Stop"

$checks = @(
  @{ Name = "health"; Path = "/health"; ThresholdMs = $HealthP95Ms },
  @{ Name = "metrics"; Path = "/metrics"; ThresholdMs = $MetricsP95Ms },
  @{ Name = "recordings"; Path = "/v1/recordings"; ThresholdMs = $ApiP95Ms },
  @{ Name = "analysis-runs"; Path = "/v1/analysis-runs"; ThresholdMs = $ApiP95Ms },
  @{ Name = "scenarios"; Path = "/v1/scenarios"; ThresholdMs = $ApiP95Ms },
  @{ Name = "project-analysis"; Path = "/v1/project/analysis"; ThresholdMs = $ApiP95Ms }
)

function Invoke-TimedRequest([string]$Url) {
  $watch = [System.Diagnostics.Stopwatch]::StartNew()
  try {
    $response = Invoke-WebRequest -Uri $Url -TimeoutSec 20 -UseBasicParsing
    $watch.Stop()
    if ($response.StatusCode -lt 200 -or $response.StatusCode -ge 300) {
      throw "Unexpected status $($response.StatusCode) from $Url"
    }
    return [int]$watch.ElapsedMilliseconds
  } catch {
    $watch.Stop()
    throw "Request failed after $([int]$watch.ElapsedMilliseconds)ms: $Url :: $($_.Exception.Message)"
  }
}

function Get-Percentile([int[]]$Values, [double]$Percentile) {
  if ($Values.Count -eq 0) {
    return 0
  }
  $sorted = @($Values | Sort-Object)
  $index = [Math]::Ceiling($sorted.Count * $Percentile) - 1
  if ($index -lt 0) { $index = 0 }
  if ($index -ge $sorted.Count) { $index = $sorted.Count - 1 }
  return [int]$sorted[$index]
}

foreach ($check in $checks) {
  $url = $ApiUrl.TrimEnd("/") + $check.Path
  foreach ($attempt in 1..$WarmupRequests) {
    [void](Invoke-TimedRequest $url)
  }

  $durations = New-Object System.Collections.Generic.List[int]
  foreach ($attempt in 1..$MeasuredRequests) {
    $durations.Add((Invoke-TimedRequest $url))
  }

  $p95 = Get-Percentile -Values $durations.ToArray() -Percentile 0.95
  $max = [int](($durations | Measure-Object -Maximum).Maximum)
  $avg = [int](($durations | Measure-Object -Average).Average)
  if ($p95 -gt $check.ThresholdMs) {
    throw "$($check.Name) p95 ${p95}ms exceeds threshold $($check.ThresholdMs)ms; avg=${avg}ms max=${max}ms"
  }
  Write-Output "$($check.Name): p95=${p95}ms avg=${avg}ms max=${max}ms threshold=$($check.ThresholdMs)ms"
}

Write-Output "Performance smoke verification passed."
