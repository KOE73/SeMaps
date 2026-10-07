param([string]$Solution)
$ErrorActionPreference = 'Stop'
$vswherePath = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
$vsInstance = (& $vswherePath -latest -products '*' -requires Microsoft.VisualStudio.Component.VSSDK -format json | ConvertFrom-Json) | Select-Object -First 1
if (!$vsInstance) { throw 'Visual Studio with VSSDK is not installed.' }
if (!$Solution) { $Solution = Join-Path $PSScriptRoot '..\SeMaps.VisualStudio.sln' }
$solutionPath = (Resolve-Path -LiteralPath $Solution).Path
$activityLogPath = Join-Path $env:TEMP 'semaps-vs-experimental-activity.xml'
$devenvPath = Join-Path $vsInstance.installationPath 'Common7\IDE\devenv.exe'
# This script explicitly launches the interactive test IDE requested by its caller.
Start-Process -FilePath $devenvPath -ArgumentList @('/RootSuffix', 'SeMapsDev', '/Log', ('"' + $activityLogPath + '"'), ('"' + $solutionPath + '"')) -WindowStyle Normal
Write-Output "Experimental Visual Studio: SeMapsDev; activity log: $activityLogPath"
