param([Parameter(Mandatory=$true)][ValidateSet('teacher','admin','low','other')][string]$Username)
$ErrorActionPreference='Stop'
$projectRoot=Split-Path -Parent $PSScriptRoot
$envFile=Join-Path $projectRoot '.env'
if (!(Test-Path -LiteralPath $envFile)) {throw 'Missing project .env file.'}
$settings=@{}
Get-Content -LiteralPath $envFile | ForEach-Object {if ($_ -match '^([^#=]+)=(.*)$') {$settings[$matches[1].Trim()]=$matches[2].Trim()}}
if (!$settings['SEED_PASSWORD']) {throw 'Set SEED_PASSWORD in the project .env first.'}
$priorPassword=$env:SEED_PASSWORD
$priorDatabase=$env:DATABASE_URL
$goExe=Join-Path $projectRoot 'work/toolchain/go/bin/go.exe'
if (!(Test-Path -LiteralPath $goExe)) {$goExe=(Get-Command go -ErrorAction Stop).Source}
$binary=Join-Path $projectRoot 'work/edu-maintenance.exe'
New-Item -ItemType Directory -Force (Join-Path $projectRoot 'work') | Out-Null
Push-Location (Join-Path $projectRoot 'backend-go')
try {
 $env:SEED_PASSWORD=$settings['SEED_PASSWORD']
 $env:DATABASE_URL=$settings['DATABASE_URL']
 & $goExe build -o $binary .
 if ($LASTEXITCODE -ne 0) {throw 'Maintenance build failed; password unchanged.'}
 & $binary --reset-seed-password $Username
 if ($LASTEXITCODE -ne 0) {throw 'Password reset failed; inspect the message above.'}
} finally {
 $env:SEED_PASSWORD=$priorPassword
 $env:DATABASE_URL=$priorDatabase
 Pop-Location
}
