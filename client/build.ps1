# PowerShell build script for client
# Version is automatically read from wails.json via go:embed
# Notice is automatically read from notice.md via go:embed

Write-Host "Building client..." -ForegroundColor Cyan

# Read version from wails.json for display
$wailsConfig = Get-Content -Path "wails.json" | ConvertFrom-Json
$version = $wailsConfig.version

Write-Host "Version: $version (embedded from wails.json)" -ForegroundColor Yellow

# Check if notice.md has content
if ((Test-Path "notice.md") -and ((Get-Content -Path "notice.md" -Raw -ErrorAction SilentlyContinue).Trim())) {
    Write-Host "Notice: notice.md will be embedded" -ForegroundColor Yellow
}

# Build with wails
wails build

if ($LASTEXITCODE -eq 0) {
    Write-Host "Build completed successfully!" -ForegroundColor Green
    Write-Host "Version $version has been embedded into the executable." -ForegroundColor Green
} else {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}
