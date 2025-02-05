#!/bin/bash

# Determine OS
OS="$(uname)"
SCRIPT_URL="https://raw.githubusercontent.com/muthuishere/cfex-cli/main/bin/cfex"
INSTALL_DIR="/usr/local/bin"

# Check for sudo privileges
if [ "$EUID" -ne 0 ]; then
    if command -v sudo > /dev/null; then
        SUDO="sudo"
    else
        echo "Error: This script requires root privileges. Please run with sudo or as root."
        exit 1
    fi
fi

# Check dependencies
check_dependency() {
    if ! command -v "$1" > /dev/null; then
        echo "Error: $1 is not installed. Please install it first."
        exit 1
    fi
}

check_dependency "curl"
check_dependency "jq"
check_dependency "cloudflared"

# Download and install the script
echo "Downloading cfex script..."
$SUDO curl -sSL "$SCRIPT_URL" -o "$INSTALL_DIR/cfex"
$SUDO chmod +x "$INSTALL_DIR/cfex"

echo "Installation completed! You can now use 'cfex' command."
