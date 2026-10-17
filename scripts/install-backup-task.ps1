param([string]$At='03:30')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$python=(Get-Command python -ErrorAction Stop).Source
$action=New-ScheduledTaskAction -Execute $python -Argument ('"'+(Join-Path $root 'scripts/backup.py')+'" --drill') -WorkingDirectory $root
$trigger=New-ScheduledTaskTrigger -Daily -At $At
$principal=New-ScheduledTaskPrincipal -UserId ([Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive -RunLevel Limited
$settings=New-ScheduledTaskSettingsSet -StartWhenAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Hours 2)
Register-ScheduledTask -TaskName 'EduAgent-DailyBackup' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description 'Project-only PostgreSQL/storage/MinIO backup and isolated restore drill; credentials stay in local configuration.' -Force | Select-Object TaskName,State
