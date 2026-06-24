# Windows MSI Packaging

This directory contains the WiX source used to build the unsigned Candela
Windows installer.

## Build

From the repository root:

```powershell
$version = "0.1.0"
$target = "amd64" # or "arm64"
$wixArch = if ($target -eq "arm64") { "arm64" } else { "x64" }
$artifactSuffix = if ($target -eq "arm64") { "-arm64" } else { "" }
$dist = "dist\windows\$target"
New-Item -ItemType Directory -Force $dist | Out-Null
$env:GOOS = "windows"
$env:GOARCH = $target
go build -ldflags "-s -w -X main.version=$version" -o "$dist\candela.exe" .\cmd\candela
Copy-Item LICENSE "$dist\LICENSE.txt"
Copy-Item README.md "$dist\README.md"
dotnet tool install --global wix
wix build packaging\windows\candela.wxs `
  -d ProductVersion=$version `
  -d SourceDir="$PWD\$dist" `
  -arch $wixArch `
  -out "dist\windows\Candela-$version-windows$artifactSuffix.msi"
```

## Install

```powershell
msiexec /i Candela-0.1.0-windows.msi /qn /norestart
```

Set `INSTALLDESKTOPSHORTCUT=1` to add the optional desktop shortcut:

```powershell
msiexec /i Candela-0.1.0-windows.msi /qn /norestart INSTALLDESKTOPSHORTCUT=1
```

## Uninstall

```powershell
msiexec /x Candela-0.1.0-windows.msi /qn /norestart
```

The v1 installer is unsigned, so Windows SmartScreen may warn before install.
Build `target=arm64` for Windows on Arm. The MSI installs `candela.exe` under
`Program Files\Candela` and appends that directory to the system `PATH`.
