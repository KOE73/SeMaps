@echo off
rem Builds semaps.exe from source (editor embedded) and the extractors, and
rem installs them for the current user: %LOCALAPPDATA%\Programs\SeMaps on PATH,
rem *.semaps associated, extractors\ beside semaps.exe (ADR_20260924-3).
rem The editor bundle (host\app\) is not in git: whenever npm is present it is
rem rebuilt here, so a stale bundle never ends up in the exe.
rem Without Node, download a ready archive from the Releases page instead.
rem An extractor whose SDK is missing (dotnet, npm) is skipped, not fatal.
rem Works from anywhere: it runs in the folder it lies in, the repository root.
cd /d "%~dp0"
where npm >nul 2>nul
if not errorlevel 1 (
  pushd editor && call npm ci && call npm run build:app && popd || exit /b 1
) else if not exist "host\app\index.html" (
  echo No editor bundle in host\app and no npm to build it.
  echo Either install Node and rerun, or download semaps from GitHub Releases.
  exit /b 1
)
set "STAGE=%TEMP%\semaps-build"
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%\extractors"
go build -o "%STAGE%\semaps.exe" ./host || exit /b 1

where dotnet >nul 2>nul
if not errorlevel 1 (
  dotnet publish extractors\csharp\src\SeMaps.Extract.CSharp -c Release -o "%STAGE%\extractors\csharp" || exit /b 1
) else (
  echo No dotnet: the C# extractor is not installed.
)

where npm >nul 2>nul
if not errorlevel 1 (
  pushd extractors\typescript && call npm ci && call npm run build && popd || exit /b 1
  xcopy /e /i /q extractors\typescript\dist "%STAGE%\extractors\typescript\dist" >nul || exit /b 1
  rmdir /s /q "%STAGE%\extractors\typescript\dist\test" >nul 2>nul
  copy extractors\typescript\package.json "%STAGE%\extractors\typescript\" >nul
  copy extractors\typescript\package-lock.json "%STAGE%\extractors\typescript\" >nul
  pushd "%STAGE%\extractors\typescript" && call npm ci --omit=dev --ignore-scripts && popd || exit /b 1
) else (
  echo No npm: the TypeScript extractor is not installed.
)

rem Whatever still runs holds the files install replaces: every semaps.exe
rem (editors, MCP servers of agent sessions) and every exe started from the
rem install folder (extractors and their build hosts) is killed, children too.
taskkill /f /t /im semaps.exe >nul 2>nul
powershell -NoProfile -Command "$d = Join-Path $env:LOCALAPPDATA 'Programs\SeMaps'; Get-Process | Where-Object { $_.Path -and $_.Path.StartsWith($d, [StringComparison]::OrdinalIgnoreCase) } | Stop-Process -Force -ErrorAction SilentlyContinue"

"%STAGE%\semaps.exe" install || exit /b 1
rmdir /s /q "%STAGE%"
