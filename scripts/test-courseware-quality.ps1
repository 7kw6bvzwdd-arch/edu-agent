param(
 [string]$Renderer='http://127.0.0.1:8000',
 [switch]$LiveReview,
 [switch]$FullDecks
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
$databaseAdminURL=$env:TEST_DATABASE_ADMIN_URL
if(!$databaseAdminURL){$databaseAdminURL='postgres://edu:edu-local-only@127.0.0.1:55432/postgres?sslmode=disable'}
$env:TEST_DATABASE_ADMIN_URL=$databaseAdminURL
$uri=[UriBuilder]::new($databaseAdminURL);$uri.Path='/'+$database
$env:TEST_DATABASE_URL=$uri.Uri.AbsoluteUri
$env:TEST_RUNTIME_DATABASE_URL='';$env:TEST_RENDER_URL=$Renderer
# Explicit opt-ins; an inherited live/replay setting must not silently change a run.
$env:TEST_FULL_DECK_LIVE='';$env:TEST_TEACHING_REVIEW_LIVE=''
$env:TEST_FULL_DECK_RESUME_ID='';$env:TEST_FULL_DECK_PLAN_FIXTURE='';$env:TEST_FULL_DECK_CHECKPOINT_FIXTURE=''
$env:TEST_FULL_DECK_TOPIC='';$env:TEST_TEACHING_REVIEW_CASES=''
if($LiveReview -or $FullDecks){
 Get-Content -LiteralPath (Join-Path $root '.env') -Encoding UTF8|ForEach-Object{if($_ -match '^([^#=]+)=(.*)$'){[Environment]::SetEnvironmentVariable($matches[1].Trim(),$matches[2].Trim(),'Process')}}
}
$pattern='TestTeachingRegressionCorpus|TestCoreTeachingSourcesArePublicSnapshotsHTTP|TestTextbookClaimEvidenceWorkflowHTTP|TestDocumentReparseKeepsIdentityAndSavedSourceHTTP|TestTeachingMaterialsAndReferencesIsolationHTTP|TestAuthenticationOutageDoesNotInvalidateSession|TestActiveSessionRefreshHTTP|TestAutomaticResumeQueueAndLeaseHTTP|TestPersistentCoursewareOutageStopsAndRetainsPagesHTTP|TestCoursewareProviderBudgetPersistsAcrossRecoveryHTTP|TestReviewOutageAutomaticallyRechecksSavedPageHTTP|TestCoursewareConversationAndFeedbackHTTP|TestCoursewareConversationCancellationHTTP|TestCoursewareCanvasSavePreviewDownloadIsolationHTTP|TestTeacherRevisionRetainsCanvasAndReviewsHTTP|TestCoursewareRetryIsolationAndReservationHTTP|TestFunctionGraphIncludesOriginOnAxisBoundary|TestVisualReviewQuadraticUnitCoefficient'
if($LiveReview){$env:TEST_TEACHING_REVIEW_LIVE='1';$env:TEST_TEACHING_REVIEW_OUTPUT=Join-Path $report 'review';$pattern+='|^TestLiveTeachingRegressionCorpus$'}
if($FullDecks){$env:TEST_FULL_DECK_LIVE='1';$env:TEST_FULL_DECK_RUN=Split-Path $report -Leaf;$pattern+='|^TestCoursewareFullQualityLive$'}
$created=$false;$testExit=1
Push-Location (Join-Path $root 'backend-go')
try{
 & $go run ../scripts/test-database.go create $database
 if($LASTEXITCODE -ne 0){throw 'Isolated database creation failed.'};$created=$true
 & $go test -count=1 -json -timeout 300m -run $pattern . > (Join-Path $report 'go.jsonl') 2> (Join-Path $report 'go-stderr.log')
 $testExit=$LASTEXITCODE
 $events=Get-Content -LiteralPath (Join-Path $report 'go.jsonl')|ForEach-Object{$_|ConvertFrom-Json}
 $tests=@($events|Where-Object {$_.Test -and $_.Test -notmatch '/' -and $_.Action -in @('pass','fail','skip')})
 $skips=@($tests|Where-Object Action -eq 'skip')
 if(!$tests.Count -or $skips.Count){$testExit=1}
 $reviews=@();$reviewPath=Join-Path $report 'review/results.json'
 if(Test-Path -LiteralPath $reviewPath){$reviews=@(Get-Content -LiteralPath $reviewPath -Raw|ConvertFrom-Json)}
 $hashes=@{}
 foreach($path in @('backend-go/courseware_page_review.go','backend-go/courseware_teaching_review.go','backend-go/testdata/teaching-review-regressions.json','backend-go/testdata/teaching-review-holdouts.json','render-service-py/app.py','render-service-py/pdf_extract.py')){
  $hashes[$path]=(Get-FileHash -LiteralPath (Join-Path $root $path) -Algorithm SHA256).Hash
 }
 @{passed=($testExit -eq 0);tests=$tests.Count;failed=@($tests|Where-Object Action -eq 'fail').Count;skipped=$skips.Count;live_review=[bool]$LiveReview;full_decks=[bool]$FullDecks;review_pages=$reviews.Count;review_false_positives=@($reviews|Where-Object {$_.variant -eq 'good' -and !$_.passed}).Count;review_misses=@($reviews|Where-Object {$_.variant -eq 'bad' -and !$_.passed}).Count;source_hashes=$hashes;renderer_version=$health.courseware_canvas_version;teacher_classroom_accepted=$false}|ConvertTo-Json -Depth 5|Set-Content -LiteralPath (Join-Path $report 'summary.json') -Encoding UTF8
 Write-Output ('Quality gate: '+$tests.Count+' tests; '+$skips.Count+' skipped; result '+($testExit -eq 0)+'. Report: '+$report)
}finally{
 if($created){& $go run ../scripts/test-database.go drop $database;if($LASTEXITCODE -ne 0){throw 'Isolated database cleanup failed.'}}
 Pop-Location
}
exit $testExit
