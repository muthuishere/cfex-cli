# cfex – Design Document

## Overview

**cfex** (Cloudflare Exposer CLI) is a single-file Bash CLI tool that wraps `cloudflared` and the Cloudflare REST API to let developers expose local services to the internet through their own domain names with a single command.

---

## Goals

| Goal | Description |
|------|-------------|
| Simplicity | One command to create a live HTTPS URL for any local port |
| Zero config | No YAML files to write by hand – cfex generates them |
| Custom domains | Use a real domain the developer already owns |
| Automated DNS | DNS records are created and deleted automatically |
| Cross-platform | Works on Linux, macOS, and Windows (via WSL) |

---

## Architecture

```
Developer Machine
│
├── cfex (bash script)               ← User-facing CLI
│     ├── validate_basic_requirements()
│     ├── create_or_run_tunnel()
│     ├── list_tunnels()
│     └── delete_tunnels()
│
├── cloudflared                      ← Cloudflare tunnel daemon
│     ├── tunnel login               ← Writes cert.pem
│     ├── tunnel create              ← Creates named tunnel
│     ├── tunnel route dns           ← Points subdomain to tunnel
│     └── tunnel run                 ← Runs the tunnel process
│
└── Cloudflare REST API              ← DNS record management
      └── zones/{id}/dns_records     ← Used for deletion
```

### Data Flow

```
cfex dev.example.com:3000
        │
        ▼
  Validate (cert.pem, API key, jq)
        │
        ▼
  Detect local protocol (HTTP/HTTPS via openssl / curl)
        │
        ▼
  Create tunnel if not exists
  Generate ~/.cloudflared/<name>_config.yml
        │
        ▼
  Create DNS CNAME record  ←── Cloudflare API
        │
        ▼
  cloudflared tunnel run   ←── keeps process alive
        │
        ▼
  Traffic: internet → Cloudflare edge → tunnel → localhost:3000
```

---

## Components

### 1. `install.sh` – Automated Setup Script

Handles the complete first-run experience:

1. **Dependency check** – verifies `curl`, `jq`, and `cloudflared` are present and provides install hints.
2. **Binary install** – downloads `bin/cfex` to `/usr/local/bin/cfex`.
3. **Cloudflare authentication** – runs `cloudflared tunnel login` if `~/.cloudflared/cert.pem` is missing.
4. **API token setup** – prompts the user for a Cloudflare API token and persists it to the appropriate shell profile (`~/.bashrc`, `~/.zshrc`, or `~/.config/fish/config.fish`).

### 2. `bin/cfex` – Main CLI

A self-contained Bash script (~650 lines). Entry point dispatches to sub-functions:

| Sub-function | Trigger | Purpose |
|---|---|---|
| `validate_basic_requirements` | Always (except help/version) | Check cert, API key, jq |
| `create_or_run_tunnel` | Default / `<domain>:<port>` | Full tunnel lifecycle |
| `list_tunnels` | `list` | Show active tunnels |
| `delete_tunnels` | `delete <domain…>` | Remove tunnels + DNS |
| `show_help` | `--help` / `-h` | Print usage |

#### Protocol Auto-Detection

Before creating a config file, cfex probes the local port:

```
openssl s_client -connect localhost:<PORT>  →  HTTPS detected
curl -sk https://localhost:<PORT>           →  HTTPS fallback
(default)                                  →  HTTP
```

The generated `_config.yml` sets `noTLSVerify: true` for self-signed certs.

#### Configuration File

Generated at `~/.cloudflared/<domain>_config.yml`:

```yaml
url: https://localhost:<PORT>        # or http://
tunnel: <TUNNEL_ID>
credentials-file: ~/.cloudflared/<TUNNEL_ID>.json
originRequest:
  noTLSVerify: true
  disableChunkedEncoding: true
ingress:
  - hostname: <FULL_DOMAIN>
    service: https://localhost:<PORT>
  - service: http_status:404
```

### 3. `uninstall.sh` – Removal Script

- Removes `/usr/local/bin/cfex`.
- Optionally removes `~/.cloudflared` (tunnels, certs, configs).

---

## Authentication & Secrets

| Secret | Where stored | How set |
|--------|-------------|---------|
| Cloudflare tunnel cert | `~/.cloudflared/cert.pem` | `cloudflared tunnel login` |
| Tunnel credentials | `~/.cloudflared/<id>.json` | Created by `cloudflared tunnel create` |
| Cloudflare API token | Environment variable `CLOUDFLARE_API_KEY` | Set by user; persisted to shell profile by `install.sh` |

**Security notes:**
- The API token is only held in the environment and the user's own shell profile – it is never written inside `~/.cloudflared` or any cfex-owned file.
- Tunnel credentials (`*.json`) are created by `cloudflared` and managed by it.

---

## Command Reference

```
cfex <domain>:<port>                 Expose local port via HTTPS tunnel
cfex <domain>:<port> --https         Force HTTPS origin
cfex <domain>:<port> --https --verify-cert  Verify origin TLS cert
cfex list                            List active tunnels
cfex delete <domain> [domain…]       Delete tunnel(s) and DNS records
cfex --help | -h                     Show help
cfex --version | -v                  Show version
```

---

## Installation Flow

```
curl -sSL .../install.sh | bash
        │
        ├─ check: curl, jq, cloudflared present?
        ├─ download: /usr/local/bin/cfex
        ├─ authenticate: cloudflared tunnel login
        └─ configure: CLOUDFLARE_API_KEY → shell profile
```

---

## Platform Support

| Platform | Status | Notes |
|----------|--------|-------|
| Linux | ✅ Supported | Primary target |
| macOS | ✅ Supported | `brew install jq cloudflared` |
| Windows (WSL) | ✅ Supported | Follow Linux steps inside WSL |

---

## Dependencies

| Tool | Purpose | Install |
|------|---------|---------|
| `cloudflared` | Tunnel daemon and auth | [Cloudflare docs](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation/) |
| `jq` | JSON parsing | `brew install jq` / `apt install jq` |
| `curl` | HTTP requests to Cloudflare API | Pre-installed on most systems |
| `openssl` | Local HTTPS probe | Pre-installed on most systems |

---

## Future Improvements

- **Persistent API key storage** – store in a dedicated config file (`~/.cfex/config`) rather than the shell profile, with stricter file permissions.
- **Multiple domain auth** – support authenticating multiple domains with separate cert files.
- **Status command** – show tunnel health, connected clients, and traffic stats.
- **Auto-restart** – systemd/launchd service integration for long-lived tunnels.
- **Windows native support** – PowerShell install script and native Windows binary path.
- **Token validation** – verify the API token has the correct permissions during `install.sh`.
