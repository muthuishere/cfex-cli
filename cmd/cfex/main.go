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

const version = "2.0.0"

const usage = `cfex: expose a local port on your own domain through Cloudflare, and manage every tunnel on this machine.

  cfex <host>:<port> [--https] [--verify-cert]   foreground tunnel, Ctrl+C stops it (cfex v1 behaviour, unchanged)
  cfex <host> <port>                             same, original syntax
  cfex add <port> <host> [--protected]           durable route on cfex's own tunnel, kept running by launchd/systemd
  cfex list [--json] [--no-http]                 EVERYTHING on this machine: managed, protected, client, legacy, unmanaged
  cfex stop  <host|tunnel>                       stop serving (keeps config + DNS)
  cfex start <host|tunnel>                       serve again
  cfex delete <host>... [--yes]                  durable route: dry run unless --yes; v1 tunnel: asks to confirm
  cfex status                                    cfex's own tunnel at a glance
  cfex doctor                                    cloudflared, token, service, DNS <-> ingress, plaintext token files
  cfex import <script|tunnel> [--dry-run]        adopt an existing tunnel into config WITHOUT changing what it serves
  cfex -h | -v

Token: CLOUDFLARE_API_TOKEN (CLOUDFLARE_API_KEY still accepted), or 'token_cmd' in the config.
Config: ~/.config/cfex/config.yaml · state: ~/.local/share/cfex/`

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
	// cfex v1 syntax: <host>:<port> or <host> <port>, foreground.
	withToken(c)
	if err := legacyRun(c, os.Args[1:]); err != nil {
		fatal(err.Error())
	}
}
