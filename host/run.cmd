@echo off
rem Runs the host from source. With no arguments opens the bundled example workspace.
rem For everyday use install it once (install.cmd in the repository root) and just type `semaps` inside a project.
cd /d "%~dp0"
if not exist "app\index.html" (
  echo No editor bundle in host\app. Build it first: cd ..\editor ^&^& npm run build:app
  exit /b 1
)
title SeMaps
if "%~1"=="" (
  go run . ..\examples\example.semaps
) else (
  go run . %*
)
pause
