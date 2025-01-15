# cfex - Cloudflare Exposer CLI

A secure command-line tool for exposing local development environments to the internet using Cloudflare's tunneling technology. Instantly create HTTPS endpoints for your local services without port forwarding or static IPs.

## What is cfex?

cfex (Cloudflare Exposer) simplifies the process of creating secure tunnels between your local development environment and the internet. It leverages Cloudflare's tunnel technology to:

- Create instant HTTPS endpoints for local services
- Share development work with clients or team members
- Test webhooks on local applications
- Access local development servers from any device
- Bypass firewalls and NAT limitations securely

All connections are end-to-end encrypted and don't require opening ports or configuring router settings.


## Platform Support

This tool works on:
- Linux
- macOS
- Windows (only with WSL - Windows Subsystem for Linux)

### Windows Users Important Note
If you're on Windows, you must use WSL (Windows Subsystem for Linux) to run this tool.

To set up WSL:
1. Open PowerShell as Administrator and run:
   ```powershell
   wsl --install
   ```
2. Restart your computer
3. Follow the Linux installation instructions within your WSL environment

## Prerequisites

Before installing, ensure you have:
- Cloudflare account with a registered domain
- `cloudflared` installed ([Installation Guide](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation))
- `jq` installed (for JSON processing)
- Cloudflare API token with Zone permissions

## Installation

### Using curl (recommended)
```bash
curl -sSL https://raw.githubusercontent.com/muthuishere/cfex-cli/main/install.sh | bash
```

### Manual Installation
```bash
git clone https://github.com/muthuishere/cfex-cli.git
cd cfex-cli
chmod +x install.sh
./install.sh
```

## Setup

### 1. Create Cloudflare API Token

1. Visit [Cloudflare Dashboard](https://dash.cloudflare.com) → Profile → API Tokens
2. Click "Create Token"
3. Choose "Create Custom Token"
4. Configure permissions:
   - Zone - DNS - Edit
   - Zone - Zone - Read
5. Set Zone Resources:
   - Include - All Zones (or specific zones)
6. Name your token (e.g., "cfex CLI Token")
7. Create and copy the token

### 2. Configure Environment

Set your API token:

```bash
export CLOUDFLARE_API_KEY='your-api-token'
```

For permanent configuration:
```bash
echo 'export CLOUDFLARE_API_KEY="your-api-token"' >> ~/.bashrc  # or ~/.zshrc
source ~/.bashrc  # or source ~/.zshrc
```

## Usage

### Basic Commands

```bash
# Create tunnel (Format 1)
cfex <domain> <port>

# Create tunnel (Format 2)
cfex <domain>:<port>

# List tunnels
cfex list

# Delete tunnel
cfex delete <domain>

# Show help
cfex --help
```

### Examples

```bash
# Expose React dev server
cfex app.yourdomain.com 3000

# Share local API
cfex api.yourdomain.com:8080

# Test webhooks
cfex webhook.yourdomain.com 4000

# Delete multiple tunnels
cfex delete app.yourdomain.com api.yourdomain.com
```

### Features

- Automatic tunnel creation and configuration
- DNS record management
- Clean resource cleanup on exit (Ctrl+C)
- Support for multiple domains
- Easy listing and deletion of tunnels

## Troubleshooting

### Common Issues & Solutions

1. **Installation Issues**
   - Verify prerequisites are installed (`cloudflared`, `jq`)
   - Ensure proper permissions for installation directory

2. **Authentication Issues**
   - Verify `CLOUDFLARE_API_KEY` is correctly set
   - Confirm API token permissions
   - Run `cloudflared tunnel login` if needed

3. **DNS Issues**
   - Allow time for DNS propagation
   - Check domain ownership in Cloudflare
   - Verify zone permissions

4. **Connection Issues**
   - Ensure local service is running
   - Check port availability
   - Verify firewall settings
