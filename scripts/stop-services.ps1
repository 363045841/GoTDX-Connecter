#Requires -Version 5.1
<#
.SYNOPSIS
  关闭 KlineChartQuantGo 历史进程（按端口 / 进程名）。

.EXAMPLE
  .\scripts\stop-services.ps1           # tdx + binance
  .\scripts\stop-services.ps1 tdx       # 仅 8080 / tdx
  .\scripts\stop-services.ps1 binance   # 仅 8081 / binance
  .\scripts\stop-services.ps1 -Force    # 不询问
#>
[CmdletBinding(SupportsShouldProcess = $true)]
param(
  [Parameter(Position = 0)]
  [ValidateSet('all', 'tdx', 'binance')]
  [string]$Service = 'all',

  [switch]$Force
)

$ErrorActionPreference = 'Continue'

$targets = switch ($Service) {
  'tdx' {
    @{ Ports = @(8080); Names = @('tdx-api', 'KlineChartQuantGo') }
  }
  'binance' {
    @{ Ports = @(8081); Names = @('binance-api', 'KlineChartQuantGo') }
  }
  default {
    @{ Ports = @(8080, 8081); Names = @('tdx-api', 'binance-api', 'KlineChartQuantGo') }
  }
}

function Get-PortOwners {
  param([int[]]$Ports)
  $ids = [System.Collections.Generic.HashSet[int]]::new()
  foreach ($port in $Ports) {
    Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue |
      Where-Object { $_.State -in @('Listen', 'Established', 'TimeWait', 'CloseWait') } |
      ForEach-Object {
        if ($_.OwningProcess -and $_.OwningProcess -gt 0) {
          [void]$ids.Add([int]$_.OwningProcess)
        }
      }
  }
  return @($ids)
}

function Get-NamedOwners {
  param([string[]]$Names)
  $ids = [System.Collections.Generic.HashSet[int]]::new()
  foreach ($name in $Names) {
    Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
      # go run . tdx 会留下父 launcher + 子 tdx-api；两边都杀
      if ($name -eq 'KlineChartQuantGo') {
        $cmd = (Get-CimInstance Win32_Process -Filter "ProcessId=$($_.Id)" -ErrorAction SilentlyContinue).CommandLine
        if ($Service -eq 'tdx' -and $cmd -notmatch '\b(tdx|tdx-api|gotdx)\b') { continue }
        if ($Service -eq 'binance' -and $cmd -notmatch '\b(binance|binance-api)\b') { continue }
      }
      [void]$ids.Add([int]$_.Id)
    }
  }
  return @($ids)
}

$portPids = Get-PortOwners -Ports $targets.Ports
$namePids = Get-NamedOwners -Names $targets.Names
$allPids = @($portPids + $namePids | Select-Object -Unique | Sort-Object)

if (-not $allPids -or $allPids.Count -eq 0) {
  Write-Host "No matching processes for service='$Service' (ports: $($targets.Ports -join ', '))."
  exit 0
}

Write-Host "Targets (service=$Service):"
foreach ($procId in $allPids) {
  $proc = Get-Process -Id $procId -ErrorAction SilentlyContinue
  $cmd = (Get-CimInstance Win32_Process -Filter "ProcessId=$procId" -ErrorAction SilentlyContinue).CommandLine
  $ports = @(
    Get-NetTCPConnection -OwningProcess $procId -ErrorAction SilentlyContinue |
      Select-Object -ExpandProperty LocalPort -Unique
  )
  $portText = if ($ports) { " ports=$($ports -join ',')" } else { '' }
  $name = if ($proc) { $proc.ProcessName } else { '?' }
  Write-Host ("  PID={0,-6} name={1}{2}" -f $procId, $name, $portText)
  if ($cmd) { Write-Host "           $cmd" }
}

if (-not $Force -and -not $PSCmdlet.ShouldProcess(($allPids -join ','), 'Stop-Process -Force')) {
  $answer = Read-Host "Kill these processes? [y/N]"
  if ($answer -notmatch '^(y|yes)$') {
    Write-Host 'Aborted.'
    exit 1
  }
}

$failed = @()
foreach ($procId in $allPids) {
  try {
    Stop-Process -Id $procId -Force -ErrorAction Stop
    Write-Host "Stopped PID $procId"
  } catch {
    Write-Warning "Failed PID $procId : $($_.Exception.Message)"
    $failed += $procId
  }
}

Start-Sleep -Milliseconds 400

$still = Get-PortOwners -Ports $targets.Ports
if ($still.Count -gt 0) {
  Write-Warning "Ports still held by: $($still -join ', ')"
  exit 2
}

if ($failed.Count -gt 0) {
  exit 1
}

Write-Host "Done. Ports $($targets.Ports -join ', ') are free."
