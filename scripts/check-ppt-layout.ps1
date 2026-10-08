param([Parameter(Mandatory=$true)][string]$Directory)
$ErrorActionPreference='Stop'
$root=(Resolve-Path -LiteralPath $Directory).Path
$files=@(Get-ChildItem -LiteralPath $root -Filter '*.pptx' -File)
if (!$files.Count) {throw 'No PPTX files to inspect.'}
$pptApp=New-Object -ComObject PowerPoint.Application
$results=@()
function Check-Text($shape,$page,$label) {
 if ($shape.HasTextFrame -and $shape.TextFrame.HasText) {
  $r=$shape.TextFrame.TextRange
  try {
   if ($r.BoundHeight -gt ($shape.Height+1) -or $r.BoundWidth -gt ($shape.Width+1)) {
    return @{slide=$page;object=$label;text=$r.Text}
   }
  } finally {[void][Runtime.InteropServices.Marshal]::ReleaseComObject($r)}
 }
}
try {
 foreach ($file in $files) {
  $deck=$null;$overflows=@()
  $out=Join-Path $root $file.BaseName
  New-Item -ItemType Directory -Force -Path $out | Out-Null
  try {
   $deck=$pptApp.Presentations.Open($file.FullName,$true,$false,$false)
   foreach($slide in $deck.Slides) {
    $slide.Export((Join-Path $out ('slide-'+$slide.SlideIndex+'.png')),'PNG',1280,720)
    foreach($shape in $slide.Shapes) {
     $overflows+=@(Check-Text $shape $slide.SlideIndex $shape.Name)
     if ($shape.HasTable) {
      for($row=1;$row -le $shape.Table.Rows.Count;$row++) {
       for($col=1;$col -le $shape.Table.Columns.Count;$col++) {
        $cellShape=$shape.Table.Cell($row,$col).Shape
        $overflows+=@(Check-Text $cellShape $slide.SlideIndex "cell-$row-$col")
        [void][Runtime.InteropServices.Marshal]::ReleaseComObject($cellShape)
       }
      }
     }
     [void][Runtime.InteropServices.Marshal]::ReleaseComObject($shape)
    }
    [void][Runtime.InteropServices.Marshal]::ReleaseComObject($slide)
   }
   $results+=@{file=$file.Name;slides=$deck.Slides.Count;overflows=$overflows}
  } finally {if($deck){$deck.Close();[void][Runtime.InteropServices.Marshal]::ReleaseComObject($deck)}}
 }
} finally {[void][Runtime.InteropServices.Marshal]::ReleaseComObject($pptApp)}
$results | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $root 'layout-qa.json') -Encoding utf8
$results | ConvertTo-Json -Depth 5
if (@($results | Where-Object {$_.overflows.Count -gt 0}).Count) {throw 'PPT text overflow detected.'}
