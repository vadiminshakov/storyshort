#!/usr/bin/env pwsh

param(
    [string]$InstallDir = "$env:USERPROFILE\bin"
)

$ErrorActionPreference = "Stop"

$REPO = "vadiminshakov/storyshort"
$BINARY_NAME = "storyshort"

function Detect-Platform {
    $OS = "windows"
    $ARCH = $env:PROCESSOR_ARCHITECTURE.ToLower()
    
    switch ($ARCH) {
        "amd64" { $ARCH = "amd64" }
        "x86_64" { $ARCH = "amd64" }
        "arm64" { $ARCH = "arm64" }
        default {
            Write-Error "Unsupported architecture: $ARCH"
            exit 1
        }
    }
    
    Write-Host "Detected OS: $OS, Architecture: $ARCH"
    return @($OS, $ARCH)
}

function Get-LatestRelease {
    Write-Host "Getting latest release info..."
    $releaseUrl = "https://api.github.com/repos/$REPO/releases/latest"
    
    try {
        $releaseInfo = Invoke-RestMethod -Uri $releaseUrl
        $tagName = $releaseInfo.tag_name
        Write-Host "Latest release: $tagName"
        return $releaseInfo
    }
    catch {
        Write-Error "Failed to get release information: $_"
        exit 1
    }
}

function Download-And-Install {
    param($ReleaseInfo, $OS, $ARCH)
    
    $assetName = "${BINARY_NAME}_${OS}_${ARCH}.zip"
    Write-Host "Looking for asset: $assetName"
    
    $asset = $ReleaseInfo.assets | Where-Object { $_.name -eq $assetName }
    
    if (-not $asset) {
        Write-Error "Asset not found: $assetName"
        Write-Host "Available assets:"
        $ReleaseInfo.assets | ForEach-Object { Write-Host "  - $($_.name)" }
        exit 1
    }
    
    $downloadUrl = $asset.browser_download_url
    Write-Host "Downloading from: $downloadUrl"
    
    $tempDir = [System.IO.Path]::GetTempPath()
    $tempFile = Join-Path $tempDir $assetName
    $extractDir = Join-Path $tempDir "storyshort_extract"
    
    try {
        # Download
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tempFile
        
        # Extract
        if (Test-Path $extractDir) {
            Remove-Item $extractDir -Recurse -Force
        }
        Expand-Archive -Path $tempFile -DestinationPath $extractDir
        
        # Find binary
        $binaryPath = Join-Path $extractDir "$BINARY_NAME.exe"
        if (-not (Test-Path $binaryPath)) {
            Write-Error "Binary $BINARY_NAME.exe not found in archive"
            Get-ChildItem $extractDir
            exit 1
        }
        
        # Create install directory
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }
        
        # Install
        $finalPath = Join-Path $InstallDir "$BINARY_NAME.exe"
        Copy-Item $binaryPath $finalPath -Force
        
        Write-Host "Installation completed!"
        Write-Host "Binary installed to: $finalPath"
        
        # Add to PATH if not already there
        $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
        if ($currentPath -notlike "*$InstallDir*") {
            Write-Host "Adding $InstallDir to user PATH..."
            [Environment]::SetEnvironmentVariable("PATH", "$currentPath;$InstallDir", "User")
            Write-Host "Please restart your terminal or run: refreshenv"
        }
        
        Write-Host "Run '$BINARY_NAME' to start the application"
    }
    finally {
        # Cleanup
        if (Test-Path $tempFile) { Remove-Item $tempFile -Force }
        if (Test-Path $extractDir) { Remove-Item $extractDir -Recurse -Force }
    }
}

function Main {
    Write-Host "StoryShort Installer for Windows"
    Write-Host "================================"
    
    $OS, $ARCH = Detect-Platform
    $releaseInfo = Get-LatestRelease
    Download-And-Install -ReleaseInfo $releaseInfo -OS $OS -ARCH $ARCH
}

Main