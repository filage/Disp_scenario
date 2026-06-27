$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$frontendRoot = Join-Path $projectRoot "frontend"
$backendRoot = Join-Path $projectRoot "backend"
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("analyst-openapi-" + [System.Guid]::NewGuid().ToString("N"))

function Get-NormalizedContent([string]$Path) {
  return ([System.IO.File]::ReadAllText($Path) -replace "`r`n", "`n").TrimEnd()
}

function Assert-SameGeneratedFile([string]$ExpectedPath, [string]$ActualPath, [string]$Label) {
  if (-not (Test-Path $ExpectedPath)) {
    throw "$Label expected generated file is missing: $ExpectedPath"
  }
  if (-not (Test-Path $ActualPath)) {
    throw "$Label actual generated file is missing: $ActualPath"
  }
  $expected = Get-NormalizedContent $ExpectedPath
  $actual = Get-NormalizedContent $ActualPath
  if ($expected -ne $actual) {
    throw "$Label generated file is out of sync with api/openapi.yaml. Run make generate or the documented OpenAPI generation commands."
  }
}

New-Item -ItemType Directory -Path $tempRoot | Out-Null
try {
  $goOutput = Join-Path $tempRoot "generated.go"
  $goConfig = Join-Path $tempRoot "oapi-codegen.yaml"
  @"
package: openapi
output: /out/generated.go
generate:
  models: true
  chi-server: true
  strict-server: true
output-options:
  skip-prune: true
"@ | Set-Content -Path $goConfig -Encoding UTF8

  docker run --rm `
    -v "${projectRoot}:/repo" `
    -v "${tempRoot}:/out" `
    -w /repo/backend `
    golang:1.25-alpine `
    go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.1 --config /out/oapi-codegen.yaml ../api/openapi.yaml
  if ($LASTEXITCODE -ne 0) { throw "Go OpenAPI generation failed" }

  $openapiTypescript = Join-Path $frontendRoot "node_modules\.bin\openapi-typescript.cmd"
  if (-not (Test-Path $openapiTypescript)) {
    throw "openapi-typescript is not installed. Run npm install in frontend first."
  }
  $tsOutput = Join-Path $tempRoot "api.ts"
  Push-Location $frontendRoot
  try {
    & $openapiTypescript ..\api\openapi.yaml -o $tsOutput
    if ($LASTEXITCODE -ne 0) { throw "TypeScript OpenAPI generation failed" }
  } finally {
    Pop-Location
  }

  Assert-SameGeneratedFile $goOutput (Join-Path $backendRoot "internal\openapi\generated.go") "Go"
  Assert-SameGeneratedFile $tsOutput (Join-Path $frontendRoot "src\generated\api.ts") "TypeScript"

  Write-Output "OpenAPI contract generation is in sync."
} finally {
  if (Test-Path $tempRoot) {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force
  }
}
