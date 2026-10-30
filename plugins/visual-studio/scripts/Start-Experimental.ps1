param([string]$Solution, [string]$HostExecutable)
$ErrorActionPreference = 'Stop'
$vswherePath = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
$vsInstance = (& $vswherePath -latest -products '*' -requires Microsoft.VisualStudio.Component.VSSDK -format json | ConvertFrom-Json) | Select-Object -First 1
if (!$vsInstance) { throw 'Visual Studio with VSSDK is not installed.' }
if (!$Solution) { $Solution = Join-Path $PSScriptRoot '..\SeMaps.VisualStudio.sln' }
$solutionPath = (Resolve-Path -LiteralPath $Solution).Path
$activityLogPath = Join-Path $env:TEMP 'semaps-vs-experimental-activity.xml'
$devenvPath = Join-Path $vsInstance.installationPath 'Common7\IDE\devenv.exe'
# This script explicitly launches the interactive test IDE requested by its caller.
$trialStart = [System.Diagnostics.ProcessStartInfo]::new()
$trialStart.FileName = $devenvPath
$trialStart.Arguments = '/RootSuffix SeMapsDev /Log "' + $activityLogPath + '" "' + $solutionPath + '"'
$trialStart.UseShellExecute = $false
$trialStart.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Normal
if ($HostExecutable) { $trialStart.EnvironmentVariables['SEMAPS_VS_HOST_EXE'] = (Resolve-Path -LiteralPath $HostExecutable).Path }
$trialProcess = [System.Diagnostics.Process]::Start($trialStart)
if (!$trialProcess) { throw 'Could not launch the experimental IDE.' }
$trialProcess.Dispose()
Write-Output "Experimental Visual Studio: SeMapsDev; activity log: $activityLogPath"
