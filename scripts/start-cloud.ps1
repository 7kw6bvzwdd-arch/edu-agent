param([switch]$NoWatch)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$settings=@{}
Get-Content -LiteralPath (Join-Path $root '.env') | ForEach-Object {if($_ -match '^([^#=]+)=(.*)$'){$settings[$matches[1].Trim()]=$matches[2].Trim()}}
if(!$settings['CLOUD_SSH_HOST']){return}
$dir=Join-Path $root 'work/cloud'
$pidFile=Join-Path $dir 'tunnel-pid.json'
function Start-TunnelWatch {
 if($NoWatch){return}
 $watchScript=Join-Path $PSScriptRoot 'watch-cloud.ps1'
 $watchPidFile=Join-Path $dir 'watch-pid.json'
 if(Test-Path -LiteralPath $watchPidFile){
  $watchSaved=Get-Content -LiteralPath $watchPidFile | ConvertFrom-Json
  $watch=Get-CimInstance Win32_Process -Filter "ProcessId = $($watchSaved.pid)" -ErrorAction SilentlyContinue
  if($watch -and $watch.Name -eq 'powershell.exe' -and $watch.CommandLine -and $watch.CommandLine.Contains($watchScript)){return}
 }
 $watchArgs=@('-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File',('"'+$watchScript+'"'))
 $watch=Start-Process -FilePath (Get-Command powershell.exe).Source -ArgumentList $watchArgs -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $dir 'watch.out.log') -RedirectStandardError (Join-Path $dir 'watch.err.log')
 @{pid=$watch.Id} | ConvertTo-Json | Set-Content -LiteralPath $watchPidFile
}
if(Test-Path -LiteralPath $pidFile){
 $saved=Get-Content -LiteralPath $pidFile | ConvertFrom-Json
 $proc=Get-CimInstance Win32_Process -Filter "ProcessId = $($saved.pid)" -ErrorAction SilentlyContinue
 if($proc -and $proc.Name -eq 'ssh.exe' -and $proc.CommandLine.Contains((Join-Path $dir 'id_ed25519'))){
  $allReady=$true
  foreach($port in @(25432,26379,29000,29001,29531)){if(!(Get-NetTCPConnection -LocalAddress '127.0.0.1' -LocalPort $port -State Listen -ErrorAction SilentlyContinue)){$allReady=$false}}
  if($allReady){Start-TunnelWatch;return};throw 'Existing cloud tunnel is not healthy; stop it before retrying.'
 }
}
foreach($port in @(25432,26379,29000,29001,29531)){if(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue){throw "Cloud tunnel port $port is already occupied; refusing to reuse another service."}}
$args=@('-N','-T','-i',('"'+(Join-Path $dir 'id_ed25519')+'"'),'-o',('"UserKnownHostsFile='+(Join-Path $dir 'known_hosts')+'"'),'-o','StrictHostKeyChecking=yes','-o','IdentitiesOnly=yes','-o','BatchMode=yes','-o','ExitOnForwardFailure=yes','-o','ServerAliveInterval=15','-o','ServerAliveCountMax=3','-o','ConnectTimeout=10')
foreach($map in @('25432:25432','26379:26379','29000:29000','29001:29001','29531:19531')){$pair=$map.Split(':');$args+=@('-L',"127.0.0.1:$($pair[0]):127.0.0.1:$($pair[1])")}
$args+=($settings['CLOUD_SSH_USER']+'@'+$settings['CLOUD_SSH_HOST'])
$proc=Start-Process -FilePath (Get-Command ssh.exe).Source -ArgumentList $args -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $dir 'tunnel.out.log') -RedirectStandardError (Join-Path $dir 'tunnel.err.log')
@{pid=$proc.Id} | ConvertTo-Json | Set-Content -LiteralPath $pidFile
for($i=0;$i -lt 40;$i++){
 $proc.Refresh();if($proc.HasExited){throw 'SSH tunnel failed. Check work/cloud/tunnel.err.log.'}
 if(Get-NetTCPConnection -LocalAddress '127.0.0.1' -LocalPort 29531 -State Listen -ErrorAction SilentlyContinue){Start-TunnelWatch;Write-Host 'Project-only cloud SSH tunnel ready (automatic reconnect enabled).';return}
 Start-Sleep -Milliseconds 250
}
throw 'Cloud tunnel timed out.'
