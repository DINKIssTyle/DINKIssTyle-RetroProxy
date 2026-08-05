@echo off
chcp 65001 > nul
echo ============================================
echo   DKST RetroProxy - Windows Build Script
echo ============================================
echo.

REM Check Go
echo [1/3] Checking Go installation...
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Go is not installed or not in PATH.
    echo Please install Go from: https://go.dev/dl/
    echo After installation, restart this script.
    pause
    exit /b 1
)
for /f "tokens=3" %%i in ('go version') do set GO_VERSION=%%i
echo        Found Go %GO_VERSION%

REM Check Node.js
echo [2/3] Checking Node.js installation...
where node >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Node.js is not installed or not in PATH.
    echo Please install Node.js from: https://nodejs.org/
    echo After installation, restart this script.
    pause
    exit /b 1
)
for /f "tokens=1" %%i in ('node --version') do set NODE_VERSION=%%i
echo        Found Node.js %NODE_VERSION%

REM Check Wails v3 CLI
echo [3/3] Checking Wails CLI installation...
where wails3 >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo        Wails CLI not found. Installing...
    go install github.com/wailsapp/wails/v3/cmd/wails3@latest
    if %ERRORLEVEL% NEQ 0 (
        echo [ERROR] Failed to install Wails CLI.
        pause
        exit /b 1
    )
    echo        Wails CLI installed successfully.
) else (
    for /f "tokens=1" %%i in ('wails3 version') do set WAILS_VERSION=%%i
    echo        Found Wails CLI
)

echo.
echo ============================================
echo   All dependencies satisfied. Building...
echo ============================================
echo.

REM Install frontend dependencies
echo Installing frontend dependencies...
cd frontend
call npm install
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to install frontend dependencies.
    cd ..
    pause
    exit /b 1
)
cd ..

REM Build the application
echo Building application...
wails3 build
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Build failed.
    pause
    exit /b 1
)

echo.
echo ============================================
echo   Build completed successfully!
echo   Output: bin\DKST RetroProxy.exe
echo ============================================
pause
