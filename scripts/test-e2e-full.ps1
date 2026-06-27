param(
  [string[]]$Spec = @(
    "tests/e2e/full-flow.spec.ts",
    "tests/e2e/upload-validation.spec.ts"
  )
)

$ErrorActionPreference = "Stop"
$frontendRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\frontend")).Path

Push-Location $frontendRoot
try {
  $env:E2E_FULL_STACK = "true"
  npx playwright test @Spec
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
} finally {
  Pop-Location
}
