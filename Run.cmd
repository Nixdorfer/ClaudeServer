@echo off
chcp 65001 >nul
title Claude API转接服务
setlocal enabledelayedexpansion

:: 记录frp是否启动
set FRP_STARTED=0

:: 检查frp.lnk是否存在，存在则先启动（后台静默运行）
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

:: 程序退出后（包括Ctrl+C），关闭frp进程
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
