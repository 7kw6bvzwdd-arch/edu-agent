param(
 [Parameter(Mandatory=$true)][string]$BriefPath,
 [Parameter(Mandatory=$true)][string]$OutPath,
 [ValidateSet('tts','estimated','tone')][string]$Voice='tts'
)
$ErrorActionPreference='Stop'
$projectRoot=Split-Path -Parent $PSScriptRoot
$brief=(Resolve-Path -LiteralPath $BriefPath).Path
$output=[System.IO.Path]::GetFullPath($OutPath)
$envPath=Join-Path $projectRoot '.env'
if (!(Test-Path -LiteralPath $envPath)) { throw 'Create .env and configure ANIMATION_LLM_* and ANIMATION_TTS_* first.' }
Get-Content -LiteralPath $envPath | ForEach-Object {
 if ($_ -match '^(ANIMATION_(?:LLM|TTS)_[A-Z_]+)=(.*)$') {
  [Environment]::SetEnvironmentVariable($matches[1],$matches[2].Trim(),'Process')
 }
}
$goExe=Join-Path $projectRoot 'work/toolchain/go/bin/go.exe'
if (!(Test-Path -LiteralPath $goExe)) { $goExe=(Get-Command go -ErrorAction Stop).Source }
$buildDir=Join-Path $projectRoot 'work/anim-v2'
New-Item -ItemType Directory -Force -Path $buildDir | Out-Null
$binary=Join-Path $buildDir 'edu-animation.exe'
& $goExe -C (Join-Path $projectRoot 'backend-go') build -o $binary .
if ($LASTEXITCODE -ne 0) { throw 'Animation command build failed.' }
& $binary --anim-generate $brief $output $Voice
if ($LASTEXITCODE -ne 0) { throw 'Animation generation failed. Existing HTML was preserved; check the sanitized error above.' }
