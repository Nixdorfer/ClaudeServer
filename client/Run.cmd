@echo off
chcp 65001&cls

cd /d "%~dp0"

set MODE=dev
set DEBUG=0
set CLEAN=0
set MOBILE=0

:parse_args
if "%~1"=="" goto done_args
if "%~1"=="-b" set MODE=build
if "%~1"=="-d" set DEBUG=1
if "%~1"=="-m" set MOBILE=1
if "%~1"=="-c" set CLEAN=1
if "%~1"=="-h" goto show_help
shift
goto parse_args
:done_args

if "%CLEAN%"=="1" goto do_clean
goto after_clean

:show_help
echo 用法: Run.cmd [选项]
echo.
echo 选项:
echo   (无)  PC端: 启动开发模式 (默认)
echo   -b    PC端: 构建优化发布版本
echo   -m    移动端: 启动 Android Studio 调试 (默认)
echo   -m -b 移动端: 构建优化发布版 APK
echo   -c    构建前清理构建产物
echo   -h    显示此帮助信息
echo.
echo 示例:
echo   Run.cmd           PC端开发模式 (cargo tauri dev)
echo   Run.cmd -b        PC端构建优化发布版本
echo   Run.cmd -m        移动端 Android Studio 调试
echo   Run.cmd -m -b     移动端构建发布版 APK
echo   Run.cmd -c -b     清理后构建发布版本
goto end

:do_clean
echo [信息] 正在清理构建产物...
if exist "src-tauri\target" rd /s /q "src-tauri\target"
if exist "src-vue\pc\dist" rd /s /q "src-vue\pc\dist"
if exist "src-vue\pc\node_modules" rd /s /q "src-vue\pc\node_modules"
if exist "src-vue\mobile\dist" rd /s /q "src-vue\mobile\dist"
if exist "src-vue\mobile\node_modules" rd /s /q "src-vue\mobile\node_modules"
echo [信息] 清理完成
if "%MODE%"=="dev" if "%CLEAN%"=="1" goto end

:after_clean
if "%MOBILE%"=="1" goto check_mobile
goto check_pc

:check_pc
where cargo >NUL 2>&1
if errorlevel 1 (
    echo [错误] 未安装 Rust
    echo [信息] 请从此处安装: https://rustup.rs/
    pause
    exit /b 1
)
if not exist "src-vue\pc\node_modules" (
    echo [信息] 正在安装PC端前端依赖...
    cd src-vue\pc
    call npm install
    cd ..\..
)
if "%MODE%"=="build" goto do_build
goto do_dev

:check_mobile
if not exist "src-vue\mobile\node_modules" (
    echo [信息] 正在安装移动端前端依赖...
    cd src-vue\mobile
    call npm install
    cd ..\..
)
if "%MODE%"=="build" goto do_mobile_build
goto do_mobile_dev

:do_dev
echo ========================================
echo 正在启动PC端开发服务器...
echo ========================================
echo [信息] 前端地址: http://localhost:5173
echo.
echo [信息] 正在启动前端开发服务器...
cd src-vue\pc
start /b cmd /c "npm run dev"
cd ..\..
timeout /t 3 /nobreak >nul
echo [信息] 正在启动 Tauri...
cd src-tauri
cargo tauri dev
cd ..
goto end

:do_build
echo ========================================
echo 正在构建PC端优化发布版本...
echo ========================================
echo.
echo [信息] 正在构建前端...
cd src-vue\pc
call npm run build
if errorlevel 1 (
    echo [错误] 前端构建失败
    cd ..\..
    pause
    goto end
)
cd ..\..
echo [信息] 正在构建 Tauri 应用...
cd src-tauri
cargo tauri build
if errorlevel 1 (
    cd ..
    echo [错误] 构建失败
    pause
    goto end
)
cd ..
echo.
echo [信息] 构建完成!
echo [信息] 可执行文件: src-tauri\target\release\Claude Chat.exe
echo.
pause
goto end

:do_mobile_dev
echo ========================================
echo 正在启动移动端 Android 模拟器调试...
echo ========================================
echo.
cd src-vue\mobile
echo [信息] 正在构建前端...
call npm run build
if errorlevel 1 (
    echo [错误] 前端构建失败
    cd ..\..
    pause
    goto end
)
echo [信息] 正在同步到 Android 项目...
call npx cap sync android
echo [信息] 正在启动 Android 模拟器...
echo [提示] 使用 chrome://inspect 调试 WebView
echo.
call npx cap run android
cd ..\..
goto end

:do_mobile_build
echo ========================================
echo 正在构建移动端发布版 APK...
echo ========================================
echo.
cd src-vue\mobile
echo [信息] 正在构建前端...
call npm run build
if errorlevel 1 (
    echo [错误] 前端构建失败
    cd ..\..
    pause
    goto end
)
echo [信息] 正在同步到 Android 项目...
call npx cap sync android
echo [信息] 正在构建 Release APK...
cd android
call gradlew assembleRelease
if errorlevel 1 (
    echo [错误] APK 构建失败
    cd ..\..\..
    pause
    goto end
)
cd ..\..\..
echo.
echo [信息] APK 构建完成!
echo [信息] APK 位置: src-vue\mobile\android\app\build\outputs\apk\release\
echo.
pause
goto end

:end
