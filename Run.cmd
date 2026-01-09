@echo off
chcp 65001 >nul
title Claude API转接服务
setlocal enabledelayedexpansion

cd /d "%~dp0"

set MODE=server
set DEBUG=0

:parse_args
if "%~1"=="" goto done_args
if "%~1"=="-b" set MODE=build
if "%~1"=="-d" (
    set MODE=build
    set DEBUG=1
)
shift
goto parse_args
:done_args

if "%MODE%"=="build" goto client_build
goto server_run

:server_run
set FRP_STARTED=0

if exist "%~dp0frp.lnk" (
    echo 正在启动 FRP...
    powershell -Command "Start-Process '%~dp0frp.lnk' -WindowStyle Hidden"
    set FRP_STARTED=1
    echo FRP 已启动（后台运行）
    echo.
)

cd /d "%~dp0bin"

if not exist "go.mod" (
    echo 正在初始化环境...
    go mod tidy
    go mod download
    echo.
)

echo 正在启动Claude API服务器...
echo 按 Ctrl+C 可安全退出
echo.
cmd /c "go run ."

if %FRP_STARTED%==1 (
    echo.
    echo 正在关闭 FRP...
    taskkill /f /im frpc.exe >nul 2>&1
    taskkill /f /im frps.exe >nul 2>&1
    echo FRP 已关闭
)

echo.
echo 服务已停止
pause
goto end

:client_build
cd /d "%~dp0client"

where wails >NUL 2>&1
if errorlevel 1 (
    echo [FATAL] Wails not installed
    echo Please run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
    pause
    goto end
)

if not exist "frontend\node_modules" (
    echo [INFO] Installing frontend dependencies...
    cd frontend
    call npm install
    cd ..
    echo.
)

if "%DEBUG%"=="1" goto build_debug
goto build_release

:build_release
echo ========================================
echo Building optimized release version...
echo ========================================
echo.

echo [1/3] Building frontend with production optimizations...
cd frontend
set NODE_ENV=production
call npm run build:prod
if errorlevel 1 (
    echo [FATAL] Frontend build failed
    pause
    goto end
)
cd ..

echo.
echo [2/3] Building Go binary with optimizations...
echo   - Stripping debug symbols (-s)
echo   - Stripping DWARF info (-w)
echo   - Trimming file paths (-trimpath)

wails build -clean -ldflags "-s -w" -trimpath
if errorlevel 1 (
    echo [FATAL] Wails build failed
    pause
    goto end
)

echo.
echo [3/3] Build complete!
echo.
echo Output: client\build\bin\client.exe
for %%F in ("build\bin\client.exe") do (
    set /a "SIZE=%%~zF/1024"
    echo Size: !SIZE! KB
)
echo.
pause
goto end

:build_debug
echo ========================================
echo Building debug and release versions...
echo ========================================
echo.

echo [1/4] Building frontend...
cd frontend
call npm run build
if errorlevel 1 (
    echo [FATAL] Frontend build failed
    pause
    goto end
)
cd ..

echo.
echo [2/4] Building release version (client.exe)...
wails build -clean -ldflags "-s -w" -trimpath
if errorlevel 1 (
    echo [FATAL] Release build failed
    pause
    goto end
)

echo.
echo [3/4] Building debug version (client-dev.exe)...
wails build -debug -ldflags "-X main.isDebugBuild=true" -o client-dev.exe
if errorlevel 1 (
    echo [FATAL] Debug build failed
    pause
    goto end
)

echo.
echo [4/4] Build complete!
echo   - client.exe (release)
echo   - client-dev.exe (debug, with devtools)
echo.

echo Starting client-dev.exe with developer tools...
start "" "build\bin\client-dev.exe"
goto end

:end
endlocal
