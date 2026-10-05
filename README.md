# cfex - Cloudflare Exposer CLI
[![Discord](https://img.shields.io/badge/AgentNexus-join%20the%20community-5865F2?logo=discord&logoColor=white)](https://discord.gg/V9C2kvHC8D)

Put a local port on your own domain, over HTTPS, with one command. cfex drives [`cloudflared`](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) for the tunnels and DNS routes, and manages every tunnel on your machine.

> The original bash version is preserved as the [v1.0.0 release](https://github.com/muthuishere/cfex-cli/releases/tag/v1.0.0) (legacy). v2 is the maintained version.

## Instant: `cfex <domain>:<port>`

```bash
cfex dev.yourdomain.com:3000       # React/Vue/Angular dev server -> https://dev.yourdomain.com
cfex api.yourdomain.com:8080       # local API server
cfex webhook.yourdomain.com:4000   # webhook testing
cfex api.yourdomain.com 8080       # same thing, two arguments
cfex secure.yourdomain.com:8443 --https [--verify-cert]   # HTTPS origin (auto-detected when omitted)
```

It runs in the foreground. Press **Ctrl+C**: the tunnel and the DNS record it created are removed. Add `--keep` to make it permanent (same as `cfex add`).

## Durable: `cfex add`

```bash
cfex add 3000 dev.yourdomain.com      # survives reboots (launchd on macOS, systemd user unit on Linux)
cfex add 8080 api.yourdomain.com      # many routes share one cfex-owned tunnel
cfex list                             # EVERYTHING on this machine, with live HTTP status
cfex stop dev.yourdomain.com          # stop serving, keep config + DNS      (cfex start to resume)
cfex delete dev.yourdomain.com        # asks y/N on a terminal; --yes for scripts; no terminal and no --yes = dry run
cfex status | doctor | troubleshoot
cfex import <script|tunnel>           # adopt an existing tunnel into the config; serving is not changed
```

`add` appends ONE ingress rule to cfex's own tunnel (a pure, unit-tested merge that never touches other routes) and creates the CNAME with `cloudflared tunnel route dns`, which never overwrites an existing record. Changing a route restarts the connector, so routes on that tunnel reconnect for a second or two.

`list` shows managed routes plus everything else cloudflared-ish here: account tunnels with their ingress rules, launchd jobs, `~/.cloudflared` scripts, live HTTP status. Classes: `managed`, `instant`, `PROTECTED`, `client`, `adopted`, `unmanaged`. Put tunnels you never want touched under `protected:` / `client:` in the config; cfex only stops/deletes what it created.

## Install

```bash
curl -sSL https://github.com/muthuishere/cfex-cli/releases/latest/download/install.sh | bash
```

Downloads the release binary for your OS/CPU (macOS and Linux, amd64 and arm64), verifies its checksum, installs it to `/usr/local/bin` (or `~/.local/bin`) and installs the agent skill. Binaries and checksums are on the [releases page](https://github.com/muthuishere/cfex-cli/releases). From source: `go install github.com/muthuishere/cfex-cli/cmd/cfex@latest`.

Prerequisites: a Cloudflare account with a domain, and [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/) installed.

### Authenticate cloudflared (once)
```bash
cloudflared tunnel login
```
Opens Cloudflare's login page: log in, pick the PRIMARY domain you want to tunnel, authorize. This writes `~/.cloudflared/cert.pem`; afterwards you can tunnel any subdomain of that domain. If you forget, cfex starts this guided login for you.

### API token (optional)
cfex works with cloudflared alone. An API token adds: DNS ownership checks (`managed-by: cfex` stamped on the records it creates), deleting DNS records on `delete`/Ctrl+C cleanup, remote ingress in `list`, and zone validation. `cfex doctor` tells you which features are on.

Create one at <https://dash.cloudflare.com/profile/api-tokens>: *Create Token* → *Edit zone DNS* template → *Zone Resources: Include → Specific zone* → your domain. Then:
```bash
export CLOUDFLARE_API_TOKEN='your-api-token'     # CLOUDFLARE_API_KEY also works; add it to ~/.zshrc or ~/.bashrc to persist
```
Or let cfex fetch it from your secret manager: `token_cmd: "op run --"` in the config (cfex re-runs itself under that command; the token only exists in its environment).

## Agent skill

`cfex skill install` copies [`skills/cfex/SKILL.md`](skills/cfex/SKILL.md) to `~/.claude/skills/cfex` and `~/.agents/skills/cfex`, so coding agents know the commands and the safety rules (`cfex skill uninstall` removes it).

## Uninstall

```bash
cfex uninstall           # or ./uninstall.sh
```
Removes the binary and the skill, then asks (default: no) whether to stop the cfex service, remove cfex's config, and remove cloudflared's own files in `~/.cloudflared`. Flags for scripts: `--stop-service --purge-config --purge-cloudflared`. Tunnels and DNS records in your Cloudflare account are never deleted; run `cfex delete` first if you want them gone.

## Configuration

`~/.config/cfex/config.yaml` (every key optional; see [`config.example.yaml`](config.example.yaml)): `default_zone`, `tunnel_name`, `service_label`, `token_env`, `token_cmd`, `protected`, `client`. State and logs: `~/.local/share/cfex/`.

## Troubleshooting

1. **Installation**: verify `cloudflared` is installed (`cfex doctor`); check permissions on the install directory.
2. **Authentication**: re-run `cloudflared tunnel login`; if you use an API token, make sure it is exported and valid.
3. **DNS**: allow time for propagation; make sure the domain is in your Cloudflare account and the hostname has no existing record (cfex never overwrites one).
4. **Connection**: make sure the service is running on the port you gave (cfex warns when nothing listens); check port conflicts and firewalls.

## Platform support
- **Linux** and **macOS** (amd64, arm64). The durable service uses systemd (user) and launchd.
- **Windows** via WSL: `wsl --install` in an elevated PowerShell, restart, then follow the Linux steps.

## Development
`make build` · `make test` (the suite refuses to run unless `CFEX_CONFIG` points under the temp dir) · releases are built by GoReleaser from a `v2*` tag.

## Alternatives
- **ngrok**: subscription required for custom domains.
- **LocalTunnel**: free, but lacks DNS management.
- **cloudflared**: the native client that cfex simplifies.

## Community

Questions, ideas, or built something with this? Join **[AgentNexus](https://discord.gg/V9C2kvHC8D)**, a Discord for people building with AI agents and open tools. This project lives in **#cfex-cli**.
