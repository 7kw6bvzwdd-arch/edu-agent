param([string]$Pattern='TestCoursewareBrief', [string]$Label='conversation', [string]$Source='backend-go')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$database='edu_verify_20261009_'+([guid]::NewGuid().ToString('N').Substring(0,12))
$go=Join-Path $root 'work/toolchain/go/bin/go.exe'
if(!(Test-Path -LiteralPath $go)){$go=(Get-Command go -ErrorAction Stop).Source}
$databaseAdminURL=$env:TEST_DATABASE_ADMIN_URL
if(!$databaseAdminURL){$databaseAdminURL='postgres://edu:edu-local-only@127.0.0.1:55432/postgres?sslmode=disable'}
$env:TEST_DATABASE_ADMIN_URL=$databaseAdminURL
$uri=[UriBuilder]::new($databaseAdminURL);$uri.Path='/'+$database
$env:TEST_DATABASE_URL=$uri.Uri.AbsoluteUri
$env:TEST_RUNTIME_DATABASE_URL='';$env:TEST_RENDER_URL='http://127.0.0.1:8000'
$out=Join-Path $root 'work/conversation-first';New-Item -ItemType Directory -Force -Path $out|Out-Null
$created=$false;$result=1
Push-Location (Join-Path $root 'backend-go')
try {
 & $go run ../scripts/test-database.go create $database
 if($LASTEXITCODE -ne 0){throw 'Could not create isolated test database.'};$created=$true
 Push-Location (Join-Path $root $Source)
 try {
  & $go test -count=1 -v -timeout 12m -run $Pattern . > (Join-Path $out ($Label+'.log')) 2>&1
  $result=$LASTEXITCODE
 }finally{Pop-Location}
 Get-Content -LiteralPath (Join-Path $out ($Label+'.log')) -Tail 40
}finally{
 if($created){& $go run ../scripts/test-database.go drop $database;if($LASTEXITCODE -ne 0){throw 'Isolated database cleanup failed.'}}
 Pop-Location
}
exit $result
