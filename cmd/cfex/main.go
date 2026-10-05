// cfex: expose a local port on your own domain through Cloudflare tunnels, and see/stop/start/delete
// every tunnel on the machine.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/muthuishere/cfex-cli/internal/cfg"
)

var version = "2.0.0" // set by goreleaser

const usage = `cfex - Cloudflare Exposer CLI: put a local port on your own domain, and manage every tunnel on this machine.

Usage:
  cfex <domain>:<port>                       Instant: https://<domain> -> localhost:<port>, Ctrl+C stops and cleans up
  cfex <domain> <port>                       Same, two-argument syntax
  cfex <domain>:<port> --https               Force HTTPS to the local service (auto-detected when omitted)
  cfex <domain>:<port> --https --verify-cert HTTPS with certificate verification
  cfex <domain>:<port> --keep                Make it permanent (same as: cfex add <port> <domain>)
  cfex add <port> <domain> [--https] [--verify-cert] [--protected]
                                             Durable route on cfex's own tunnel (launchd / systemd keeps it running)
  cfex list [--json] [--no-http]             Everything on this machine, with live HTTP status
  cfex stop|start <domain|tunnel>            Stop / resume serving (keeps config and DNS)
  cfex delete <domain>... [--yes]            Delete tunnels (asks y/N on a terminal; --yes for scripts)
  cfex status | doctor | troubleshoot        Health, prerequisites and what needs an API token, common problems
  cfex import <script|tunnel> [--dry-run]    Adopt an existing tunnel into config without changing what it serves
  cfex skill install|uninstall|path          Agent skill (~/.claude/skills/cfex, ~/.agents/skills/cfex)
  cfex uninstall                             Remove cfex (never your tunnels unless you ask)

Options:
  --https         Force HTTPS mode for the local service
  --verify-cert   Verify the local service's SSL certificate (default: ignore certificate errors)
  --keep          Keep the tunnel instead of removing it on exit
  -y, --yes       Do not ask for confirmation
  -h, --help      Show this help
  -v, --version   Show version

Examples:
  cfex example.com:8080                      Auto-detect whether port 8080 speaks HTTP or HTTPS
  cfex sub.example.com:3000 --https --verify-cert
  cfex api.example.com 8080
  cfex list
  cfex delete example.com

Note: certificate errors from a local HTTPS service are ignored by default for easier development; use --verify-cert for strict checking.
Setup: run 'cloudflared tunnel login' once. An API token (CLOUDFLARE_API_TOKEN) is optional; 'cfex doctor' says what it enables.
Config: ~/.config/cfex/config.yaml · state: ~/.local/share/cfex/`

const troubleshoot = `Troubleshooting
1. Installation: verify cloudflared is installed (cfex doctor). Check permissions on the install directory (/usr/local/bin or ~/.local/bin).
2. Authentication: run 'cloudflared tunnel login' again. If you use an API token, make sure it is exported and valid (cfex doctor).
3. DNS: allow time for propagation. Verify the domain is in your Cloudflare account and that the hostname has no existing record (cfex never overwrites one).
4. Connection: make sure the service is running on the port you gave (cfex warns when nothing listens) and check port conflicts or firewalls.
5. Windows: use WSL (wsl --install in an elevated PowerShell, restart, then follow the Linux steps).`

func fatal(msg string) { fmt.Fprintln(os.Stderr, "cfex:", msg); os.Exit(1) }

// withToken re-executes cfex under the configured token_cmd (e.g. "op run --") when no token is in the environment,
// so the token value only ever exists in the child's environment.
func withToken(c *cfg.Config) {
	if token(c) != "" || os.Getenv("CFEX_WRAPPED") == "1" || strings.TrimSpace(c.TokenCmd) == "" {
		return
	}
	prefix := strings.Fields(c.TokenCmd)
	bin, err := exec.LookPath(prefix[0])
	if err != nil {
		fatal("token_cmd: " + err.Error())
	}
	self, _ := os.Executable()
	args := append(append(prefix, self), os.Args[1:]...)
	env := append(os.Environ(), "CFEX_WRAPPED=1")
	fatal(syscall.Exec(bin, args, env).Error())
}

// token returns the API token from the environment only.
func token(c *cfg.Config) string {
	for _, n := range []string{c.TokenEnv, "CLOUDFLARE_API_TOKEN", "CLOUDFLARE_API_KEY"} {
		if n != "" && os.Getenv(n) != "" {
			return os.Getenv(n)
		}
	}
	return ""
}

var subcommands = map[string]func(*cfg.Config, []string) error{
	"skill": cmdSkill, "uninstall": cmdUninstall, "troubleshoot": func(*cfg.Config, []string) error { fmt.Println(troubleshoot); return nil },
	"add": cmdAdd, "list": cmdList, "ls": cmdList, "stop": cmdStop, "start": cmdStart,
	"delete": cmdDelete, "rm": cmdDelete, "status": cmdStatus, "doctor": cmdDoctor, "import": cmdImport,
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		fmt.Println(usage)
		return
	}
	if os.Args[1] == "-v" || os.Args[1] == "--version" {
		fmt.Println("cfex version", version)
		return
	}
	if os.Args[1] == "skill" || os.Args[1] == "troubleshoot" {
		c, _ := cfg.Load()
		if err := subcommands[os.Args[1]](c, os.Args[2:]); err != nil {
			fatal(err.Error())
		}
		return
	}
	c, err := cfg.Load()
	if err != nil {
		fatal(err.Error())
	}
	cmd, args := os.Args[1], os.Args[2:]
	if f, ok := subcommands[cmd]; ok {
		withToken(c)
		if err := f(c, args); err != nil {
			fatal(err.Error())
		}
		return
	}
	// instant mode: <host>:<port> or <host> <port>
	withToken(c)
	if err := cmdInstant(c, os.Args[1:]); err != nil {
		fatal(err.Error())
	}
}
