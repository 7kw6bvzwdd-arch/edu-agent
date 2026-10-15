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
function Check-Endpoint([string]$key,[string]$address,[bool]$video=$false) {
	if($address -match '[\s\\]') { $problems.Add("${key}: remove whitespace and command-line continuation characters.");return }
 $parsed=$null
 if(![Uri]::TryCreate($address,[UriKind]::Absolute,[ref]$parsed) -or $parsed.Scheme -notin @('http','https') -or !$parsed.Host -or $parsed.UserInfo -or $parsed.Query -or $parsed.Fragment) {
  $problems.Add("${key}: use an HTTP(S) base URL without credentials, query or fragment.");return
 }
 if(!$video -and $parsed.AbsolutePath.TrimEnd('/') -match '/(responses|contents/generations/tasks)$') {
  $problems.Add("${key}: this app uses Chat Completions; use the API base URL or /chat/completions, not /responses or a video task path.")
 }
 if($video -and $parsed.AbsolutePath.TrimEnd('/') -match '/chat/completions$') {
  $problems.Add("${key}: a text chat endpoint cannot generate videos.")
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
if($reviewKey) {
 Check-Endpoint 'EVAL_LLM_BASE_URL' $reviewBase
 if(!$reviewModel) { $problems.Add('EVAL_LLM_MODEL: missing for configured reviewer.') }
 Write-Output 'Independent reviewer: fields present; online access not yet verified.'
} else { Write-Output 'Independent reviewer: disabled until EVAL_LLM_API_KEY is filled.' }
$videoProvider=Value 'VIDEO_PROVIDER' 'ark'
$visionKey=Value 'VISION_LLM_API_KEY'
if($visionKey) {
 Check-Endpoint 'VISION_LLM_BASE_URL' (Value 'VISION_LLM_BASE_URL')
 if(!(Value 'VISION_LLM_MODEL')) { $problems.Add('VISION_LLM_MODEL: missing for configured visual reviewer.') }
 Write-Output 'Visual reviewer: fields present; image support requires online verification.'
} else { Write-Output 'Visual reviewer: not configured; full video requires manual acceptance.' }
if($videoProvider -notin @('ark','minimax','dashscope')) { $problems.Add('VIDEO_PROVIDER: choose ark, minimax or dashscope.') }
$videoModel=Value 'VIDEO_MODEL' (Value 'ARK_VIDEO_MODEL')
if($videoModel) {
 $videoBase=Value 'VIDEO_BASE_URL' (Value 'ARK_VIDEO_BASE_URL' $(if($videoProvider -eq 'minimax'){'https://api.minimax.cn'}elseif($videoProvider -eq 'ark'){$base}else{''}))
 Check-Endpoint 'VIDEO_BASE_URL' $videoBase $true
 if($videoProvider -ne 'minimax' -and $videoBase -match '^https?://api\.minimax\.(cn|io)(/|$)') { $problems.Add('VIDEO_PROVIDER: set minimax for the MiniMax API.') }
 if(!(Value 'VIDEO_API_KEY' (Value 'ARK_VIDEO_API_KEY' $(if($videoProvider -eq 'ark'){$key}else{''})))) { $problems.Add('VIDEO_API_KEY: missing; MiniMax and DashScope require their own key.') }
 if($videoModel -match '^https?://') { $problems.Add('VIDEO_MODEL: enter an enabled video model/endpoint ID, not a URL.') }
 $seconds=0
 if(![int]::TryParse((Value 'VIDEO_DURATION' (Value 'ARK_VIDEO_DURATION' '5')),[ref]$seconds) -or $seconds -lt 2 -or $seconds -gt 15) {
  $problems.Add('VIDEO_DURATION: enter an integer from 2 to 15 supported by the selected model.')
 }
 if($videoProvider -eq 'minimax' -and ($videoModel -notin @('MiniMax-H3','MiniMax-H3-Max') -or $seconds -lt 4 -or ($videoModel -eq 'MiniMax-H3-Max' -and $seconds -lt 5))) { $problems.Add('MiniMax: use MiniMax-H3 (4-15 seconds) or MiniMax-H3-Max (5-15 seconds).') }
 if($videoProvider -eq 'dashscope') {
  if($videoModel -notin @('wan2.7-t2v','wan2.7-t2v-2026-06-12')) { $problems.Add('DashScope: use wan2.7-t2v; image-to-video and chat models are not supported here.') }
  $uri=$null
  if([Uri]::TryCreate($videoBase,[UriKind]::Absolute,[ref]$uri) -and $uri.AbsolutePath.TrimEnd('/') -notin @('','/api/v1','/api/v1/services/aigc/video-generation/video-synthesis')) { $problems.Add('VIDEO_BASE_URL: use the host or /api/v1, not compatible-mode/v1.') }
 }
 Write-Output 'Cloud video: configured locally; actual model access still requires online verification.'
} else { Write-Output 'Cloud video: disabled until VIDEO_MODEL is filled; text and Manim can still run.' }
Write-Output 'No secrets printed, no network requests sent, and no configuration files changed.'
Write-Output 'Changes to .env take effect after restarting the local services.'
if($problems.Count) { $problems | ForEach-Object { Write-Output "ERROR $_" }; exit 1 }
Write-Output 'Configuration shape check passed. This does not verify credentials or model availability.'
exit 0
