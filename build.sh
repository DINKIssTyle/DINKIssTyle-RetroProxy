#!/bin/bash
set -e

echo "============================================"
echo "  DKST RetroProxy - Build Script"
echo "  (macOS / Ubuntu / Linux)"
echo "============================================"
echo ""

# Detect OS
OS="$(uname -s)"
case "${OS}" in
    Darwin*)    PLATFORM="macOS";;
    Linux*)     PLATFORM="Linux";;
    *)          PLATFORM="Unknown";;
esac
echo "Detected platform: $PLATFORM"
echo ""

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check Go
echo "[1/3] Checking Go installation..."
if ! command_exists go; then
    echo "       Go is not installed."
    if [ "$PLATFORM" = "macOS" ]; then
        echo "       Installing Go via Homebrew..."
        if ! command_exists brew; then
            echo "[ERROR] Homebrew not found. Please install it first:"
            echo "        /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
            exit 1
        fi
        brew install go
    elif [ "$PLATFORM" = "Linux" ]; then
        echo "       Installing Go..."
        if command_exists apt-get; then
            sudo apt-get update
            sudo apt-get install -y golang-go
        elif command_exists dnf; then
            sudo dnf install -y golang
        elif command_exists pacman; then
            sudo pacman -S --noconfirm go
        else
            echo "[ERROR] Could not detect package manager. Please install Go manually:"
            echo "        https://go.dev/dl/"
            exit 1
        fi
    fi
fi
GO_VERSION=$(go version | awk '{print $3}')
echo "       Found Go $GO_VERSION"

# Check Node.js
echo "[2/3] Checking Node.js installation..."
if ! command_exists node; then
    echo "       Node.js is not installed."
    if [ "$PLATFORM" = "macOS" ]; then
        echo "       Installing Node.js via Homebrew..."
        brew install node
    elif [ "$PLATFORM" = "Linux" ]; then
        echo "       Installing Node.js..."
        if command_exists apt-get; then
            curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
            sudo apt-get install -y nodejs
        elif command_exists dnf; then
            sudo dnf install -y nodejs npm
        elif command_exists pacman; then
            sudo pacman -S --noconfirm nodejs npm
        else
            echo "[ERROR] Could not detect package manager. Please install Node.js manually:"
            echo "        https://nodejs.org/"
            exit 1
        fi
    fi
fi
NODE_VERSION=$(node --version)
echo "       Found Node.js $NODE_VERSION"

# Check Wails v3 CLI
echo "[3/3] Checking Wails CLI installation..."
if ! command_exists wails3; then
    echo "       Wails CLI not found. Installing..."
    go install github.com/wailsapp/wails/v3/cmd/wails3@latest
    # Add Go bin to PATH for this session
    export PATH="$PATH:$(go env GOPATH)/bin"
    if ! command_exists wails3; then
        echo "[ERROR] Wails installed but not in PATH. Add this to your shell profile:"
        echo "        export PATH=\$PATH:\$(go env GOPATH)/bin"
        exit 1
    fi
    echo "       Wails CLI installed successfully."
else
    echo "       Found Wails CLI"
fi

# Check Wails dependencies (Linux only)
if [ "$PLATFORM" = "Linux" ]; then
    echo ""
    echo "Checking Linux dependencies for Wails (GTK4 & WebKitGTK 6.0)..."
    if command_exists apt-get; then
        sudo apt-get update
        sudo apt-get install -y libgtk-4-dev libwebkitgtk-6.0-dev build-essential pkg-config
    elif command_exists dnf; then
        sudo dnf install -y gtk4-devel webkitgtk6.0-devel gcc gcc-c++ make pkgconfig
    elif command_exists pacman; then
        sudo pacman -S --noconfirm gtk4 webkitgtk-6.0 base-devel pkgconf
    fi
fi

echo ""
echo "============================================"
echo "  All dependencies satisfied. Building..."
echo "============================================"
echo ""

# Install frontend dependencies
echo "Installing frontend dependencies..."
cd frontend
npm install
cd ..

# Build the application
echo "Building application..."
wails3 build

echo ""
echo "============================================"
echo "  Build completed successfully!"
if [ "$PLATFORM" = "macOS" ]; then
    echo "  Output: bin/DKST RetroProxy.app"
else
    echo "  Output: bin/DKST RetroProxy"
fi
echo "============================================"
