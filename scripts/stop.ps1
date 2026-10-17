$ErrorActionPreference='Stop'
$projectRoot=Split-Path -Parent $PSScriptRoot
# Stop the reconnect loop first so shutdown cannot recreate the tunnel.
$watchPidFile=Join-Path $projectRoot 'work/cloud/watch-pid.json'
if(Test-Path -LiteralPath $watchPidFile){
 $savedWatch=Get-Content -LiteralPath $watchPidFile | ConvertFrom-Json
 $watchScript=Join-Path $PSScriptRoot 'watch-cloud.ps1'
 if($savedWatch.script){$watchScript=$savedWatch.script}
 $watch=Get-CimInstance Win32_Process -Filter "ProcessId = $($savedWatch.pid)" -ErrorAction SilentlyContinue
 if($watch -and $watch.Name -eq 'powershell.exe' -and $watch.CommandLine -and $watch.CommandLine.Contains($watchScript)){
  $watchProcess=Get-Process -Id $savedWatch.pid -ErrorAction SilentlyContinue
  Stop-Process -Id $savedWatch.pid -ErrorAction Stop
  if($watchProcess){$watchProcess.WaitForExit()}
 }
 Remove-Item -LiteralPath $watchPidFile
}
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
$tunnelPidFile=Join-Path $projectRoot 'work/cloud/tunnel-pid.json'
if (Test-Path -LiteralPath $tunnelPidFile) {
 $saved=Get-Content -LiteralPath $tunnelPidFile | ConvertFrom-Json
 $tunnel=Get-CimInstance Win32_Process -Filter "ProcessId = $($saved.pid)" -ErrorAction SilentlyContinue
 if ($tunnel -and $tunnel.Name -eq 'ssh.exe' -and $tunnel.CommandLine.Contains((Join-Path $projectRoot 'work/cloud/id_ed25519'))) {
  Stop-Process -Id $saved.pid
 }
 Remove-Item -LiteralPath $tunnelPidFile
}
Write-Host 'Education-agent processes and its tunnel stopped. All local/cloud containers and data are retained.'
