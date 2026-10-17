param([string]$ProjectRoot=(Split-Path -Parent $PSScriptRoot))
# Keep only this project's private tunnel alive; never restart cloud containers.
$ErrorActionPreference='Stop'
while($true){
 try { & (Join-Path $PSScriptRoot 'start-cloud.ps1') -NoWatch -ProjectRoot $ProjectRoot }
 catch { Write-Warning ('Project cloud tunnel unavailable; retrying: '+$_.Exception.Message) }
 Start-Sleep -Seconds 5
}
