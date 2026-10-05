#!/bin/bash
# Uninstalls cfex. Runs `cfex uninstall`, which removes the binary and agent skill and asks before touching anything else.
# Your tunnels and DNS records in Cloudflare are never deleted.
set -euo pipefail
if command -v cfex > /dev/null; then
    exec cfex uninstall "$@"
fi
for d in /usr/local/bin "$HOME/.local/bin"; do
    if [ -f "$d/cfex" ]; then
        echo "Removing $d/cfex..."
        if [ -w "$d" ]; then rm -f "$d/cfex"; else sudo rm -f "$d/cfex"; fi
        echo "cfex has been uninstalled successfully."
        exit 0
    fi
done
echo "cfex is not installed."
