#!/bin/bash

set -e

# Determine OS
OS="$(uname)"
SCRIPT_URL="https://raw.githubusercontent.com/muthuishere/cfex-cli/main/bin/cfex"
INSTALL_DIR="/usr/local/bin"

echo "========================================"
echo "  cfex - Cloudflare Exposer CLI Setup   "
echo "========================================"
echo ""

# ---------------------------------------------------------------------------
# 1. Sudo / privilege check
# ---------------------------------------------------------------------------
SUDO=""
if [ "$EUID" -ne 0 ]; then
    if command -v sudo > /dev/null 2>&1; then
        SUDO="sudo"
    else
        echo "Error: This script requires root privileges. Please run with sudo or as root."
        exit 1
    fi
fi

# ---------------------------------------------------------------------------
# 2. Dependency checks
# ---------------------------------------------------------------------------
check_dependency() {
    if ! command -v "$1" > /dev/null 2>&1; then
        echo "Error: '$1' is not installed. Please install it first."
        case "$1" in
            jq)
                echo "  macOS:          brew install jq"
                echo "  Ubuntu/Debian:  sudo apt install jq"
                ;;
            cloudflared)
                echo "  See: https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation/"
                ;;
        esac
        exit 1
    fi
}

echo "Checking dependencies..."
check_dependency "curl"
check_dependency "jq"
check_dependency "cloudflared"
echo "All dependencies found."
echo ""

# ---------------------------------------------------------------------------
# 3. Download and install the cfex binary
# ---------------------------------------------------------------------------
echo "Downloading cfex..."
$SUDO curl -sSL "$SCRIPT_URL" -o "$INSTALL_DIR/cfex"
$SUDO chmod +x "$INSTALL_DIR/cfex"
echo "cfex installed to $INSTALL_DIR/cfex"
echo ""

# ---------------------------------------------------------------------------
# 4. Cloudflare tunnel authentication
# ---------------------------------------------------------------------------
CLOUDFLARED_CERT="$HOME/.cloudflared/cert.pem"

if [ -f "$CLOUDFLARED_CERT" ]; then
    echo "cloudflared is already authenticated (cert.pem found)."
else
    echo "Authenticating with Cloudflare..."
    echo ""
    echo "A browser window will open. Please:"
    echo "  1. Log in to your Cloudflare account."
    echo "  2. Select the PRIMARY domain you want to use for tunneling."
    echo "  3. Click 'Authorize'."
    echo ""
    echo "Starting authentication in 3 seconds..."
    sleep 3
    if cloudflared tunnel login; then
        echo ""
        echo "Authentication successful!"
    else
        echo ""
        echo "Authentication failed. You can re-authenticate later by running:"
        echo "  cloudflared tunnel login"
        echo ""
    fi
fi
echo ""

# ---------------------------------------------------------------------------
# 5. Cloudflare API token setup
# ---------------------------------------------------------------------------
detect_shell_profile() {
    local shell_name
    shell_name="$(basename "${SHELL:-}")"
    case "$shell_name" in
        zsh)  echo "$HOME/.zshrc" ;;
        fish) echo "$HOME/.config/fish/config.fish" ;;
        *)    echo "$HOME/.bashrc" ;;
    esac
}

SHELL_PROFILE="$(detect_shell_profile)"

if [ -n "$CLOUDFLARE_API_KEY" ]; then
    echo "CLOUDFLARE_API_KEY is already set in the current environment."
    read -r -p "Do you want to persist it to $SHELL_PROFILE? [y/N] " persist_existing
    if echo "$persist_existing" | grep -qi "^y"; then
        if ! grep -q "CLOUDFLARE_API_KEY" "$SHELL_PROFILE" 2>/dev/null; then
            echo "" >> "$SHELL_PROFILE"
            echo "# Added by cfex install.sh" >> "$SHELL_PROFILE"
            echo "export CLOUDFLARE_API_KEY=\"$CLOUDFLARE_API_KEY\"" >> "$SHELL_PROFILE"
            echo "API key persisted to $SHELL_PROFILE"
        else
            echo "CLOUDFLARE_API_KEY entry already exists in $SHELL_PROFILE — skipping."
        fi
    fi
else
    echo "Cloudflare API Token Setup"
    echo "--------------------------"
    echo "Create an API token at: https://dash.cloudflare.com/profile/api-tokens"
    echo "  1. Click 'Create Token'."
    echo "  2. Use the 'Edit zone DNS' template."
    echo "  3. Under 'Zone Resources', select the domain you just authenticated."
    echo "  4. Click 'Continue to summary', then 'Create Token'."
    echo ""
    read -r -s -p "Paste your Cloudflare API token (leave blank to skip): " api_token
    echo ""
    if [ -n "$api_token" ]; then
        export CLOUDFLARE_API_KEY="$api_token"
        if ! grep -q "CLOUDFLARE_API_KEY" "$SHELL_PROFILE" 2>/dev/null; then
            echo "" >> "$SHELL_PROFILE"
            echo "# Added by cfex install.sh" >> "$SHELL_PROFILE"
            echo "export CLOUDFLARE_API_KEY=\"$api_token\"" >> "$SHELL_PROFILE"
            echo "API key saved to $SHELL_PROFILE"
            echo "Run 'source $SHELL_PROFILE' or open a new terminal to apply."
        else
            echo "CLOUDFLARE_API_KEY entry already exists in $SHELL_PROFILE — skipping."
            echo "Update it manually if you want to change the token."
        fi
    else
        echo "Skipped. Set it later with:"
        echo "  export CLOUDFLARE_API_KEY='your-api-token'"
        echo "  echo 'export CLOUDFLARE_API_KEY=\"your-api-token\"' >> $SHELL_PROFILE"
    fi
fi

echo ""
echo "========================================"
echo "  Setup complete!"
echo ""
echo "  Quick start:"
echo "    cfex dev.yourdomain.com:3000"
echo ""
echo "  More info:  cfex --help"
echo "========================================"
