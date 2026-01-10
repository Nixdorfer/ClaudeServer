$ErrorActionPreference = "Continue"
$ROOT = $PSScriptRoot + "\"
trap {
    Set-Location $ROOT
    break
}
$INFO_JSON = Get-Content "${ROOT}info.json" -Raw -Encoding UTF8 | ConvertFrom-Json
$VERSION = $INFO_JSON[0].version
$CLIENT_FLAG = $args -contains "-c"
$MOBILE_FLAG = $args -contains "-m"
$BUILD_FLAG = $args -contains "-b"
$DEV_FLAG = $args -contains "-d"

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

function Check-Wails {
    if (-not (Get-Command wails3 -ErrorAction SilentlyContinue)) {
        Write-Host "[Info] Wails CLI not found. Installing..."
        go install github.com/wailsapp/wails/v3/cmd/wails3@latest
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[Error] Failed to install Wails CLI."
            exit 1
        }
        Write-Host "[Info] Wails CLI installed."
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

try {
    if (-not $CLIENT_FLAG) {
        Check-Go
        $frpProcess = $null
        $frpPath = "${ROOT}frp.lnk"
        if (Test-Path $frpPath) {
            Write-Host "[FRP] Starting frp..."
            $frpProcess = Start-Process $frpPath -PassThru
            Start-Sleep -Seconds 2
        }
        if ($DEV_FLAG) {
            Write-Host "[Server] Starting dev mode..."
        } else {
            Write-Host "[Server] Starting release mode..."
        }
        Set-Location "${ROOT}server"
        go run .
        if ($frpProcess) {
            Write-Host "[FRP] Stopping frp..."
            Stop-Process -Id $frpProcess.Id -Force -ErrorAction SilentlyContinue
        }
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
        exit 0
    }
    Check-Node
    Check-Go
    Check-Wails
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
        Set-Location "${ROOT}client\src-wails"
        wails3 task windows:build
        $TIMESTAMP = (Get-Date).ToString("HHmmss")
        Copy-Item "${ROOT}client\src-wails\bin\ClaudeChat.exe" "${ROOT}ClaudeClient-win64-v${VERSION}-${TIMESTAMP}.exe"
        Write-Host "[PC] Output: ${ROOT}ClaudeClient-win64-v${VERSION}-${TIMESTAMP}.exe"
    } else {
        Write-Host "[PC] Starting dev mode..."
        Set-Location "${ROOT}client\src-wails"
        $env:DEV_MODE = "true"
        wails3 dev -config ./build/config.yml
        Stop-Process -Name "node" -Force -ErrorAction SilentlyContinue
    }
} finally {
    Set-Location $ROOT
}
