#!/bin/bash
# Installs cfex (v2) from the latest GitHub release.
#   curl -sSL https://github.com/muthuishere/cfex-cli/releases/latest/download/install.sh | bash
# Environment: CFEX_VERSION (e.g. v2.0.0), CFEX_INSTALL_DIR, CFEX_NO_SKILL=1 to skip the agent skill.
set -euo pipefail

REPO="muthuishere/cfex-cli"

check_dependency() {
    if ! command -v "$1" > /dev/null; then
        echo "Error: $1 is not installed. Please install it first." >&2
        exit 1
    fi
}
check_dependency curl
check_dependency tar
check_dependency cloudflared

case "$(uname -s)" in
    Darwin) OS=darwin ;;
    Linux)  OS=linux ;;
    *) echo "Error: unsupported OS $(uname -s). On Windows use WSL." >&2; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64|amd64)  ARCH=amd64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    *) echo "Error: unsupported CPU $(uname -m)" >&2; exit 1 ;;
esac

VERSION="${CFEX_VERSION:-}"
if [ -z "$VERSION" ]; then
    VERSION="$(curl -sSL "https://api.github.com/repos/$REPO/releases/latest" | grep -m1 '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
fi
[ -n "$VERSION" ] || { echo "Error: could not find the latest cfex release" >&2; exit 1; }
NUM="${VERSION#v}"
ASSET="cfex_${NUM}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$VERSION"

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
echo "Downloading cfex $VERSION ($OS/$ARCH)..."
curl -sSL -f "$BASE/$ASSET" -o "$TMP/$ASSET"
curl -sSL -f "$BASE/checksums.txt" -o "$TMP/checksums.txt"
( cd "$TMP" && grep " $ASSET\$" checksums.txt > check.txt \
    && { command -v sha256sum >/dev/null && sha256sum -c check.txt >/dev/null || shasum -a 256 -c check.txt >/dev/null; } ) \
    || { echo "Error: checksum verification failed" >&2; exit 1; }
tar -xzf "$TMP/$ASSET" -C "$TMP" cfex

INSTALL_DIR="${CFEX_INSTALL_DIR:-/usr/local/bin}"
if [ -w "$INSTALL_DIR" ] || [ "$(id -u)" -eq 0 ]; then SUDO=""
elif [ -z "${CFEX_INSTALL_DIR:-}" ] && command -v sudo > /dev/null; then SUDO="sudo"
else INSTALL_DIR="$HOME/.local/bin"; SUDO=""; mkdir -p "$INSTALL_DIR"; fi
$SUDO install -m 0755 "$TMP/cfex" "$INSTALL_DIR/cfex"
echo "Installed $INSTALL_DIR/cfex"
case ":$PATH:" in *":$INSTALL_DIR:"*) ;; *) echo "Note: add $INSTALL_DIR to your PATH" ;; esac

if [ "${CFEX_NO_SKILL:-}" != "1" ]; then "$INSTALL_DIR/cfex" skill install || true; fi
echo "Installation completed! Next: cloudflared tunnel login, then: cfex dev.yourdomain.com:3000"
