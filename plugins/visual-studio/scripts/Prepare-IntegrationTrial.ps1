#Requires -Version 7.0
param([string]$HostExecutable)
$ErrorActionPreference = 'Stop'
$repository = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '../../..')).Path
$trialDirectory = Join-Path $repository ('plugins/visual-studio/bin/ide-trials/' + [guid]::NewGuid().ToString('N'))
$projectDirectory = Join-Path $trialDirectory 'workspace/projects/ide_trial'
New-Item -ItemType Directory -Path (Join-Path $projectDirectory 'views') -Force | Out-Null
function Write-TrialJson([string]$Path, $Value) { $Value | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $Path -Encoding utf8NoBOM }
$entities = @(
    @{ id = 'e_host'; kind = 'class'; origin = 'authored'; code = @(@{ lang = 'csharp'; symbol = 'SeMaps.VisualStudio.HostConnection'; ref = 'plugins/visual-studio/src/SeMaps.VisualStudio/HostConnection.cs#L12' }) },
    @{ id = 'e_picker'; kind = 'class'; origin = 'authored'; code = @(@{ lang = 'csharp'; symbol = 'SeMaps.VisualStudio.ProjectPicker'; ref = 'plugins/visual-studio/src/SeMaps.VisualStudio/ProjectPicker.cs#L8' }) },
    @{ id = 'e_resolver'; kind = 'class'; origin = 'authored'; code = @(@{ lang = 'csharp'; symbol = 'SeMaps.VisualStudio.SymbolResolver'; ref = 'plugins/visual-studio/src/SeMaps.VisualStudio/SymbolResolver.cs#L25' }) }
)
Write-TrialJson (Join-Path $projectDirectory 'project.json') @{ id = 'ide_trial'; title = 'IDE bridge trial'; contractVersion = 5; languages = @('ru'); defaultView = 'v_main'; defaultAxis = 'axis_subsystem' }
Write-TrialJson (Join-Path $projectDirectory 'entities.json') @{ entities = $entities }
Write-TrialJson (Join-Path $projectDirectory 'relations.json') @{ relations = @() }
$entries = @{}
foreach ($entity in $entities) { $entries[$entity.id] = @{ name = @{ v = $entity.id; origin = 'authored'; at = '2026-10-07T00:00:00Z' } } }
Write-TrialJson (Join-Path $projectDirectory 'text.ru.json') @{ contractVersion = 5; language = 'ru'; entries = $entries }
Write-TrialJson (Join-Path $projectDirectory 'views/v_main.view.json') @{ id = 'v_main'; project = 'ide_trial'; axis = 'axis_subsystem'; placements = @(@{ entity = 'e_host'; parent = $null; x = 100; y = 100; width = 240; height = 80 }); edges = @() }
$portProbe = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
$portProbe.Start()
$trialPort = $portProbe.LocalEndpoint.Port
$portProbe.Stop()
$sourceRoot = [System.IO.Path]::GetRelativePath($trialDirectory, $repository).Replace('\', '/')
@"
version: 1
name: SeMaps IDE integration trial
workspace: workspace
source_root: '$sourceRoot'
port: $trialPort
"@ | Set-Content -LiteralPath (Join-Path $trialDirectory 'trial.semaps') -Encoding utf8NoBOM
$solution = Get-Content -LiteralPath (Join-Path $repository 'plugins/visual-studio/SeMaps.VisualStudio.sln') -Raw
$solution = $solution.Replace('"src\SeMaps.VisualStudio\SeMaps.VisualStudio.csproj"', '"' + (Join-Path $repository 'plugins/visual-studio/src/SeMaps.VisualStudio/SeMaps.VisualStudio.csproj') + '"')
$solution = $solution.Replace('"tests\SeMaps.VisualStudio.Tests\SeMaps.VisualStudio.Tests.csproj"', '"' + (Join-Path $repository 'plugins/visual-studio/tests/SeMaps.VisualStudio.Tests/SeMaps.VisualStudio.Tests.csproj') + '"')
$solution | Set-Content -LiteralPath (Join-Path $trialDirectory 'IntegrationTrial.sln') -Encoding utf8NoBOM
Push-Location $repository
try {
    & npm run build:app --prefix editor | Out-Host
    if ($LASTEXITCODE -ne 0) { throw 'Editor build failed.' }
    if ($HostExecutable) {
        Copy-Item -LiteralPath (Resolve-Path -LiteralPath $HostExecutable).Path -Destination (Join-Path $trialDirectory 'semaps.exe')
    } else {
        & go build -o (Join-Path $trialDirectory 'semaps.exe') ./host | Out-Host
        if ($LASTEXITCODE -ne 0) { throw 'Trial host build failed.' }
    }
} finally { Pop-Location }
[pscustomobject]@{ Directory = $trialDirectory; Solution = Join-Path $trialDirectory 'IntegrationTrial.sln'; HostExecutable = Join-Path $trialDirectory 'semaps.exe'; Port = $trialPort }
