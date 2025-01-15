#!/bin/bash

# uninstall.sh

# Check for sudo privileges
if [ "$EUID" -ne 0 ]; then
    if command -v sudo > /dev/null; then
        SUDO="sudo"
    else
        echo "Error: This script requires root privileges. Please run with sudo or as root."
        exit 1
    fi
fi

INSTALL_DIR="/usr/local/bin"
SCRIPT_NAME="cfex"

# Remove the script
if [ -f "$INSTALL_DIR/$SCRIPT_NAME" ]; then
    echo "Removing $SCRIPT_NAME from $INSTALL_DIR..."
    $SUDO rm -f "$INSTALL_DIR/$SCRIPT_NAME"
    echo "$SCRIPT_NAME has been uninstalled successfully."
else
    echo "$SCRIPT_NAME is not installed in $INSTALL_DIR."
fi

# Optional: Clean up cloudflared configuration
read -p "Do you want to remove cloudflared configuration files? (y/N) " response
if [[ "$response" =~ ^[Yy]$ ]]; then
    if [ -d "$HOME/.cloudflared" ]; then
        echo "Removing cloudflared configuration files..."
        rm -rf "$HOME/.cloudflared"
        echo "Cloudflared configuration files have been removed."
    else
        echo "No cloudflared configuration files found."
    fi
fi

echo "Uninstallation completed!"
