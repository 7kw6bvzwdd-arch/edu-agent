param([Parameter(Mandatory=$true)][string]$ReleasePath)
$ErrorActionPreference='Stop'
$projectRoot=Split-Path -Parent $PSScriptRoot
$release=(Resolve-Path -LiteralPath $ReleasePath).Path
$manifest=Get-Content -LiteralPath (Join-Path $release 'manifest.json') -Raw | ConvertFrom-Json
python (Join-Path $PSScriptRoot 'verify-release.py') --release $release --expect-tag $manifest.tag
if($LASTEXITCODE -ne 0){throw 'Release integrity verification failed.'}
$binary=Join-Path $release 'backend-go/edu-server.exe'
$identity=& $binary --version | ConvertFrom-Json
if($LASTEXITCODE -ne 0 -or $identity.release_tag -ne $manifest.tag -or $identity.release_commit -ne $manifest.commit){throw 'Binary identity differs from release manifest.'}
foreach($port in @(8000,8080)){if(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue){throw "Port $port is occupied; stop the current service before switching releases."}}
Set-Location -LiteralPath $projectRoot
if (!(Test-Path -LiteralPath '.env')) { throw 'Copy .env.example to .env and set JWT_SECRET and SEED_PASSWORD first.' }
Get-Content -LiteralPath '.env' | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { [Environment]::SetEnvironmentVariable($matches[1].Trim(),$matches[2].Trim(),'Process') } }
if(!$env:DATABASE_URL -or !$env:RUNTIME_DATABASE_URL){throw 'Set explicit DATABASE_URL and RUNTIME_DATABASE_URL before starting.'}
$env:FRONTEND_DIST=Join-Path $release 'frontend-vue/dist'
$env:RENDER_SERVICE_URL='http://127.0.0.1:8000'
$env:LISTEN_ADDR='127.0.0.1:8080'
if(!$env:STORAGE_DIR){$env:STORAGE_DIR=Join-Path $projectRoot 'storage'}
elseif(![IO.Path]::IsPathRooted($env:STORAGE_DIR)){$env:STORAGE_DIR=[IO.Path]::GetFullPath((Join-Path (Join-Path $projectRoot 'backend-go') $env:STORAGE_DIR))}
if($env:ANIMATION_EXPORT_SCRIPT){$env:ANIMATION_EXPORT_SCRIPT=Join-Path $release 'animation-video/export.mjs'}
$venv=Join-Path $projectRoot ('work/release-runtime/'+$manifest.commit+'/.venv')
$pythonExe=Join-Path $venv 'Scripts/python.exe'
if(!(Test-Path -LiteralPath $pythonExe)){python -m venv $venv;if($LASTEXITCODE -ne 0){throw 'Python environment creation failed.'}}
& $pythonExe -m pip install --quiet -r (Join-Path $release 'render-service-py/requirements.txt')
if ($LASTEXITCODE -ne 0) { throw 'Python dependency installation failed.' }
if ($env:CLOUD_SSH_HOST) {
 & (Join-Path $release 'scripts/start-cloud.ps1') -ProjectRoot $projectRoot
} else {
 docker compose --project-directory $projectRoot --env-file (Join-Path $projectRoot '.env') -f (Join-Path $release 'compose.yaml') up -d --wait db
 if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL failed to start.' }
}
if($env:ANIMATION_EXPORT_SCRIPT){
 Push-Location (Join-Path $release 'animation-video')
 try {
  $env:PUPPETEER_SKIP_DOWNLOAD='true'
  npm ci --no-audit --no-fund
  if($LASTEXITCODE -ne 0){throw 'Animation video dependency installation failed.'}
 }finally{Pop-Location}
}
New-Item -ItemType Directory -Force work | Out-Null
$env:SHUTDOWN_REQUEST_FILE=Join-Path $projectRoot ('work/shutdown-'+[guid]::NewGuid().ToString('N')+'.request')
$rendererDir=Join-Path $release 'render-service-py'
$render=$null;$backend=$null
try {
$render=Start-Process -FilePath $pythonExe -ArgumentList '-m','uvicorn','app:app','--host','127.0.0.1','--port','8000' -WorkingDirectory $rendererDir -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $projectRoot 'work/render.out.log') -RedirectStandardError (Join-Path $projectRoot 'work/render.err.log')
$ready=$false
for($i=0;$i -lt 40;$i++){try{Invoke-RestMethod 'http://127.0.0.1:8000/health' -TimeoutSec 2 | Out-Null;$ready=$true;break}catch{};Start-Sleep -Milliseconds 250}
if(!$ready){throw 'Renderer did not become healthy.'}
$backend=Start-Process -FilePath $binary -WorkingDirectory (Join-Path $release 'backend-go') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $projectRoot 'work/backend.out.log') -RedirectStandardError (Join-Path $projectRoot 'work/backend.err.log')
$healthy=$false
for ($attempt=0;$attempt -lt 40;$attempt++) { try { $status=Invoke-RestMethod 'http://127.0.0.1:8080/api/health' -TimeoutSec 2; if ($status.status -eq 'ok' -and $status.release_tag -eq $manifest.tag -and $status.release_commit -eq $manifest.commit) { $healthy=$true; break } } catch {} ; Start-Sleep -Milliseconds 250 }
if (!$healthy) { throw 'Backend did not become healthy. Check work/backend.err.log.' }
@{render=$render.Id;backend=$backend.Id;release=$release;tag=$manifest.tag;commit=$manifest.commit;shutdown_file=$env:SHUTDOWN_REQUEST_FILE} | ConvertTo-Json | Set-Content -LiteralPath 'work/service-pids.json'
@{release=$release;tag=$manifest.tag;commit=$manifest.commit;started_at=(Get-Date -Format o)} | ConvertTo-Json | Set-Content -LiteralPath 'work/active-release.json'
Write-Host "Started http://127.0.0.1:8080 from tag $($manifest.tag), commit $($manifest.commit)."
}catch{
 foreach($process in @($backend,$render)){if($process){taskkill.exe /PID $process.Id /T /F | Out-Null}}
 throw
}
