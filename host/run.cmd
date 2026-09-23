@echo off
rem Runs the host from source. With no arguments opens the bundled example workspace.
rem For everyday use install it once (install.cmd) and just type `semaps` inside a project.
cd /d "%~dp0"
title SeMaps
if "%~1"=="" (
  go run . ..\examples\example.semaps
) else (
  go run . %*
)
pause
