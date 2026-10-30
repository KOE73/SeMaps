param(
    [ValidateSet('Debug', 'Release')][string]$Configuration = 'Debug',
    [switch]$Deploy
)
$ErrorActionPreference = 'Stop'
$vswherePath = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
if (!(Test-Path -LiteralPath $vswherePath)) { throw 'Visual Studio Installer/vswhere is not installed.' }
$vsInstance = (& $vswherePath -latest -products '*' -requires Microsoft.VisualStudio.Component.VSSDK -format json | ConvertFrom-Json) | Select-Object -First 1
if (!$vsInstance) { throw 'Install the Visual Studio extension development workload (VSSDK).' }
$msbuildPath = Join-Path $vsInstance.installationPath 'MSBuild\Current\Bin\MSBuild.exe'
$vsixProjectPath = Join-Path $PSScriptRoot '..\src\SeMaps.VisualStudio\SeMaps.VisualStudio.csproj'
$deployValue = if ($Deploy) { 'true' } else { 'false' }
& $msbuildPath $vsixProjectPath -restore -t:Build "-p:Configuration=$Configuration" "-p:DeployExtension=$deployValue" "-p:DeployTargetInstanceId=$($vsInstance.instanceId)" -p:VSSDKTargetPlatformRegRootSuffix=SeMapsDev -verbosity:minimal -nologo
if ($LASTEXITCODE -ne 0) { throw "VSIX build failed with exit code $LASTEXITCODE." }
$artifactPath = Join-Path $PSScriptRoot "..\src\SeMaps.VisualStudio\bin\$Configuration\net472\SeMaps.VisualStudio.vsix"
if (!(Test-Path -LiteralPath $artifactPath)) { throw 'Build returned successfully but did not produce a VSIX.' }
Write-Output (Resolve-Path -LiteralPath $artifactPath).Path
