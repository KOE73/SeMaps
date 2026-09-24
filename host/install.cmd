@echo off
rem Builds semaps.exe from source (editor embedded) and the extractors, and
rem installs them for the current user: %LOCALAPPDATA%\Programs\SeMaps on PATH,
rem *.semaps associated, extractors\ beside semaps.exe (ADR_20260924-3).
rem The editor bundle (app\) is not in git: whenever npm is present it is rebuilt
rem here, so a stale app\ never ends up in the exe.
rem Without Node, download a ready semaps.exe from the Releases page and run it.
rem An extractor whose SDK is missing (dotnet, npm) is skipped, not fatal.
cd /d "%~dp0"
where npm >nul 2>nul
if not errorlevel 1 (
  pushd ..\editor && call npm ci && call npm run build:app && popd || exit /b 1
) else if not exist "app\index.html" (
  echo No editor bundle in host\app and no npm to build it.
  echo Either install Node and rerun, or download semaps.exe from GitHub Releases.
  exit /b 1
)
set "STAGE=%TEMP%\semaps-build"
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%\extractors"
go build -o "%STAGE%\semaps.exe" . || exit /b 1

where dotnet >nul 2>nul
if not errorlevel 1 (
  dotnet publish ..\extractors\csharp\src\SeMaps.Extract.CSharp -c Release -o "%STAGE%\extractors\csharp" || exit /b 1
) else (
  echo No dotnet: the C# extractor is not installed.
)

where npm >nul 2>nul
if not errorlevel 1 (
  pushd ..\extractors\typescript && call npm ci && call npm run build && popd || exit /b 1
  xcopy /e /i /q ..\extractors\typescript\dist "%STAGE%\extractors\typescript\dist" >nul || exit /b 1
  del /s /q "%STAGE%\extractors\typescript\dist\test" >nul 2>nul
  rmdir /s /q "%STAGE%\extractors\typescript\dist\test" >nul 2>nul
  copy ..\extractors\typescript\package.json "%STAGE%\extractors\typescript\" >nul
  copy ..\extractors\typescript\package-lock.json "%STAGE%\extractors\typescript\" >nul
  pushd "%STAGE%\extractors\typescript" && call npm ci --omit=dev --ignore-scripts && popd || exit /b 1
) else (
  echo No npm: the TypeScript extractor is not installed.
)

"%STAGE%\semaps.exe" install || exit /b 1
rmdir /s /q "%STAGE%"
