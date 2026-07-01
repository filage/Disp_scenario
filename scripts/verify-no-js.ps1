$ErrorActionPreference = "Stop"

$forbidden = Get-ChildItem -Path $PSScriptRoot\.. -Recurse -File |
  Where-Object {
    $_.FullName -notmatch '\\node_modules\\|\\.next\\|\\coverage\\|\\dist\\|\\.graphify\\|\\demo-dashboard\\assets\\' -and
    $_.Extension -in @('.js', '.jsx', '.cjs', '.mjs')
  }

if ($forbidden) {
  $forbidden | ForEach-Object { Write-Error "Forbidden JavaScript source: $($_.FullName)" }
  exit 1
}

Write-Host "No forbidden JavaScript source files found."
