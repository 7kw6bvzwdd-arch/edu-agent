param(
 [Parameter(Mandatory=$true)][string]$Path,
 [Parameter(Mandatory=$true)][string]$OutputDirectory
)
$ErrorActionPreference='Stop'
$file=(Resolve-Path -LiteralPath $Path).Path
$out=[IO.Path]::GetFullPath($OutputDirectory)
if ($out -ne ($file+'.slides')) {throw 'Preview output must be adjacent to its PPTX.'}
New-Item -ItemType Directory -Force -Path $out | Out-Null
$app=$null;$deck=$null
try {
 $app=New-Object -ComObject PowerPoint.Application
 $deck=$app.Presentations.Open($file,$true,$false,$false)
 foreach($slide in $deck.Slides) {
  $slide.Export((Join-Path $out ('slide-'+$slide.SlideIndex+'.png')),'PNG',1280,720)
  [void][Runtime.InteropServices.Marshal]::ReleaseComObject($slide)
 }
} finally {
 if($deck){$deck.Close();[void][Runtime.InteropServices.Marshal]::ReleaseComObject($deck)}
 # Do not Quit an Office instance the teacher may already be using.
 if($app){[void][Runtime.InteropServices.Marshal]::ReleaseComObject($app)}
}
