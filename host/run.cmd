@echo off
rem Usage: run.cmd [--workspace <dir>] [--source-root <dir>] [--port <n>]
rem With no arguments opens the bundled example workspace.
cd /d "%~dp0"
title SeMaps
if "%~1"=="" (
  go run . --workspace ..\examples\workspace --source-root ..
) else (
  go run . %*
)
pause
