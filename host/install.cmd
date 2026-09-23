@echo off
rem Builds semaps.exe from source (editor embedded) and installs it for the
rem current user: %LOCALAPPDATA%\Programs\SeMaps on PATH, *.semaps associated.
rem The editor bundle (app\) is not in git: it is built here when npm is present.
rem Without Node, download a ready semaps.exe from the Releases page and run it.
cd /d "%~dp0"
if not exist "app\index.html" (
  where npm >nul 2>nul || (
    echo No editor bundle in host\app and no npm to build it.
    echo Either install Node and rerun, or download semaps.exe from GitHub Releases.
    exit /b 1
  )
  pushd ..\editor && call npm ci && call npm run build:app && popd || exit /b 1
)
set "OUT=%TEMP%\semaps-build\semaps.exe"
go build -o "%OUT%" . || exit /b 1
"%OUT%" install || exit /b 1
del "%OUT%"
