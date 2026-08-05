#!/bin/bash
set -e

echo "============================================"
echo "  DKST RetroProxy - Linux Build Script"
echo "============================================"
echo ""

# Verify OS is Linux
OS="$(uname -s)"
if [[ "${OS}" != Linux* ]]; then
    echo "[WARNING] This script is intended for Linux (detected: ${OS})."
    echo "          Proceeding with build..."
    echo ""
fi

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# 1. Check Go
echo "[1/5] Checking Go installation..."
if ! command_exists go; then
    echo "       Go is not installed. Installing Go via package manager..."
    if command_exists apt-get; then
        sudo apt-get update
        sudo apt-get install -y golang-go
    elif command_exists dnf; then
        sudo dnf install -y golang
    elif command_exists pacman; then
        sudo pacman -S --noconfirm go
    else
        echo "[ERROR] Could not detect package manager. Please install Go manually: https://go.dev/dl/"
        exit 1
    fi
fi
GO_VERSION=$(go version | awk '{print $3}')
echo "       Found Go $GO_VERSION"

# 2. Check Node.js
echo "[2/5] Checking Node.js installation..."
if ! command_exists node; then
    echo "       Node.js is not installed. Installing Node.js..."
    if command_exists apt-get; then
        curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
        sudo apt-get install -y nodejs
    elif command_exists dnf; then
        sudo dnf install -y nodejs npm
    elif command_exists pacman; then
        sudo pacman -S --noconfirm nodejs npm
    else
        echo "[ERROR] Could not detect package manager. Please install Node.js manually: https://nodejs.org/"
        exit 1
    fi
fi
NODE_VERSION=$(node --version)
echo "       Found Node.js $NODE_VERSION"

# 3. Check Linux GUI Development Dependencies
echo "[3/5] Checking Linux GTK/WebKit dependencies..."
if command_exists apt-get; then
    if apt-cache show libwebkit2gtk-4.1-dev >/dev/null 2>&1; then
        sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev build-essential
    else
        sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev build-essential
    fi
elif command_exists dnf; then
    sudo dnf install -y gtk3-devel webkit2gtk3-devel gcc gcc-c++ make
elif command_exists pacman; then
    sudo pacman -S --noconfirm gtk3 webkit2gtk base-devel
fi
echo "       Linux dependencies satisfied."

# 4. Check Wails v3 CLI
echo "[4/5] Checking Wails CLI installation..."
export PATH="$PATH:$(go env GOPATH)/bin"
if ! command_exists wails3; then
    echo "       Wails CLI not found. Installing github.com/wailsapp/wails/v3/cmd/wails3@latest ..."
    go install github.com/wailsapp/wails/v3/cmd/wails3@latest
    if ! command_exists wails3; then
        echo "[ERROR] Wails CLI installed but not found in PATH."
        echo "        Please ensure \$(go env GOPATH)/bin is in your PATH."
        exit 1
    fi
    echo "       Wails CLI installed successfully."
else
    echo "       Found Wails CLI"
fi

# 5. Install frontend dependencies and build
echo "[5/5] Installing frontend dependencies and building app..."
echo "       Installing npm packages..."
cd frontend
npm install
cd ..

echo "       Building RetroProxy for Linux..."
GOOS=linux wails3 build

echo ""
echo "============================================"
echo "  Linux Build completed successfully!"
echo "  Output: bin/RetroProxy"
echo "============================================"
