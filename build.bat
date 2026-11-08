@echo off
rem Builds "TLOU Mod Installer.exe" on Windows. Needs Go 1.24 or newer: https://go.dev/dl/
setlocal
cd /d "%~dp0"
echo [1/3] Downloading dependencies...
go mod download || goto :fail
echo [2/3] Embedding icon, manifest and version info...
rem rsrc_windows_amd64.syso is in the repo (built from tlou.rc). Only if it is missing,
rem fall back to rsrc, which embeds the icon and manifest but no version info.
if exist rsrc_windows_amd64.syso goto :build
if not exist icon.ico certutil -f -decode icon.ico.b64 icon.ico >nul || goto :fail
go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -ico icon.ico -arch amd64 -o rsrc_windows_amd64.syso || goto :fail
:build
echo [3/3] Building...
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags "-H windowsgui" -o "TLOU Mod Installer.exe" . || goto :fail
echo.
echo Done: "TLOU Mod Installer.exe"
certutil -hashfile "TLOU Mod Installer.exe" SHA256
exit /b 0
:fail
echo Build failed.
exit /b 1
