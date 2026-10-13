param(
 [Parameter(Mandatory=$true)][string]$Path,
 [Parameter(Mandatory=$true)][string]$OutputDirectory,
 [ValidateRange(0,6)][int]$SamplePages=0
)
$ErrorActionPreference='Stop'
$file=(Resolve-Path -LiteralPath $Path).Path
$out=[IO.Path]::GetFullPath($OutputDirectory)
if ($out -ne ($file+'.slides')) {throw 'Preview output must be adjacent to its PPTX.'}
New-Item -ItemType Directory -Force -Path $out | Out-Null
$app=$null;$deck=$null
try {
 $app=New-Object -ComObject PowerPoint.Application
 $app.AutomationSecurity=3
 $deck=$app.Presentations.Open($file,$true,$false,$false)
 $selected=@()
 if($SamplePages -gt 0){
  $count=[Math]::Min($SamplePages,$deck.Slides.Count)
  for($i=0;$i -lt $count;$i++){$selected+=1+[Math]::Floor($i*($deck.Slides.Count-1)/[Math]::Max(1,$count-1))}
 }
 foreach($slide in $deck.Slides) {
  if($SamplePages -eq 0 -or $selected -contains $slide.SlideIndex){$slide.Export((Join-Path $out ('slide-'+$slide.SlideIndex+'.png')),'PNG',1280,720)}
  [void][Runtime.InteropServices.Marshal]::ReleaseComObject($slide)
 }
} finally {
 if($deck){$deck.Close();[void][Runtime.InteropServices.Marshal]::ReleaseComObject($deck)}
 # Do not Quit an Office instance the teacher may already be using.
 if($app){[void][Runtime.InteropServices.Marshal]::ReleaseComObject($app)}
}
