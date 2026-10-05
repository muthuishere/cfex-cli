---
name: cfex
description: Expose a local port on your own domain; list/stop/start/delete Cloudflare tunnels via `cfex`. Trigger: expose localhost:PORT on my domain, put a dev server on a custom domain, share a local app over HTTPS, webhook testing, list/stop/delete tunnels, what is on cloudflared.
---

# cfex: your local port on your own domain, through Cloudflare

## Instant (foreground, Ctrl+C stops it and cleans up)
```bash
cfex dev.yourdomain.com:3000      # https://dev.yourdomain.com -> localhost:3000
cfex api.yourdomain.com:8080
cfex webhook.yourdomain.com:4000
cfex api.yourdomain.com 8080      # same thing, two arguments
cfex secure.yourdomain.com:8443 --https [--verify-cert]   # HTTPS origin (auto-detected when omitted)
cfex dev.yourdomain.com:3000 --keep                       # make it permanent (same as `cfex add`)
```
Instant mode makes a throwaway cloudflared tunnel and a DNS route, and removes both on exit.

## Durable (survives reboots, many routes on one tunnel)
```bash
cfex add 3000 dev.yourdomain.com [--https] [--protected]   # launchd (macOS) / systemd user unit (Linux)
cfex list [--json] [--no-http]    # EVERYTHING cloudflared-ish on this machine: host, port, tunnel, class, running, HTTP status
cfex stop|start dev.yourdomain.com
cfex delete dev.yourdomain.com [--yes]   # asks y/N on a terminal; --yes for scripts; no terminal and no --yes = dry run
cfex status | doctor | troubleshoot
cfex import <script|tunnel>       # adopt an existing tunnel into the config without changing what it serves
cfex skill install | uninstall    # this skill
cfex uninstall                    # removes the binary and skill; never your tunnels unless you say so
```

## Setup
1. `cloudflared tunnel login` once (pick your domain). cfex drives cloudflared for tunnels and DNS routes.
2. Optional `CLOUDFLARE_API_TOKEN` (Zone:Read, DNS:Edit): adds DNS ownership checks, DNS deletion and remote ingress in `list`. `cfex doctor` says which features need it.
3. Config `~/.config/cfex/config.yaml` (default_zone, protected, client, token_cmd). State `~/.local/share/cfex/`.

## Safety rules
- cfex only stops/deletes what it created. Tunnels matching `protected:` / `client:` in the config, adopted and unmanaged tunnels are listed but refused.
- `delete` removes only the DNS record that points at the tunnel it deletes, and never overwrites an existing record.
- Never print or paste tokens; the API token comes from the environment or `token_cmd`, never from a flag or file.
- Never run `cloudflared` or `rm` on `~/.cloudflared` to "fix" things: it holds the login cert and tunnel credentials.
