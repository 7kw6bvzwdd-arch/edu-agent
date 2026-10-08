$ErrorActionPreference='Stop'
$projectRoot=Split-Path -Parent $PSScriptRoot
$pidFile=Join-Path $projectRoot 'work/service-pids.json'
if (Test-Path -LiteralPath $pidFile) {
 $serviceIds=Get-Content -LiteralPath $pidFile | ConvertFrom-Json
 foreach ($serviceId in @($serviceIds.render,$serviceIds.backend)) {
  $process=Get-Process -Id $serviceId -ErrorAction SilentlyContinue
  if ($process -and $process.Path -and $process.Path.StartsWith($projectRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) {
   # Windows venv Python launches a child interpreter which also owns the renderer port.
   taskkill.exe /PID $serviceId /T /F | Out-Null
   if ($LASTEXITCODE -ne 0) { throw "Could not stop service process tree $serviceId." }
  }
 }
 Remove-Item -LiteralPath $pidFile
}
Write-Host 'Local processes stopped. PostgreSQL data and container are retained.'
