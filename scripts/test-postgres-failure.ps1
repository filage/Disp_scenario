param(
  [string]$ApiUrl = "http://localhost:8787"
)

$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

function Get-Health() {
  try {
    $body = Invoke-RestMethod -Uri "$ApiUrl/health" -TimeoutSec 5
    return [pscustomobject]@{
      StatusCode = 200
      Body = $body
    }
  } catch {
    $statusCode = $null
    if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
      $statusCode = $_.Exception.Response.StatusCode.value__
    }
    if ($statusCode -eq 503 -and $_.ErrorDetails.Message) {
      try {
        return [pscustomobject]@{
          StatusCode = $statusCode
          Body = ($_.ErrorDetails.Message | ConvertFrom-Json)
        }
      } catch {
        return [pscustomobject]@{
          StatusCode = $statusCode
          Body = $null
        }
      }
    }
    throw
  }
}

function Wait-Healthy() {
  foreach ($attempt in 1..60) {
    try {
      $response = Get-Health
      if (
        $response.StatusCode -eq 200 `
        -and $response.Body.status -eq "ok" `
        -and $response.Body.dependencies.postgres -eq "ok" `
        -and $response.Body.dependencies.redis -eq "ok" `
        -and $response.Body.dependencies.s3 -eq "ok"
      ) {
        return
      }
    } catch {
      # Keep polling until the API and PostgreSQL are healthy again.
    }
    Start-Sleep -Seconds 2
  }
  throw "Health endpoint did not recover after PostgreSQL restart"
}

Push-Location $projectRoot
try {
  Wait-Healthy

  docker compose stop postgres
  if ($LASTEXITCODE -ne 0) { throw "Stopping PostgreSQL failed" }

  $postgresFailureObserved = $false
  foreach ($attempt in 1..20) {
    try {
      $response = Get-Health
      if (
        $response.StatusCode -eq 503 `
        -and $response.Body.status -eq "degraded" `
        -and $response.Body.dependencies.postgres -eq "error"
      ) {
        $postgresFailureObserved = $true
        break
      }
    } catch {
      # Continue polling; the API may still be reconnecting or the dependency
      # failure body may not be available yet.
    }
    Start-Sleep -Seconds 1
  }
  if (-not $postgresFailureObserved) {
    throw "Health endpoint did not report PostgreSQL outage"
  }

  docker compose up -d postgres
  if ($LASTEXITCODE -ne 0) { throw "Starting PostgreSQL failed" }
  Wait-Healthy

  Write-Output "PostgreSQL outage health degradation verification passed."
} finally {
  $cleanupErrorActionPreference = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  try {
    docker compose up -d postgres *> $null
    Wait-Healthy
  } finally {
    $ErrorActionPreference = $cleanupErrorActionPreference
    Pop-Location
  }
}
