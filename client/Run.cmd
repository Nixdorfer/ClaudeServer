@echo off
chcp 65001&cls

cd /d "%~dp0"

set MODE=dev
set CLEAN=0

:parse_args
if "%~1"=="" goto done_args
if "%~1"=="-b" set MODE=build
if "%~1"=="-c" set CLEAN=1
if "%~1"=="-h" goto show_help
shift
goto parse_args
:done_args

if "%CLEAN%"=="1" goto do_clean
goto after_clean

:show_help
echo Usage: Run.cmd [options]
echo.
echo Options:
echo   -b    Build release version
echo   -c    Clean build artifacts before building
echo   -h    Show this help message
echo.
echo Examples:
echo   Run.cmd         Start development mode
echo   Run.cmd -b      Build release version
echo   Run.cmd -c -b   Clean and build release
goto end

:do_clean
echo [INFO] Cleaning build artifacts...
if exist "build" rd /s /q "build"
if exist "frontend\dist" rd /s /q "frontend\dist"
if exist "frontend\node_modules" rd /s /q "frontend\node_modules"
echo [INFO] Clean complete
if "%MODE%"=="dev" if "%CLEAN%"=="1" goto end

:after_clean
where wails >NUL 2>&1
if errorlevel 1 (
    echo [FATAL] Wails CLI not installed
    echo [INFO] Install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest
    pause
    exit /b 1
)

where go >NUL 2>&1
if errorlevel 1 (
    echo [FATAL] Go not installed
    pause
    exit /b 1
)

if not exist "frontend\node_modules" (
    echo [INFO] Installing frontend dependencies...
    cd frontend
    call npm install
    cd ..
)

if "%MODE%"=="build" goto do_build
goto do_dev

:do_dev
echo [INFO] Starting development server...
echo [INFO] Frontend: http://localhost:5173
echo [INFO] Backend API: http://localhost:34115
echo.
wails dev
goto end

:do_build
echo [INFO] Building release version...
wails build
if errorlevel 1 (
    echo [FATAL] Build failed
    pause
    goto end
)
echo.
echo [INFO] Build complete!
echo [INFO] Executable: build\bin\client.exe
echo.
pause
goto end

:end
