param(
 [string]$Renderer='http://127.0.0.1:8000',
 [switch]$LiveReview,
 [switch]$FullDecks,
 [string]$Topic='',
 [switch]$WebImages,
 [switch]$Closure,
 [switch]$RequireImage,
 [switch]$OnlyFullDecks,
 [string]$Material='',
 [switch]$FailFast
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$go=Join-Path $root 'work/toolchain/go/bin/go.exe'
if(!(Test-Path -LiteralPath $go)){$go=(Get-Command go -ErrorAction Stop).Source}
$health=Invoke-RestMethod ($Renderer.TrimEnd('/')+'/health')
if($health.status -ne 'ok'){throw 'A healthy real renderer is required.'}
$database='edu_verify_20261009_'+([guid]::NewGuid().ToString('N').Substring(0,12))
$report=Join-Path $root ('work/quality-gate/'+(Get-Date -Format 'yyyyMMdd-HHmmss')+'-'+$database.Substring($database.Length-4))
New-Item -ItemType Directory -Force -Path $report|Out-Null
& (Join-Path $root '.venv/Scripts/python.exe') (Join-Path $root 'scripts/capture-acceptance-source.py') --repo $root --output (Join-Path $report 'source')
if($LASTEXITCODE -ne 0){throw 'Candidate source capture failed.'}
$databaseAdminURL=$env:TEST_DATABASE_ADMIN_URL
if(!$databaseAdminURL){throw 'Set explicit TEST_DATABASE_ADMIN_URL for the isolated test cluster.'}
$env:TEST_DATABASE_ADMIN_URL=$databaseAdminURL
$uri=[UriBuilder]::new($databaseAdminURL);$uri.Path='/'+$database
$env:TEST_DATABASE_URL=$uri.Uri.AbsoluteUri
$env:TEST_RUNTIME_DATABASE_URL='';$env:TEST_RENDER_URL=$Renderer
# Explicit opt-ins; an inherited live/replay setting must not silently change a run.
$env:TEST_FULL_DECK_LIVE='';$env:TEST_TEACHING_REVIEW_LIVE=''
$env:TEST_FULL_DECK_RESUME_ID='';$env:TEST_FULL_DECK_PLAN_FIXTURE='';$env:TEST_FULL_DECK_CHECKPOINT_FIXTURE=''
$env:TEST_FULL_DECK_TOPIC='';$env:TEST_TEACHING_REVIEW_CASES=''
$env:TEST_FULL_DECK_WEB='';$env:TEST_FULL_DECK_CLOSURE='';$env:TEST_FULL_DECK_REQUIRE_IMAGE='';$env:TEST_FULL_DECK_MATERIAL='';$env:TEST_FULL_DECK_MICRO=''
$env:TEST_FULL_DECK_FAIL_FAST=if($FailFast){'1'}else{''}
if($LiveReview -or $FullDecks){
 Get-Content -LiteralPath (Join-Path $root '.env') -Encoding UTF8|ForEach-Object{if($_ -match '^([^#=]+)=(.*)$'){[Environment]::SetEnvironmentVariable($matches[1].Trim(),$matches[2].Trim(),'Process')}}
}
$pattern='TestTeachingRegressionCorpus|TestCoreTeachingSourcesArePublicSnapshotsHTTP|TestTextbookClaimEvidenceWorkflowHTTP|TestDocumentReparseKeepsIdentityAndSavedSourceHTTP|TestTeachingMaterialsAndReferencesIsolationHTTP|TestAuthenticationOutageDoesNotInvalidateSession|TestActiveSessionRefreshHTTP|TestAutomaticResumeQueueAndLeaseHTTP|TestPersistentCoursewareOutageStopsAndRetainsPagesHTTP|TestCoursewareProviderBudgetPersistsAcrossRecoveryHTTP|TestReviewOutageAutomaticallyRechecksSavedPageHTTP|TestCoursewareConversationAndFeedbackHTTP|TestCoursewareConversationCancellationHTTP|TestCoursewareCanvasSavePreviewDownloadIsolationHTTP|TestTeacherRevisionRetainsCanvasAndReviewsHTTP|TestCoursewareRetryIsolationAndReservationHTTP|TestFunctionGraphIncludesOriginOnAxisBoundary|TestVisualReviewQuadraticUnitCoefficient'
if($LiveReview){$env:TEST_TEACHING_REVIEW_LIVE='1';$env:TEST_TEACHING_REVIEW_OUTPUT=Join-Path $report 'review';$pattern+='|^TestLiveTeachingRegressionCorpus$'}
if($FullDecks){$env:TEST_FULL_DECK_LIVE='1';$env:TEST_FULL_DECK_RUN=Split-Path $report -Leaf;$pattern+='|^TestCoursewareFullQualityLive$'}
if($OnlyFullDecks){if(!$FullDecks){throw 'OnlyFullDecks requires FullDecks.'};$pattern='^TestCoursewareFullQualityLive$'}
if($Topic){$env:TEST_FULL_DECK_TOPIC=$Topic}
if($WebImages){$env:TEST_FULL_DECK_WEB='1'}
if($Closure){$env:TEST_FULL_DECK_CLOSURE='1'}
if($RequireImage){$env:TEST_FULL_DECK_REQUIRE_IMAGE='1'}
if($Material){$env:TEST_FULL_DECK_MATERIAL=(Resolve-Path -LiteralPath $Material).Path}
$created=$false;$testExit=1
Push-Location (Join-Path $root 'backend-go')
try{
 & $go run ../scripts/test-database.go create $database
 if($LASTEXITCODE -ne 0){throw 'Isolated database creation failed.'};$created=$true
 & $go test -count=1 -json -timeout 40m -run $pattern . > (Join-Path $report 'go.jsonl') 2> (Join-Path $report 'go-stderr.log')
 $testExit=$LASTEXITCODE
 $events=Get-Content -LiteralPath (Join-Path $report 'go.jsonl')|ForEach-Object{$_|ConvertFrom-Json}
 $tests=@($events|Where-Object {$_.Test -and $_.Test -notmatch '/' -and $_.Action -in @('pass','fail','skip')})
 $skips=@($tests|Where-Object Action -eq 'skip')
 if(!$tests.Count -or $skips.Count){$testExit=1}
 $reviews=@();$reviewPath=Join-Path $report 'review/results.json'
 if(Test-Path -LiteralPath $reviewPath){$reviews=@(Get-Content -LiteralPath $reviewPath -Raw|ConvertFrom-Json)}
 $hashes=(Get-Content -LiteralPath (Join-Path $report 'source/manifest.json') -Raw|ConvertFrom-Json).files
 @{passed=($testExit -eq 0);tests=$tests.Count;failed=@($tests|Where-Object Action -eq 'fail').Count;skipped=$skips.Count;live_review=[bool]$LiveReview;full_decks=[bool]$FullDecks;review_pages=$reviews.Count;review_false_positives=@($reviews|Where-Object {$_.variant -eq 'good' -and !$_.passed}).Count;review_misses=@($reviews|Where-Object {$_.variant -eq 'bad' -and !$_.passed}).Count;source_hashes=$hashes;renderer_version=$health.courseware_canvas_version;teacher_classroom_accepted=$false}|ConvertTo-Json -Depth 5|Set-Content -LiteralPath (Join-Path $report 'summary.json') -Encoding UTF8
 Write-Output ('Quality gate: '+$tests.Count+' tests; '+$skips.Count+' skipped; result '+($testExit -eq 0)+'. Report: '+$report)
}finally{
 if($created){& $go run ../scripts/test-database.go drop $database;if($LASTEXITCODE -ne 0){throw 'Isolated database cleanup failed.'}}
 Pop-Location
}
exit $testExit
