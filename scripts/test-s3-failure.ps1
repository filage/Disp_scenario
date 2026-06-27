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
  foreach ($attempt in 1..40) {
    try {
      $response = Get-Health
      if ($response.StatusCode -eq 200 -and $response.Body.status -eq "ok" -and $response.Body.dependencies.s3 -eq "ok") {
        return
      }
    } catch {
      # Keep polling until the API and MinIO are healthy again.
    }
    Start-Sleep -Seconds 2
  }
  throw "Health endpoint did not recover after S3 restart"
}

Push-Location $projectRoot
try {
  Wait-Healthy

  docker compose stop minio
  if ($LASTEXITCODE -ne 0) { throw "Stopping MinIO failed" }

  $s3FailureObserved = $false
  foreach ($attempt in 1..20) {
    try {
      $response = Get-Health
      if ($response.StatusCode -eq 503 -and $response.Body.status -eq "degraded" -and $response.Body.dependencies.s3 -eq "error") {
        $s3FailureObserved = $true
        break
      }
    } catch {
      # Continue polling; the API may still be restarting or the dependency
      # failure body may not be available yet.
    }
    Start-Sleep -Seconds 1
  }
  if (-not $s3FailureObserved) {
    throw "Health endpoint did not report S3 outage"
  }

  docker compose up -d minio minio-init
  if ($LASTEXITCODE -ne 0) { throw "Starting MinIO failed" }
  Wait-Healthy

  Write-Output "S3 outage health degradation verification passed."
} finally {
  $cleanupErrorActionPreference = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  try {
    docker compose up -d minio minio-init *> $null
    Wait-Healthy
  } finally {
    $ErrorActionPreference = $cleanupErrorActionPreference
    Pop-Location
  }
}
