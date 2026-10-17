param([string]$EnvFile=(Join-Path (Split-Path -Parent $PSScriptRoot) '.env'))
$ErrorActionPreference='Stop'
if (!(Test-Path -LiteralPath $EnvFile -PathType Leaf)) { throw 'Configuration file does not exist.' }
$settings=@{}
foreach($line in Get-Content -LiteralPath $EnvFile) {
 if($line -match '^([^#=]+)=(.*)$') { $settings[$matches[1].Trim()]=$matches[2].Trim() }
}
$problems=[Collections.Generic.List[string]]::new()
function Value([string]$key,[string]$fallback='') {
 if($settings[$key]) { return $settings[$key] }
 return $fallback
}
function Check-Endpoint([string]$key,[string]$address) {
	if($address -match '[\s\\]') { $problems.Add("${key}: remove whitespace and command-line continuation characters.");return }
 $parsed=$null
 if(![Uri]::TryCreate($address,[UriKind]::Absolute,[ref]$parsed) -or $parsed.Scheme -notin @('http','https') -or !$parsed.Host -or $parsed.UserInfo -or $parsed.Query -or $parsed.Fragment) {
  $problems.Add("${key}: use an HTTP(S) base URL without credentials, query or fragment.");return
 }
 if( $parsed.AbsolutePath.TrimEnd('/') -match '/(responses|contents/generations/tasks)$') {
  $problems.Add("${key}: this app uses Chat Completions; use the API base URL or /chat/completions, not /responses or a video task path.")
 }
}
$base=Value 'LLM_BASE_URL' 'https://ark.cn-beijing.volces.com/api/v3'
$key=Value 'LLM_API_KEY'
$model=Value 'LLM_MODEL'
$mode=Value 'APP_MODE' 'sample'
if($mode -notin @('live','sample')) { $problems.Add('APP_MODE: choose live or sample.') }
Write-Output "Mode: $mode"
foreach($agent in @('COURSEWARE','ANIMATION')) {
 $agentURL=Value "${agent}_LLM_BASE_URL" $base
 $agentKey=Value "${agent}_LLM_API_KEY" $key
 $agentModel=Value "${agent}_LLM_MODEL" $model
 if($mode -eq 'live') {
  Check-Endpoint "${agent}_LLM_BASE_URL" $agentURL
  if(!$agentKey) { $problems.Add("${agent}_LLM_API_KEY: missing (or set shared LLM_API_KEY).") }
  if(!$agentModel) { $problems.Add("${agent}_LLM_MODEL: missing (or set shared LLM_MODEL).") }
  if($agentModel -match '^https?://') { $problems.Add("${agent}_LLM_MODEL: enter a model/endpoint ID, not a URL.") }
 }
 Write-Output ("{0} text configuration: {1}" -f $agent, $(if($agentKey -and $agentModel){'fields present'}else{'incomplete'}))
}
$reviewBase=Value 'EVAL_LLM_BASE_URL'
$reviewKey=Value 'EVAL_LLM_API_KEY'
$reviewModel=Value 'EVAL_LLM_MODEL'
$policy=Value 'COURSEWARE_REVIEW_POLICY' 'key'
if($policy -notin @('key','all','none')){$problems.Add('COURSEWARE_REVIEW_POLICY: choose key, all or none.')}
$reviewEnabled=(Value 'COURSEWARE_CONTENT_REVIEW_DISABLED' 'false') -ne 'true' -and $policy -ne 'none'
if($mode -eq 'live' -and $reviewEnabled -and !$reviewKey){$problems.Add('EVAL_LLM_API_KEY: independent content reviewer is required by the selected policy.')}
if($reviewKey) {
 Check-Endpoint 'EVAL_LLM_BASE_URL' $reviewBase
 if(!$reviewModel) { $problems.Add('EVAL_LLM_MODEL: missing for configured reviewer.') }
 $generatorURL=(Value 'COURSEWARE_LLM_BASE_URL' $base).TrimEnd('/') -replace '/chat/completions$',''
 $reviewURL=$reviewBase.TrimEnd('/') -replace '/chat/completions$',''
 if($reviewEnabled -and $generatorURL -eq $reviewURL -and (Value 'COURSEWARE_LLM_MODEL' $model) -eq $reviewModel){$problems.Add('EVAL_LLM_MODEL: choose a reviewer different from the courseware generator.')}
 Write-Output 'Independent reviewer: fields present; online access not yet verified.'
} else { Write-Output 'Independent reviewer: disabled until EVAL_LLM_API_KEY is filled.' }
$visionKey=Value 'VISION_LLM_API_KEY'
if($visionKey) {
 Check-Endpoint 'VISION_LLM_BASE_URL' (Value 'VISION_LLM_BASE_URL')
 if(!(Value 'VISION_LLM_MODEL')) { $problems.Add('VISION_LLM_MODEL: missing for configured visual reviewer.') }
 Write-Output 'Visual reviewer: fields present; image support requires online verification.'
} else { Write-Output 'Visual reviewer: not configured; courseware image review requires configuration.' }
Write-Output 'No secrets printed, no network requests sent, and no configuration files changed.'
Write-Output 'Changes to .env take effect after restarting the local services.'
if($problems.Count) { $problems | ForEach-Object { Write-Output "ERROR $_" }; exit 1 }
Write-Output 'Configuration shape check passed. This does not verify credentials or model availability.'
exit 0
