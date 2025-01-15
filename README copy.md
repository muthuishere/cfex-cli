# cfex - Cloudflare Exposer CLI

Expose your local services to the internet using your own domain names through Cloudflare's tunnel technology. Create instant HTTPS endpoints for any local service without port forwarding or static IPs.

## Why cfex?

When developing web applications, you often need to:
- Share your local development server with clients
- Test webhook integrations
- Access your local services from mobile devices
- Demo features from your development environment

cfex lets you do all this using your own domain names:
```bash
# Share your React app
cfex dev.yourdomain.com:3000    # https://dev.yourdomain.com

# Expose your API
cfex api.yourdomain.com:8080    # https://api.yourdomain.com

# Test webhooks
cfex webhook.yourdomain.com:4000 # https://webhook.yourdomain.com
```

All endpoints are:
- Secured with HTTPS
- Using your own domain names
- Work behind firewalls/NAT
- Zero configuration needed

## Quick Start

```bash
# 1. Install prerequisites
cloudflared tunnel login
jq --version || sudo apt-get install jq  # or brew install jq for macOS

# 2. Get API token from Cloudflare dashboard
# Use 'Edit zone DNS' template for the domain you authenticated with

# 3. Set API token (add to ~/.bashrc or ~/.zshrc for persistence)
# Important: Keep your API token secure and never share it
export CLOUDFLARE_API_KEY='your-api-token'

# 4. Install cfex
curl -sSL https://raw.githubusercontent.com/muthuishere/cfex-cli/main/install.sh | bash

# 5. Create your first tunnel
# Make sure your service is running on the specified port
# before creating the tunnel
cfex yourdomain.com:3000
```

## Common Usage Examples

### Development Server
```bash
# React/Vue/Angular dev server
cfex dev.yourdomain.com:3000

# Access from any device at:
# https://dev.yourdomain.com
```

### API Development
```bash
# Local API server
cfex api.yourdomain.com:8080

# Test from anywhere using:
# https://api.yourdomain.com
```

### Multiple Services
```bash
# Frontend
cfex app.yourdomain.com:3000

# Backend
cfex api.yourdomain.com:8080

# Webhook testing
cfex webhook.yourdomain.com:4000
```


### Additional Commands
```bash


# List tunnels
cfex list

# Delete tunnel(s)
cfex delete domain.com

# Show help
cfex --help
```

## FAQ and Detailed Guide

<details>
<summary>What is cfex?</summary>

cfex (Cloudflare Exposer) simplifies creating secure tunnels between your local environment and the internet. It leverages Cloudflare's tunnel technology to:
- Create instant HTTPS endpoints for local services
- Share development work with clients
- Test webhooks locally
- Access local servers from anywhere
- Bypass firewalls securely
</details>

<details>
<summary>Platform Support</summary>

- Linux
- macOS
- Windows (via WSL only)

For Windows users:
1. Open PowerShell as Administrator: `wsl --install`
2. Restart computer
3. Follow Linux installation steps in WSL
</details>

<details>
<summary>Detailed Setup Guide</summary>

### 1. Authenticate with Cloudflare
```bash
cloudflared tunnel login
```
This will:
- Open Cloudflare's login page
- Ask for domain selection
- Create certificate at ~/.cloudflared/cert.pem

### 2. Create API Token
1. Visit [Cloudflare Dashboard](https://dash.cloudflare.com/profile/api-tokens)
2. Create Token → "Edit zone DNS" template
3. Select specific zone (same as authenticated domain)
4. Copy and set token:
```bash
export CLOUDFLARE_API_KEY='your-api-token'
```

### 3. Install cfex
```bash
curl -sSL https://raw.githubusercontent.com/muthuishere/cfex-cli/main/install.sh | bash
```
</details>

<details>
<summary>Troubleshooting</summary>

1. **Installation Issues**
   - Verify `cloudflared` and `jq` installation
   - Check directory permissions

2. **Authentication Issues**
   - Verify API token is set and valid
   - Confirm zone permissions
   - Re-run `cloudflared tunnel login` if needed

3. **DNS Issues**
   - Allow time for DNS propagation
   - Verify domain ownership
   - Check zone permissions

4. **Connection Issues**
   - Verify local service is running
   - Check port availability
   - Check firewall settings
</details>
```
