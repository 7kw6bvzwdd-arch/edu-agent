param([switch]$BuildAnimation)
$ErrorActionPreference='Stop'
$projectRoot=Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectRoot
if (!(Test-Path -LiteralPath '.env')) { throw 'Copy .env.example to .env and set JWT_SECRET and SEED_PASSWORD first.' }
Get-Content -LiteralPath '.env' | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { [Environment]::SetEnvironmentVariable($matches[1].Trim(),$matches[2].Trim(),'Process') } }
$goExe=Join-Path $projectRoot 'work/toolchain/go/bin/go.exe'
if (!(Test-Path -LiteralPath $goExe)) { $goExe=(Get-Command go -ErrorAction Stop).Source }
$pythonExe=Join-Path $projectRoot '.venv/Scripts/python.exe'
if (!(Test-Path -LiteralPath $pythonExe)) { python -m venv .venv }
& $pythonExe -m pip install --quiet -r render-service-py/requirements.txt
if ($LASTEXITCODE -ne 0) { throw 'Python dependency installation failed.' }
if ($env:CLOUD_SSH_HOST) {
 & (Join-Path $PSScriptRoot 'start-cloud.ps1')
} else {
 docker compose up -d --wait db
 if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL failed to start.' }
}
if ($BuildAnimation) { docker build -f render-service-py/Dockerfile.manim -t edu-manim:local render-service-py; if ($LASTEXITCODE -ne 0) { throw 'Animation image build failed.' } }
Push-Location frontend-vue
npm ci
if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed.' }
npm run build
if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
Pop-Location
New-Item -ItemType Directory -Force work | Out-Null
$backendSource=Join-Path $projectRoot 'backend-go'
$backendDeployment=Join-Path $projectRoot 'work/backend-deployment.json'
if(Test-Path -LiteralPath $backendDeployment){
 $backendSource=(Get-Content -LiteralPath $backendDeployment -Raw|ConvertFrom-Json).runtime_source_directory
 if(!(Test-Path -LiteralPath (Join-Path $backendSource 'go.mod'))){throw 'Deployed backend source missing; inspect work/backend-deployment.json.'}
}
Push-Location $backendSource
& $goExe build -o (Join-Path $projectRoot 'work/edu-server.exe') .
if ($LASTEXITCODE -ne 0) { throw 'Backend build failed.' }
Pop-Location
foreach ($port in @(8000,8080)) { if (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue) { throw "Port $port is already in use. Stop the current service before starting another instance." } }
$rendererDir=Join-Path $projectRoot 'render-service-py'
$rendererDeployment=Join-Path $projectRoot 'work/renderer-deployment.json'
if(Test-Path -LiteralPath $rendererDeployment){
 $rendererDir=(Get-Content -LiteralPath $rendererDeployment -Raw|ConvertFrom-Json).runtime_directory
 if(!(Test-Path -LiteralPath (Join-Path $rendererDir 'app.py'))){throw 'Deployed renderer source missing; inspect work/renderer-deployment.json.'}
}
$render=Start-Process -FilePath $pythonExe -ArgumentList '-m','uvicorn','app:app','--host','127.0.0.1','--port','8000' -WorkingDirectory $rendererDir -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $projectRoot 'work/render.out.log') -RedirectStandardError (Join-Path $projectRoot 'work/render.err.log')
$backend=Start-Process -FilePath (Join-Path $projectRoot 'work/edu-server.exe') -WorkingDirectory (Join-Path $projectRoot 'backend-go') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $projectRoot 'work/backend.out.log') -RedirectStandardError (Join-Path $projectRoot 'work/backend.err.log')
@{render=$render.Id;backend=$backend.Id} | ConvertTo-Json | Set-Content -LiteralPath 'work/service-pids.json'
$healthy=$false
for ($attempt=0;$attempt -lt 40;$attempt++) { try { $status=Invoke-RestMethod 'http://127.0.0.1:8080/api/health'; if ($status.status -eq 'ok') { $healthy=$true; break } } catch {} ; Start-Sleep -Milliseconds 250 }
if (!$healthy) { throw 'Backend did not become healthy. Check work/backend.err.log.' }
Write-Host 'Started http://127.0.0.1:8080 (logs: work/*.log). Accounts: teacher, admin, low, other. SEED_PASSWORD initializes new accounts only; existing passwords stay unchanged.'
Write-Host 'To explicitly reset one demo account from .env: ./scripts/reset-password.ps1 -Username teacher'
