[CmdletBinding()]
param(
    [string]$OutputDirectory,
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
$taskRepoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$taskSkillRoot = Join-Path $PSScriptRoot 'semaps-architecture'
if (-not $OutputDirectory) {
    $OutputDirectory = Join-Path $taskRepoRoot 'dist/skills'
}
$taskOutputRoot = [IO.Path]::GetFullPath($OutputDirectory)
$taskSkillText = [IO.File]::ReadAllText((Join-Path $taskSkillRoot 'SKILL.md'))
$taskFrontmatter = [regex]::Match($taskSkillText, '\A---\r?\n(?<fields>[\s\S]*?)\r?\n---(?:\r?\n|$)')
if (-not $taskFrontmatter.Success) { throw 'SKILL.md needs YAML frontmatter.' }
$taskFields = $taskFrontmatter.Groups['fields'].Value
$taskVersionMatch = [regex]::Match($taskFields, '(?m)^  version: "(?<version>[0-9]+\.[0-9]+\.[0-9]+)"\r?$')
if (-not $taskVersionMatch.Success -or
    $taskFields -notmatch '(?m)^name: semaps-architecture\r?$' -or
    $taskFields -notmatch '(?m)^license: MIT\r?$') {
    throw 'Expected semaps-architecture, MIT, and a quoted metadata.version in SKILL.md.'
}
$taskVersion = $taskVersionMatch.Groups['version'].Value

# Explicit inventory: do not ship the repository, listing drafts or local state.
$taskFiles = @(
    'LICENSE',
    'SKILL.md',
    'agents/openai.yaml',
    'references/code-to-diagram.md',
    'references/diagram-to-code.md',
    'references/setup.md'
)

function Get-BytesSha256([byte[]]$Bytes) {
    $taskHasher = [Security.Cryptography.SHA256]::Create()
    try {
        return ([BitConverter]::ToString($taskHasher.ComputeHash($Bytes))).Replace('-', '').ToLowerInvariant()
    } finally {
        $taskHasher.Dispose()
    }
}

$taskRecords = @()
foreach ($taskRelative in $taskFiles) {
    $taskPath = Join-Path $taskSkillRoot $taskRelative
    $taskBytes = [IO.File]::ReadAllBytes($taskPath)
    $taskRecords += [pscustomobject]@{
        path = $taskRelative
        bytes = $taskBytes.Length
        sha256 = Get-BytesSha256 $taskBytes
    }
}
$taskRootLicenseHash = (Get-FileHash -LiteralPath (Join-Path $taskRepoRoot 'LICENSE') -Algorithm SHA256).Hash.ToLowerInvariant()
if (($taskRecords | Where-Object path -EQ 'LICENSE').sha256 -ne $taskRootLicenseHash) {
    throw 'The packaged LICENSE must be byte-identical to the repository MIT LICENSE.'
}

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$taskArchives = @(
    [pscustomobject]@{ file = 'semaps-architecture-github.zip'; prefix = 'semaps-architecture/' },
    [pscustomobject]@{ file = 'semaps-architecture-agensi.zip'; prefix = '' }
)
$taskManifestPath = Join-Path $taskOutputRoot 'semaps-architecture-manifest.json'
$taskTargets = @($taskArchives | ForEach-Object { Join-Path $taskOutputRoot $_.file }) + @($taskManifestPath)
foreach ($taskTarget in $taskTargets) {
    if ((Test-Path -LiteralPath $taskTarget) -and -not $Force) {
        throw "Output already exists: $taskTarget. Choose another output directory or pass -Force."
    }
}
[IO.Directory]::CreateDirectory($taskOutputRoot) | Out-Null
$taskArchiveRecords = @()
foreach ($taskArchive in $taskArchives) {
    $taskArchivePath = Join-Path $taskOutputRoot $taskArchive.file
    $taskMode = if ($Force) { [IO.FileMode]::Create } else { [IO.FileMode]::CreateNew }
    $taskStream = [IO.File]::Open($taskArchivePath, $taskMode, [IO.FileAccess]::Write, [IO.FileShare]::None)
    $taskZip = [IO.Compression.ZipArchive]::new($taskStream, [IO.Compression.ZipArchiveMode]::Create, $false)
    try {
        foreach ($taskRecord in $taskRecords) {
            $taskEntry = $taskZip.CreateEntry($taskArchive.prefix + $taskRecord.path, [IO.Compression.CompressionLevel]::Optimal)
            $taskEntry.LastWriteTime = [DateTimeOffset]::new(2000, 1, 1, 0, 0, 0, [TimeSpan]::Zero)
            $taskEntryStream = $taskEntry.Open()
            try {
                $taskBytes = [IO.File]::ReadAllBytes((Join-Path $taskSkillRoot $taskRecord.path))
                if ((Get-BytesSha256 $taskBytes) -ne $taskRecord.sha256) {
                    throw "Source changed during packaging: $($taskRecord.path). Rebuild from a stable source."
                }
                $taskEntryStream.Write($taskBytes, 0, $taskBytes.Length)
            } finally {
                $taskEntryStream.Dispose()
            }
        }
    } finally {
        $taskZip.Dispose()
        $taskStream.Dispose()
    }

    $taskZip = [IO.Compression.ZipFile]::OpenRead($taskArchivePath)
    try {
        if ($taskZip.Entries.Count -ne $taskRecords.Count) { throw 'Unexpected archive entry count.' }
        foreach ($taskRecord in $taskRecords) {
            $taskEntry = $taskZip.GetEntry($taskArchive.prefix + $taskRecord.path)
            if ($null -eq $taskEntry -or $taskEntry.Length -ne $taskRecord.bytes) {
                throw "Missing or truncated ZIP entry: $($taskRecord.path)."
            }
            $taskEntryStream = $taskEntry.Open()
            $taskBuffer = [IO.MemoryStream]::new()
            try {
                $taskEntryStream.CopyTo($taskBuffer)
                if ((Get-BytesSha256 $taskBuffer.ToArray()) -ne $taskRecord.sha256) {
                    throw "ZIP hash mismatch: $($taskRecord.path)."
                }
            } finally {
                $taskEntryStream.Dispose()
                $taskBuffer.Dispose()
            }
        }
    } finally {
        $taskZip.Dispose()
    }
    $taskArchiveRecords += [pscustomobject]@{
        path = $taskArchive.file
        prefix = $taskArchive.prefix
        bytes = (Get-Item -LiteralPath $taskArchivePath).Length
        sha256 = (Get-FileHash -LiteralPath $taskArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    }
}
$taskManifest = [ordered]@{
    skill = 'semaps-architecture'
    version = $taskVersion
    license = 'MIT'
    files = $taskRecords
    archives = $taskArchiveRecords
}
$taskJson = ($taskManifest | ConvertTo-Json -Depth 6) + [Environment]::NewLine
$taskManifestMode = if ($Force) { [IO.FileMode]::Create } else { [IO.FileMode]::CreateNew }
$taskManifestStream = [IO.File]::Open($taskManifestPath, $taskManifestMode, [IO.FileAccess]::Write, [IO.FileShare]::None)
try {
    $taskJsonBytes = [Text.UTF8Encoding]::new($false).GetBytes($taskJson)
    $taskManifestStream.Write($taskJsonBytes, 0, $taskJsonBytes.Length)
} finally {
    $taskManifestStream.Dispose()
}
Write-Output "Packaged and verified $($taskRecords.Count) MIT skill files:"
$taskArchiveRecords | Format-Table path, bytes, sha256 -AutoSize
Write-Output "Manifest: $taskManifestPath"
