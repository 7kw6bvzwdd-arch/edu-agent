param([string]$Pattern='TestWorkflow', [string]$Label='workflow', [string]$Source='backend-go')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$database='edu_verify_20261009_'+([guid]::NewGuid().ToString('N').Substring(0,12))
$go=Join-Path $root 'work/toolchain/go/bin/go.exe'
if(!(Test-Path -LiteralPath $go)){$go=(Get-Command go -ErrorAction Stop).Source}
$databaseAdminURL=$env:TEST_DATABASE_ADMIN_URL
if(!$databaseAdminURL){throw 'Set explicit TEST_DATABASE_ADMIN_URL for the isolated test cluster.'}
$env:TEST_DATABASE_ADMIN_URL=$databaseAdminURL
$uri=[UriBuilder]::new($databaseAdminURL);$uri.Path='/'+$database
$env:TEST_DATABASE_URL=$uri.Uri.AbsoluteUri
if($env:TEST_RUNTIME_DATABASE_BASE_URL){$runtime=[UriBuilder]::new($env:TEST_RUNTIME_DATABASE_BASE_URL);$runtime.Path='/'+$database;$env:TEST_RUNTIME_DATABASE_URL=$runtime.Uri.AbsoluteUri}else{$env:TEST_RUNTIME_DATABASE_URL=''};$env:TEST_RENDER_URL='http://127.0.0.1:8000'
$out=Join-Path $root 'work/conversation-first';New-Item -ItemType Directory -Force -Path $out|Out-Null
$created=$false;$result=1
Push-Location (Join-Path $root 'backend-go')
try {
 & $go run ../scripts/test-database.go create $database
 if($LASTEXITCODE -ne 0){throw 'Could not create isolated test database.'};$created=$true
 Push-Location (Join-Path $root $Source)
 try {
  $testBinary=Join-Path $out ($Label+'-test.exe'); & $go test -c -o $testBinary .; if($LASTEXITCODE -ne 0){throw 'Test compilation failed.'}; Push-Location (Join-Path $root 'backend-go'); try { & $testBinary "-test.count=1" "-test.v" "-test.timeout=12m" "-test.run=$Pattern" > (Join-Path $out ($Label+'.log')) 2>&1 } finally {Pop-Location}
  $result=$LASTEXITCODE
 }finally{Pop-Location}
 Get-Content -LiteralPath (Join-Path $out ($Label+'.log')) -Tail 40
}finally{
 if($created){& $go run ../scripts/test-database.go drop $database;if($LASTEXITCODE -ne 0){throw 'Isolated database cleanup failed.'}}
 Pop-Location
}
exit $result



