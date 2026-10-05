package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/muthuishere/cfex-cli/internal/cf"
	"github.com/muthuishere/cfex-cli/internal/cfg"
)

// The durable connector for cfex's own tunnel runs as a launchd agent (macOS) or a systemd user unit (Linux).
// The connector token is never written into the unit: it is read from a 0600 file (default) or handed over
// by the configured secret manager at start time.

func tokenFilePath() string { return filepath.Join(cfg.StateDir(), "connector.token") }

func connectorArgs(c *cfg.Config) ([]string, error) {
	cfd, err := exec.LookPath("cloudflared")
	if err != nil {
		return nil, fmt.Errorf("cloudflared not installed (https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/)")
	}
	if strings.TrimSpace(c.ConnectorRunCmd) == "" {
		return []string{cfd, "tunnel", "--no-autoupdate", "run", "--token-file", tokenFilePath()}, nil
	}
	if c.ConnectorEnv == "" {
		return nil, fmt.Errorf("connector_run_cmd needs connector_env (the variable it exports)")
	}
	pre := strings.Fields(c.ConnectorRunCmd)
	bin, err := exec.LookPath(pre[0])
	if err != nil {
		return nil, err
	}
	inner := fmt.Sprintf(`TUNNEL_TOKEN="$%s" exec %s tunnel --no-autoupdate run`, c.ConnectorEnv, cfd)
	return append(append([]string{bin}, pre[1:]...), "/bin/sh", "-c", inner), nil
}

// storeToken keeps the connector token where the service can read it, never printing it.
func storeToken(c *cfg.Config, cl *cf.Client, fresh bool) error {
	if strings.TrimSpace(c.ConnectorRunCmd) == "" {
		if _, err := os.Stat(tokenFilePath()); err == nil && !fresh {
			return nil
		}
		tok, err := cl.TunnelToken(c.AccountID, c.TunnelID)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(cfg.StateDir(), 0o700); err != nil {
			return err
		}
		return os.WriteFile(tokenFilePath(), []byte(tok), 0o600)
	}
	if !fresh {
		return nil
	}
	if strings.TrimSpace(c.ConnectorSetCmd) == "" {
		return fmt.Errorf("connector_run_cmd is set but connector_set_cmd is not: cannot store the new connector token")
	}
	tok, err := cl.TunnelToken(c.AccountID, c.TunnelID)
	if err != nil {
		return err
	}
	f := strings.Fields(c.ConnectorSetCmd)
	cmd := exec.Command(f[0], f[1:]...)
	cmd.Stdin = strings.NewReader(tok)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("connector_set_cmd failed: %v: %s", err, strings.ReplaceAll(string(out), tok, "[redacted]"))
	}
	fmt.Println("connector token stored via connector_set_cmd")
	return nil
}

func plistPath(c *cfg.Config) string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "LaunchAgents", c.ServiceLabel+".plist")
}
func unitPath(c *cfg.Config) string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "systemd", "user", c.ServiceLabel+".service")
}
func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func serviceBody(c *cfg.Config, args []string) string {
	logp := filepath.Join(cfg.StateDir(), "cloudflared.log")
	if runtime.GOOS == "darwin" {
		var a strings.Builder
		for _, x := range args {
			a.WriteString("<string>" + esc(x) + "</string>")
		}
		return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array>%s</array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
<key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, esc(c.ServiceLabel), a.String(), esc(logp), esc(logp))
	}
	q := make([]string, len(args))
	for i, x := range args {
		q[i] = "'" + strings.ReplaceAll(x, "'", `'\''`) + "'"
	}
	return fmt.Sprintf("[Unit]\nDescription=cfex tunnel connector\nAfter=network-online.target\n\n[Service]\nExecStart=%s\nRestart=always\nRestartSec=5\nStandardOutput=append:%s\nStandardError=append:%s\n\n[Install]\nWantedBy=default.target\n",
		strings.Join(q, " "), logp, logp)
}

func serviceLoaded(c *cfg.Config) bool {
	if runtime.GOOS == "darwin" {
		return exec.Command("launchctl", "print", domain()+"/"+c.ServiceLabel).Run() == nil
	}
	return exec.Command("systemctl", "--user", "is-active", "--quiet", c.ServiceLabel+".service").Run() == nil
}

func serviceStart(c *cfg.Config) error {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("launchctl", "bootout", domain()+"/"+c.ServiceLabel).Run()
		if out, err := exec.Command("launchctl", "bootstrap", domain(), plistPath(c)).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
		}
		return nil
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	if out, err := exec.Command("systemctl", "--user", "enable", "--now", c.ServiceLabel+".service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl: %v: %s", err, out)
	}
	return nil
}

func serviceStop(c *cfg.Config) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("launchctl", "bootout", domain()+"/"+c.ServiceLabel).Run()
	}
	return exec.Command("systemctl", "--user", "stop", c.ServiceLabel+".service").Run()
}

// installService writes the unit (idempotent) and starts it.
func installService(c *cfg.Config) error {
	args, err := connectorArgs(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.StateDir(), 0o700); err != nil {
		return err
	}
	body := serviceBody(c, args)
	p := plistPath(c)
	if runtime.GOOS != "darwin" {
		p = unitPath(c)
		os.MkdirAll(filepath.Dir(p), 0o755)
	}
	if b, err := os.ReadFile(p); err == nil && string(b) == body && serviceLoaded(c) {
		return nil
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		return err
	}
	return serviceStart(c)
}
