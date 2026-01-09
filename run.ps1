$ErrorActionPreference = "Continue"
$ROOT = $PSScriptRoot + "\"
$tauriConfig = Get-Content "${ROOT}client\src-tauri\tauri.conf.json" | ConvertFrom-Json
$VERSION = $tauriConfig.version
$CLIENT_FLAG = $args -contains "-c"
$MOBILE_FLAG = $args -contains "-m"
$BUILD_FLAG = $args -contains "-b"

function Check-Node {
    if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
        Write-Host "[Error] Node.js not found. Installing via winget..."
        winget install OpenJS.NodeJS.LTS -e --silent
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[Error] Failed to install Node.js. Please install manually."
            exit 1
        }
        Write-Host "[Info] Node.js installed. Please restart the terminal."
        exit 1
    }
}

function Check-Go {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Host "[Error] Go not found. Installing via winget..."
        winget install GoLang.Go -e --silent
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[Error] Failed to install Go. Please install manually."
            exit 1
        }
        Write-Host "[Info] Go installed. Please restart the terminal."
        exit 1
    }
}

function Check-Rust {
    if (-not (Get-Command cargo -ErrorAction SilentlyContinue)) {
        Write-Host "[Error] Rust not found. Installing via rustup..."
        Invoke-WebRequest -Uri "https://win.rustup.rs" -OutFile "$env:TEMP\rustup-init.exe"
        & "$env:TEMP\rustup-init.exe" -y
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[Error] Failed to install Rust. Please install manually from https://rustup.rs"
            exit 1
        }
        Write-Host "[Info] Rust installed. Please restart the terminal."
        exit 1
    }
}

function Check-Android {
    if (-not $env:ANDROID_HOME) {
        Write-Host "[Error] ANDROID_HOME not set."
        Write-Host "[Info] Please install Android Studio and set ANDROID_HOME environment variable."
        Write-Host "[Info] Download: https://developer.android.com/studio"
        exit 1
    }
    if (-not (Test-Path "$env:ANDROID_HOME\platform-tools\adb.exe")) {
        Write-Host "[Error] Android SDK platform-tools not found."
        Write-Host "[Info] Please install via Android Studio SDK Manager."
        exit 1
    }
}

function Compile-Notice {
    node "${ROOT}client\compile-notice.js"
}

function Start-Emulator {
    Write-Host "[Emulator] Killing existing emulator..."
    Stop-Process -Name "qemu-system-x86_64" -Force -ErrorAction SilentlyContinue
    Stop-Process -Name "netsimd" -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    Write-Host "[Emulator] Restarting ADB..."
    & "$env:ANDROID_HOME\platform-tools\adb.exe" kill-server 2>$null
    & "$env:ANDROID_HOME\platform-tools\adb.exe" start-server 2>$null
    Write-Host "[Emulator] Getting AVD list..."
    $avdList = & "$env:ANDROID_HOME\emulator\emulator.exe" -list-avds 2>$null
    $AVD_NAME = ($avdList | Select-Object -First 1)
    if (-not $AVD_NAME) {
        Write-Host "[Error] No AVD found. Please create one in Android Studio."
        exit 1
    }
    Write-Host "[Emulator] Starting $AVD_NAME..."
    Start-Process -FilePath "$env:ANDROID_HOME\emulator\emulator.exe" -ArgumentList "-avd", $AVD_NAME
    Write-Host "[Emulator] Waiting for device to be online..."
    while ($true) {
        Start-Sleep -Seconds 3
        $devices = & "$env:ANDROID_HOME\platform-tools\adb.exe" devices
        if ($devices -match "emulator.*device") {
            Write-Host "[Emulator] Device ready."
            Start-Sleep -Seconds 3
            break
        }
        Write-Host "[Emulator] Still waiting..."
    }
}

if (-not $CLIENT_FLAG) {
    Check-Go
    Write-Host "[Server] Starting..."
    Set-Location "${ROOT}server"
    go run .
    Set-Location $ROOT
    exit 0
}

