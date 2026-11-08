#!/bin/sh
# Cross-builds "TLOU Mod Installer.exe" from Linux/macOS. Needs Go 1.24 or newer.
set -e
cd "$(dirname "$0")"
go mod download
# rsrc_windows_amd64.syso is in the repo (icon, manifest and version info, built from tlou.rc).
# To rebuild it after changing tlou.rc:  llvm-windres --target=pe-x86-64 tlou.rc -O coff -o rsrc_windows_amd64.syso
# Only if it is missing, fall back to rsrc (icon and manifest, no version info).
if [ ! -f rsrc_windows_amd64.syso ]; then
  [ -f icon.ico ] || base64 -d icon.ico.b64 > icon.ico
  go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -ico icon.ico -arch amd64 -o rsrc_windows_amd64.syso
fi
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui" -o "TLOU Mod Installer.exe" .
sha256sum "TLOU Mod Installer.exe"
