$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

$checks = @(
  @{
    Name = "1 full upload analysis flow"
    Path = "frontend/tests/e2e/full-flow.spec.ts"
    Pattern = "real video"
  },
  @{
    Name = "2 unsupported upload validation"
    Path = "frontend/tests/e2e/upload-validation.spec.ts"
    Pattern = "unsupported file upload is rejected"
  },
  @{
    Name = "3 retry reuses active run"
    Path = "backend/internal/analysis/service_integration_test.go"
    Pattern = "TestRetryReusesActiveRetryRun"
  },
  @{
    Name = "4 worker crash reconciliation"
    Path = "backend/internal/outbox/reconciler_integration_test.go"
    Pattern = "TestReconcilerMarksStaleProcessingJobFailed"
  },
  @{
    Name = "5 Gemini temporary failure retry"
    Path = "backend/internal/vision/gemini_test.go"
    Pattern = "TestGenerateRetriesTemporaryGeminiFailure"
  },
  @{
    Name = "6 Redis outage"
    Path = "scripts/test-redis-recovery.ps1"
    Pattern = "Outbox event was not published after Redis recovery"
  },
  @{
    Name = "6 PostgreSQL outage"
    Path = "scripts/test-postgres-failure.ps1"
    Pattern = "Health endpoint did not report PostgreSQL outage"
  },
  @{
    Name = "6 S3 outage"
    Path = "scripts/test-s3-failure.ps1"
    Pattern = "Health endpoint did not report S3 outage"
  },
  @{
    Name = "7 event edit rebuild report"
    Path = "frontend/tests/e2e/full-flow.spec.ts"
    Pattern = "reportRebuilt"
  },
  @{
    Name = "8 QA resolve complete"
    Path = "frontend/tests/e2e/full-flow.spec.ts"
    Pattern = "assertQAResolveAndComplete"
  },
  @{
    Name = "9 JSON export"
    Path = "frontend/tests/e2e/full-flow.spec.ts"
    Pattern = "exports/timeline.json"
  },
  @{
    Name = "10 delete S3 cleanup"
    Path = "frontend/tests/e2e/full-flow.spec.ts"
    Pattern = "deleteRecordingAndAssertCleanup"
  },
  @{
    Name = "11 outbox dispatcher recovery"
    Path = "scripts/test-redis-recovery.ps1"
    Pattern = "Outbox did not record a failed Redis publication"
  },
  @{
    Name = "12 OIDC JWT RBAC"
    Path = "backend/internal/auth/middleware_oidc_test.go"
    Pattern = "TestOIDCJWTAndRBACMiddleware"
  },
  @{
    Name = "13 Loki correlation search"
    Path = "frontend/tests/e2e/full-flow.spec.ts"
    Pattern = "waitForCorrelationLogs"
  }
)

foreach ($check in $checks) {
  $path = Join-Path $projectRoot $check.Path
  if (-not (Test-Path $path)) {
    throw "Missing E2E coverage artifact for $($check.Name): $($check.Path)"
  }
  $content = Get-Content -Path $path -Raw
  if (-not $content.Contains($check.Pattern)) {
    throw "Missing E2E coverage marker for $($check.Name): '$($check.Pattern)' in $($check.Path)"
  }
}

$coverageDoc = Join-Path $projectRoot "docs\E2E_COVERAGE.md"
if (-not (Test-Path $coverageDoc)) {
  throw "Missing docs/E2E_COVERAGE.md"
}
$coverageContent = Get-Content -Path $coverageDoc -Raw
foreach ($number in 1..13) {
  if (-not $coverageContent.Contains("| $number |")) {
    throw "docs/E2E_COVERAGE.md does not list PLAN E2E scenario $number"
  }
}

Write-Output "PLAN E2E coverage matrix is complete."