if ($MOBILE_FLAG) {
    Check-Node
    Check-Android
    Compile-Notice
    if ($BUILD_FLAG) {
        Write-Host "[Mobile] Building release..."
        Write-Host "[Mobile] Stopping dev servers..."
        Stop-Process -Name "node" -Force -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 2
        Set-Location "${ROOT}client\src-vue\mobile"
        Remove-Item -Path "node_modules\.vite" -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -Path "dist" -Recurse -Force -ErrorAction SilentlyContinue
        npm install
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[Error] npm install failed"
            exit 1
        }
        npm run build
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[Error] Build failed"
            exit 1
        }
        npx cap sync
        Set-Location android
        Write-Host "[Mobile] Cleaning previous build..."
        Remove-Item -Path ".gradle" -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -Path "app\.cxx" -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -Path "app\build" -Recurse -Force -ErrorAction SilentlyContinue
        .\gradlew clean
        .\gradlew assembleRelease
        $TIMESTAMP = (Get-Date).ToString("HHmmss")
        Copy-Item "${ROOT}client\src-vue\mobile\android\app\build\outputs\apk\release\app-release.apk" "${ROOT}ClaudeClient-android-v${VERSION}-${TIMESTAMP}.apk"
        Write-Host "[Mobile] Output: ${ROOT}ClaudeClient-android-v${VERSION}-${TIMESTAMP}.apk"
        Set-Location $ROOT
    } else {
        Write-Host "[Mobile] Starting dev mode..."
        Set-Location "${ROOT}client\src-vue\mobile"
        Write-Host "[Mobile] Cleaning cache..."
        Remove-Item -Path "node_modules\.vite" -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -Path "dist" -Recurse -Force -ErrorAction SilentlyContinue
        npm install
        npm run build
        npx cap sync
        $viteJob = Start-Process -FilePath "cmd" -ArgumentList "/c", "npm run dev" -PassThru -WindowStyle Normal
        Start-Emulator
        Set-Location "${ROOT}client\src-vue\mobile"
        npx cap run android --target emulator-5554
        Stop-Process -Id $viteJob.Id -Force -ErrorAction SilentlyContinue
    }
    Set-Location $ROOT
    exit 0
}

Check-Node
Check-Rust
Compile-Notice
if ($BUILD_FLAG) {
    Write-Host "[PC] Building release..."
    Write-Host "[PC] Stopping dev servers..."
    Stop-Process -Name "node" -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    Set-Location "${ROOT}client\src-vue\pc"
    Remove-Item -Path "node_modules\.vite" -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -Path "dist" -Recurse -Force -ErrorAction SilentlyContinue
    npm install
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[Error] npm install failed"
        exit 1
    }
    npm run build
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[Error] Build failed"
        exit 1
    }
    Set-Location "${ROOT}client\src-tauri"
    cargo tauri build
    $TIMESTAMP = (Get-Date).ToString("HHmmss")
    Copy-Item "${ROOT}client\src-tauri\target\release\claude-client.exe" "${ROOT}ClaudeClient-win64-v${VERSION}-${TIMESTAMP}.exe"
    Write-Host "[PC] Output: ${ROOT}ClaudeClient-win64-v${VERSION}-${TIMESTAMP}.exe"
    Set-Location $ROOT
} else {
    Write-Host "[PC] Starting dev mode..."
    Set-Location "${ROOT}client\src-vue\pc"
    Write-Host "[PC] Cleaning cache..."
    Remove-Item -Path "node_modules\.vite" -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -Path "dist" -Recurse -Force -ErrorAction SilentlyContinue
    npm install
    $viteJob = Start-Process -FilePath "cmd" -ArgumentList "/c", "npm run dev" -PassThru -WindowStyle Normal
    Start-Sleep -Seconds 3
    Set-Location "${ROOT}client\src-tauri"
    cargo tauri dev
    Stop-Process -Id $viteJob.Id -Force -ErrorAction SilentlyContinue
    Set-Location $ROOT
}
