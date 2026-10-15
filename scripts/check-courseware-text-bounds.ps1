param([Parameter(Mandatory=$true)][string]$Path,[Parameter(Mandatory=$true)][string]$Report)
$ErrorActionPreference='Stop'
$file=(Resolve-Path -LiteralPath $Path).Path
$app=$null;$deck=$null;$issues=@()
try{
 $app=New-Object -ComObject PowerPoint.Application
 $app.AutomationSecurity=3
 $deck=$app.Presentations.Open($file,$true,$false,$false)
 $count=$deck.Slides.Count
 foreach($slide in $deck.Slides){
  foreach($shape in $slide.Shapes){
   if($shape.HasTextFrame -and $shape.TextFrame.HasText){
    $range=$shape.TextFrame.TextRange
    $widthOverflow=$range.BoundWidth -gt ($shape.Width+1)
    if($widthOverflow){
     # PowerPoint includes invisible trailing ideographic spaces in BoundWidth.
     # Keep the actual PPTX intact and measure each line's visible characters.
     $widthOverflow=$false
     for($lineIndex=1;$lineIndex -le $range.Lines().Count;$lineIndex++){
      $line=$range.Lines($lineIndex,1);$visible=$line.Text.TrimEnd()
      if($visible.Length){
       $visibleRange=$line.Characters(1,$visible.Length)
       if($visibleRange.BoundWidth -gt ($shape.Width+1)){$widthOverflow=$true}
       [void][Runtime.InteropServices.Marshal]::ReleaseComObject($visibleRange)
      }
      [void][Runtime.InteropServices.Marshal]::ReleaseComObject($line)
     }
    }
    if($range.BoundHeight -gt ($shape.Height+1) -or $widthOverflow){$issues+=@{page=$slide.SlideIndex;shape=$shape.Name}}
    [void][Runtime.InteropServices.Marshal]::ReleaseComObject($range)
   }
   [void][Runtime.InteropServices.Marshal]::ReleaseComObject($shape)
  }
  [void][Runtime.InteropServices.Marshal]::ReleaseComObject($slide)
 }
 @{slides=$count;overflows=@($issues)}|ConvertTo-Json -Depth 4|Set-Content -LiteralPath $Report -Encoding utf8
}finally{
 if($deck){$deck.Close();[void][Runtime.InteropServices.Marshal]::ReleaseComObject($deck)}
 if($app){[void][Runtime.InteropServices.Marshal]::ReleaseComObject($app)}
}
if($issues.Count){exit 1}
exit 0
