param([string]$Topics='quadratic,circuits',[string]$CheckpointFixture='')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$database='edu_verify_20261009_'+([guid]::NewGuid().ToString('N').Substring(0,12))
$go=Join-Path $root 'work/toolchain/go/bin/go.exe'
if(!(Test-Path -LiteralPath $go)){$go=(Get-Command go -ErrorAction Stop).Source}
$out=Join-Path $root ('work/conversation-first/live-'+(Get-Date -Format 'yyyyMMdd-HHmmss'))
New-Item -ItemType Directory -Force -Path $out|Out-Null
Get-Content -LiteralPath (Join-Path $root '.env') -Encoding UTF8|ForEach-Object{if($_ -match '^([^#=]+)=(.*)$'){[Environment]::SetEnvironmentVariable($matches[1].Trim(),$matches[2].Trim(),'Process')}}
$databaseAdminURL=$env:TEST_DATABASE_ADMIN_URL
if(!$databaseAdminURL){throw 'Set explicit TEST_DATABASE_ADMIN_URL for the isolated test cluster.'}
$env:TEST_DATABASE_ADMIN_URL=$databaseAdminURL
$uri=[UriBuilder]::new($databaseAdminURL);$uri.Path='/'+$database
$env:TEST_DATABASE_URL=$uri.Uri.AbsoluteUri;$env:TEST_RUNTIME_DATABASE_URL='';$env:TEST_RENDER_URL='http://127.0.0.1:8000'
$env:TEST_FULL_DECK_LIVE='1';$env:TEST_FULL_DECK_TOPIC=$Topics;$env:TEST_FULL_DECK_RUN=Split-Path $out -Leaf
$env:TEST_FULL_DECK_RESUME_ID='';$env:TEST_FULL_DECK_PLAN_FIXTURE='';$env:TEST_FULL_DECK_CHECKPOINT_FIXTURE=''
if($CheckpointFixture){
 if($Topics -notin @('linear','buoyancy','quadratic','circuits')){throw 'Checkpoint replay requires one explicit topic.'}
 $env:TEST_FULL_DECK_CHECKPOINT_FIXTURE=(Resolve-Path -LiteralPath $CheckpointFixture).Path
}
$created=$false;$result=1
Push-Location (Join-Path $root 'backend-go')
try{
 & $go run ../scripts/test-database.go create $database
 if($LASTEXITCODE -ne 0){throw 'Isolated database creation failed.'};$created=$true
 & $go test -count=1 -v -timeout 140m -run '^TestCoursewareFullQualityLive$' . > (Join-Path $out 'go.log') 2>&1
 $result=$LASTEXITCODE
 Get-Content -LiteralPath (Join-Path $out 'go.log') -Tail 30
}finally{
 if($created){& $go run ../scripts/test-database.go drop $database;if($LASTEXITCODE -ne 0){throw 'Isolated database cleanup failed.'}}
 Pop-Location
}
exit $result
