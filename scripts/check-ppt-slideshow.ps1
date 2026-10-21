param(
 [Parameter(Mandatory=$true)][string]$Path,
 [Parameter(Mandatory=$true)][string]$OutputDirectory,
 [ValidateSet('gif','click')][string]$Mode='gif',
 [int[]]$Pages=@(),
 [ValidateRange(3,12)][int]$Frames=6
)
$ErrorActionPreference='Stop'
$file=(Resolve-Path -LiteralPath $Path).Path
$out=[IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $out | Out-Null
Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class PptCapture {
 [StructLayout(LayoutKind.Sequential)] public struct Rect { public int Left,Top,Right,Bottom; }
 [DllImport("user32.dll")] public static extern bool GetClientRect(IntPtr hwnd,out Rect rect);
 [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr hwnd,IntPtr hdc,uint flags);
 [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern IntPtr FindWindow(string className,string title);
 private delegate bool EnumCallback(IntPtr hwnd,IntPtr parameter);
 [DllImport("user32.dll")] private static extern bool EnumWindows(EnumCallback callback,IntPtr parameter);
 [DllImport("user32.dll")] private static extern bool IsWindowVisible(IntPtr hwnd);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] private static extern int GetWindowText(IntPtr hwnd,StringBuilder text,int count);
 public static IntPtr Slideshow(string filename) {
  IntPtr result=FindWindow("screenClass",null);
  if(result!=IntPtr.Zero)return result;
  EnumWindows((hwnd,parameter)=>{
   var title=new StringBuilder(512);GetWindowText(hwnd,title,512);
   string value=title.ToString();
   if(IsWindowVisible(hwnd)&&value.Contains(filename)&&(value.Contains("Slide Show")||value.Contains("幻灯片放映"))){result=hwnd;return false;}
   return true;
  },IntPtr.Zero);
  return result;
 }
}
'@
[void][PptCapture]::SetProcessDPIAware()
$lock=[Threading.Mutex]::new($false,'Local\EduAgentPowerPointPreview')
$locked=$false;$app=$null;$deck=$null;$show=$null;$records=@()
function Capture($window,$target) {
 $hwnd=[PptCapture]::Slideshow([IO.Path]::GetFileName($file))
 if($hwnd -eq [IntPtr]::Zero){throw 'Cannot locate the actual PowerPoint slideshow window.'}
 $rect=[PptCapture+Rect]::new()
 if(![PptCapture]::GetClientRect($hwnd,[ref]$rect)){throw 'Cannot locate slideshow client area.'}
 $width=$rect.Right-$rect.Left;$height=$rect.Bottom-$rect.Top
 if($width -lt 320 -or $height -lt 180){throw 'Slideshow window is too small.'}
 $bitmap=[Drawing.Bitmap]::new($width,$height)
 $graphics=[Drawing.Graphics]::FromImage($bitmap)
 $dc=$graphics.GetHdc()
 try {if(![PptCapture]::PrintWindow($hwnd,$dc,3)){throw 'Slideshow capture failed.'}}
 finally {$graphics.ReleaseHdc($dc);$graphics.Dispose()}
 try {$bitmap.Save($target,[Drawing.Imaging.ImageFormat]::Png)} finally {$bitmap.Dispose()}
}
try {
 try {$locked=$lock.WaitOne(55000)} catch [Threading.AbandonedMutexException] {$locked=$true}
 if(!$locked){throw 'PowerPoint is busy; retry without interrupting an existing render.'}
 $app=New-Object -ComObject PowerPoint.Application
 $app.AutomationSecurity=3
 $app.Visible=-1
 if($app.SlideShowWindows.Count -gt 0){throw 'An existing slideshow is active; leave it open and retry later.'}
 $deck=$app.Presentations.Open($file,$true,$false,$true)
 if(!$Pages.Count){$Pages=@(1..$deck.Slides.Count)}
 $settings=$deck.SlideShowSettings
 $settings.ShowType=2
 $settings.ShowWithAnimation=-1
 $settings.AdvanceMode=1
 $show=$settings.Run()
 $show.Width=960;$show.Height=540
 Start-Sleep -Milliseconds 800
 foreach($page in $Pages){
  if($page -lt 1 -or $page -gt $deck.Slides.Count){throw 'Page outside deck.'}
  $show.View.GotoSlide($page,$true)
  Start-Sleep -Milliseconds 250
  $images=@()
  $count=if($Mode -eq 'click'){3}else{$Frames}
  for($frame=0;$frame -lt $count;$frame++){
   if($Mode -eq 'click' -and $frame -gt 0){$show.View.Next();Start-Sleep -Milliseconds 350}
   $target=Join-Path $out ('slide-'+$page+'-'+$frame+'.png')
   Capture $show $target
   $images+=@{frame=$frame;file=$target;captured_at=(Get-Date).ToString('o')}
   if($Mode -eq 'gif'){Start-Sleep -Milliseconds 370}
  }
  $records+=@{page=$page;mode=$Mode;images=$images;effects=$deck.Slides.Item($page).TimeLine.MainSequence.Count}
 }
 @{mode='fixed_render_fixture';source=$file;powerpoint_version=$app.Version;frames=$records;human_classroom_accepted=$false;capture='actual_slide_show_window'}|ConvertTo-Json -Depth 8|Set-Content -LiteralPath (Join-Path $out 'slideshow.json') -Encoding utf8
} finally {
 if($show){try{$show.View.Exit()}catch{};[void][Runtime.InteropServices.Marshal]::ReleaseComObject($show)}
 if($deck){try{$deck.Close()}catch{Write-Warning 'Presentation was already closed during slideshow cleanup.'};[void][Runtime.InteropServices.Marshal]::ReleaseComObject($deck)}
 if($app){[void][Runtime.InteropServices.Marshal]::ReleaseComObject($app)}
 if($locked){$lock.ReleaseMutex()};$lock.Dispose()
}
Write-Output ('Actual slideshow frames saved: '+$out)
