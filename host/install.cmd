@echo off
rem Installs semaps.exe (editor embedded) into %GOPATH%\bin and associates *.semaps
rem with it for the current user: Enter / double click on a project file opens it.
cd /d "%~dp0"
go install . || exit /b 1
for /f "delims=" %%G in ('go env GOPATH') do set "SEMAPS_EXE=%%G\bin\semaps.exe"
reg add "HKCU\Software\Classes\.semaps" /ve /d "SeMaps.Project" /f >nul
reg add "HKCU\Software\Classes\SeMaps.Project" /ve /d "SeMaps project" /f >nul
reg add "HKCU\Software\Classes\SeMaps.Project\shell\open\command" /ve /d "\"%SEMAPS_EXE%\" \"%%1\"" /f >nul
echo Installed %SEMAPS_EXE%
echo *.semaps files now open with it.
