#!/bin/bash
set -e

echo "============================================"
echo "  DKST RetroProxy - macOS Build Script"
echo "============================================"
echo ""

# Verify OS is macOS
OS="$(uname -s)"
if [[ "${OS}" != Darwin* ]]; then
    echo "[WARNING] This script is intended for macOS (detected: ${OS})."
    echo "          Proceeding with build..."
    echo ""
fi

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# 1. Check Go
echo "[1/4] Checking Go installation..."
if ! command_exists go; then
    echo "       Go is not installed."
    echo "       Installing Go via Homebrew..."
    if ! command_exists brew; then
        echo "[ERROR] Homebrew not found. Please install Homebrew first:"
        echo "        /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
        exit 1
    fi
    brew install go
fi
GO_VERSION=$(go version | awk '{print $3}')
echo "       Found Go $GO_VERSION"

# 2. Check Node.js
echo "[2/4] Checking Node.js installation..."
if ! command_exists node; then
    echo "       Node.js is not installed."
    echo "       Installing Node.js via Homebrew..."
    if ! command_exists brew; then
        echo "[ERROR] Homebrew not found. Please install Homebrew first."
        exit 1
    fi
    brew install node
fi
NODE_VERSION=$(node --version)
echo "       Found Node.js $NODE_VERSION"

# 3. Check Wails v3 CLI
echo "[3/4] Checking Wails CLI installation..."
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

# 4. Install frontend dependencies and build
echo "[4/4] Installing frontend dependencies and building app..."
echo "       Installing npm packages..."
cd frontend
npm install
cd ..

echo "       Packaging RetroProxy for macOS (.app bundle with icons)..."
wails3 package

echo ""
echo "============================================"
echo "  macOS Build completed successfully!"
echo "  Output: bin/DKST RetroProxy.app"
echo "============================================"
