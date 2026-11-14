<#
.SYNOPSIS
  Measures how light the Peregrine desktop app is on Windows: memory and idle CPU of the
  app process plus every WebView2 process it starts.

.DESCRIPTION
  Memory is "private working set" (what Task Manager shows as Memory), summed over the
  whole process tree. CPU is the tree's CPU time over the sampling window, as a percentage
  of one core. The app is launched fresh and closed again at the end.

  Note: it uses your normal Peregrine profile (settings and queue in %APPDATA%\Peregrine).

.EXAMPLE
  powershell -File scripts\footprint.ps1
  powershell -File scripts\footprint.ps1 -Hidden          # start in the tray, no window
  powershell -File scripts\footprint.ps1 -Exe path\to\Peregrine.exe -Settle 60 -Window 60
#>
param(
  [string]$Exe = "$PSScriptRoot\..\desktop\build\bin\Peregrine.exe",
  [switch]$Hidden,
  [int]$Settle = 45,  # seconds to wait after launch before measuring
  [int]$Window = 60   # seconds of idle CPU sampling
)
$ErrorActionPreference = 'Stop'

# Works for other apps too (to compare), as long as they are not already running.
$name = [IO.Path]::GetFileNameWithoutExtension($Exe)
if (Get-Process $name -ErrorAction SilentlyContinue) {
  throw "$name is already running; close it first (single-instance apps hand over to it)."
}

function Get-Tree([int]$root) {
  $all = Get-CimInstance Win32_Process
  $ids = [System.Collections.Generic.List[int]]::new(); $ids.Add($root)
  $q = [System.Collections.Generic.Queue[int]]::new(); $q.Enqueue($root)
  while ($q.Count) {
    $c = $q.Dequeue()
    foreach ($x in $all | Where-Object { $_.ParentProcessId -eq $c }) { $ids.Add([int]$x.ProcessId); $q.Enqueue([int]$x.ProcessId) }
  }
  $ids
}

function Get-CpuSeconds($ids) {
  ($ids | ForEach-Object { Get-Process -Id $_ -ErrorAction SilentlyContinue } | Measure-Object CPU -Sum).Sum
}

$p = if ($Hidden) { Start-Process $Exe -ArgumentList '--hidden' -PassThru } else { Start-Process $Exe -PassThru }
try {
  Start-Sleep $Settle
  $ids = Get-Tree $p.Id
  $c0 = Get-CpuSeconds $ids
  Start-Sleep $Window
  $c1 = Get-CpuSeconds $ids
  $ids = Get-Tree $p.Id
  $perf = Get-CimInstance Win32_PerfFormattedData_PerfProc_Process | Where-Object { $ids -contains $_.IDProcess }
  $mb = { param($b) [math]::Round($b / 1MB, 1) }

  $perf | Sort-Object WorkingSetPrivate -Descending |
    Select-Object Name, IDProcess, @{ n = 'PrivateMB'; e = { & $mb $_.WorkingSetPrivate } } |
    Format-Table -AutoSize | Out-String | Write-Output

  [pscustomobject]@{
    Mode          = if ($Hidden) { 'hidden (tray)' } else { 'window' }
    Processes     = $ids.Count
    TotalMB       = & $mb (($perf | Measure-Object WorkingSetPrivate -Sum).Sum)
    EngineMB      = & $mb (($perf | Where-Object IDProcess -eq $p.Id).WorkingSetPrivate)
    IdleCpuPct1Core = [math]::Round(($c1 - $c0) / $Window * 100, 2)
  } | Format-List | Out-String | Write-Output
}
finally {
  Stop-Process -Id $p.Id -ErrorAction SilentlyContinue
}
