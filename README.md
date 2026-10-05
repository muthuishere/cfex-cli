# cfex - Cloudflare Exposer CLI
[![Discord](https://img.shields.io/badge/AgentNexus-join%20the%20community-5865F2?logo=discord&logoColor=white)](https://discord.gg/V9C2kvHC8D)

> **v2 (Go)** adds a durable mode and a machine-wide view on top of everything v1 does. All v1 commands
> (`cfex host:3000`, `cfex host 3000`, `--https`, `--verify-cert`, `cfex list`, `cfex delete host`) keep working.
> The v1 bash script is still in `bin/cfex` until the Go binary is released.

```bash
cfex dev.example.com:3000          # v1: foreground tunnel, Ctrl+C stops it
cfex add 3000 dev.example.com      # v2: durable route on cfex's own tunnel, survives reboots (launchd / systemd)
cfex list                          # EVERYTHING on this machine, with live HTTP status
cfex stop dev.example.com          # stop serving, keep config + DNS      (cfex start to resume)
cfex delete dev.example.com        # dry run; --yes to apply
cfex status | doctor | import <script|tunnel>
```

### What v2 adds
- **`add`**: one named tunnel owned by cfex; each route adds ONE ingress rule (a pure, unit-tested merge that never touches other
  rules) and a proxied CNAME stamped `managed-by: cfex`. The connector runs under launchd (macOS) or a systemd user unit (Linux).
- **`list`**: managed routes plus everything else cloudflared-ish on the machine: account tunnels with their ingress rules, launchd
  jobs, `~/.cloudflared` scripts, live HTTP status. Classes: `managed`, `PROTECTED`, `client`, `adopted`, `legacy` (v1), `unmanaged`.
- **Safety**: only routes cfex created can be stopped/deleted. `delete` is a dry run unless `--yes` and refuses any DNS record without
  the `managed-by: cfex` comment. List tunnels you never want touched under `protected:` / `client:` in the config.
- **Secrets**: the API token is read from `CLOUDFLARE_API_TOKEN` only (or `token_cmd` runs cfex under your secret manager); it is never
  printed or written to disk. The connector token is a 0600 file, or lives in your secret manager (`connector_run_cmd`).
- `import` adopts an existing tunnel into the config without changing what it serves. `doctor` checks cloudflared, the token, the
  service, DNS <-> ingress consistency and lists plaintext token files by name.

Config: `~/.config/cfex/config.yaml` (see `config.example.yaml`). Token permissions: Zone:Read, DNS:Edit, Account: Cloudflare Tunnel:Edit.
Build: `make build` · test: `make test` (the suite refuses to run unless `CFEX_CONFIG` points under the temp dir).

Behaviour differences from v1: replacing an orphaned DNS record now only happens when it is a tunnel CNAME (v1 deleted any record
of that name); `cfex list` without an API token falls back to the v1 view.

---

Expose your local services to the internet using your domain names via Cloudflare's tunneling technology. With cfex, you can create instant HTTPS endpoints for any local service without port forwarding or static IPs.

## What is cfex?

cfex (Cloudflare Exposer CLI) simplifies creating secure tunnels between your local development environment and the internet. It leverages Cloudflare’s tunnel technology to:
- Expose local services via HTTPS endpoints.
- Use custom domain names for development and testing.
- Access your services behind firewalls or NAT.
- Test webhooks and APIs locally without additional setup.

---

## Why cfex?

When developing web applications, cfex is useful for:
- **Sharing local development servers**: Share your work with clients or team members.
- **Webhook testing**: Test external integrations directly from your local environment.
- **Accessing local services on mobile**: Use your app on other devices.
- **Demoing features**: Quickly share links for feedback without deploying.

With cfex, you can:
```bash
# Share your React app
cfex dev.yourdomain.com:3000    # https://dev.yourdomain.com

# Expose your API
cfex api.yourdomain.com:8080    # https://api.yourdomain.com

# Test webhooks
cfex webhook.yourdomain.com:4000 # https://webhook.yourdomain.com
```

---

## Quick Start

### Prerequisites
1. **Cloudflare Account with Registered Domain**: [Sign up for free](https://dash.cloudflare.com/).
2. **cloudflared Installed**: [Install cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation/).
3. **jq Installed**:
   - macOS: `brew install jq`
   - Ubuntu/Debian: `sudo apt install jq`
4. **Cloudflare API Token**: [Create API Token](https://dash.cloudflare.com/profile/api-tokens) with:
   - Permissions: `Zone-DNS Edit` and `Zone-Read`
   - Zone: Include the domain you want to use.

### Installation
```bash
# Install cfex
curl -sSL https://raw.githubusercontent.com/muthuishere/cfex-cli/main/install.sh | bash
```

### Authenticate Cloudflared
```bash
cloudflared tunnel login
```
This command:
- Opens Cloudflare's login page.
- Creates a certificate at `~/.cloudflared/cert.pem`.

### Set API Token
```bash
export CLOUDFLARE_API_KEY='your-api-token'
```
For persistence:
```bash
echo 'export CLOUDFLARE_API_KEY="your-api-token"' >> ~/.bashrc  # or ~/.zshrc
source ~/.bashrc  # or ~/.zshrc
```

---

## Usage

### Expose Services
```bash
# React/Vue/Angular dev server
cfex dev.yourdomain.com:3000

# Local API server
cfex api.yourdomain.com:8080

# Webhook testing
cfex webhook.yourdomain.com:4000
```

### Manage Tunnels
```bash
# List tunnels
cfex list

# Delete tunnel(s)
cfex delete domain.com

# Show help
cfex --help
```

---

## Troubleshooting

1. **Installation Issues**:
   - Verify `cloudflared` and `jq` are installed.
   - Check permissions for installation directories.

2. **Authentication Issues**:
   - Ensure the API token is set and valid.
   - Re-run `cloudflared tunnel login` if needed.

3. **DNS Issues**:
   - Allow time for DNS propagation.
   - Verify domain ownership and permissions.

4. **Connection Issues**:
   - Ensure the service is running on the specified port.
   - Check for port conflicts or firewall settings.

---

## Platform Support
- **Linux**
- **macOS**
- **Windows** (via WSL):
  - Install WSL: `wsl --install` in PowerShell (Admin).
  - Restart your computer.
  - Follow Linux setup instructions.

---

## Alternatives
- **ngrok**: Subscription required for custom domains.
- **LocalTunnel**: Free, but lacks features like DNS management.
- **cloudflared**: The native client that cfex simplifies.

cfex provides a streamlined solution for exposing your local services using Cloudflare, making it ideal for developers working with custom domains and secure tunnels.

## Community

Questions, ideas, or built something with this? Join **[AgentNexus](https://discord.gg/V9C2kvHC8D)** — a Discord
for people building with AI agents and open tools. This project lives in **#cfex-cli**.
