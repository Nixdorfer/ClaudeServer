@echo off
REM Build script for client
REM Version is automatically read from wails.json via go:embed
REM Notice is automatically read from notice.md via go:embed

echo Building client...
echo Version and notice will be embedded automatically from wails.json and notice.md

wails build

echo Build completed!
