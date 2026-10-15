param([int]$TimeoutSeconds=45)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$cloud=Join-Path $root 'work/cloud'
$settings=@{}
Get-Content -LiteralPath (Join-Path $root '.env') | ForEach-Object {if($_ -match '^([^#=]+)=(.*)$'){$settings[$matches[1].Trim()]=$matches[2].Trim()}}
function Assert-Login {
 $body=@{username='teacher';password=$settings['SEED_PASSWORD']} | ConvertTo-Json
 $login=Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:8080/api/auth/login' -ContentType 'application/json' -Body $body -TimeoutSec 10
 if(!$login.token){throw 'Teacher login returned no token.'}
}
Assert-Login
$saved=Get-Content -LiteralPath (Join-Path $cloud 'tunnel-pid.json') | ConvertFrom-Json
$tunnel=Get-CimInstance Win32_Process -Filter "ProcessId = $($saved.pid)"
$key=Join-Path $cloud 'id_ed25519'
if(!$tunnel -or $tunnel.Name -ne 'ssh.exe' -or !$tunnel.CommandLine.Contains($key)){throw 'Refusing to stop a process not owned by this project.'}
Write-Host 'Simulating disconnect of only the education-project SSH tunnel.'
Stop-Process -Id $saved.pid -ErrorAction Stop
$recovered=$false
try {
 $deadline=[DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
 do {
  Start-Sleep -Seconds 2
  try {
   $new=Get-Content -LiteralPath (Join-Path $cloud 'tunnel-pid.json') | ConvertFrom-Json
   if($new.pid -eq $saved.pid){continue}
   $health=Invoke-RestMethod 'http://127.0.0.1:8080/api/health' -TimeoutSec 5
   if($health.status -ne 'ok'){continue}
   foreach($name in @('postgresql','redis','minio','milvus')){if($health.dependencies.$name -ne 'ok'){throw "Dependency $name not healthy."}}
   Assert-Login
   $recovered=$true
   Write-Host 'PASS: tunnel automatically reconnected, four dependencies healthy, teacher login HTTP 200.'
   break
  } catch {}
 } while([DateTime]::UtcNow -lt $deadline)
 if(!$recovered){throw 'FAIL: tunnel did not automatically reconnect; login remains unavailable.'}
} finally {
 if(!$recovered){ & (Join-Path $PSScriptRoot 'start-cloud.ps1') }
}
