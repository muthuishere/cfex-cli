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

	"github.com/muthuishere/cfex-cli/internal/cfd"
	"github.com/muthuishere/cfex-cli/internal/cfg"
)

// The durable tunnel runs as a launchd agent (macOS) or a systemd user unit (Linux): `cloudflared tunnel --config
// ~/.config/cfex/tunnel.yml run <name>`. It uses cloudflared's own credentials file, so no token is ever written into the unit.

func connectorArgs(c *cfg.Config) ([]string, error) {
	bin, err := cfd.Bin()
	if err != nil {
		return nil, err
	}
	return []string{bin, "tunnel", "--no-autoupdate", "--config", cfg.TunnelFile(), "run", c.TunnelName}, nil
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
	return fmt.Sprintf("[Unit]\nDescription=cfex tunnel\nAfter=network-online.target\n\n[Service]\nExecStart=%s\nRestart=always\nRestartSec=5\nStandardOutput=append:%s\nStandardError=append:%s\n\n[Install]\nWantedBy=default.target\n",
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

// serviceReload makes cloudflared pick up a changed tunnel.yml (it does not hot-reload local config): a restart,
// so every route on the durable tunnel reconnects for a second or two.
func serviceReload(c *cfg.Config) error {
	if !serviceLoaded(c) {
		return installService(c)
	}
	if runtime.GOOS == "darwin" {
		return exec.Command("launchctl", "kickstart", "-k", domain()+"/"+c.ServiceLabel).Run()
	}
	return exec.Command("systemctl", "--user", "restart", c.ServiceLabel+".service").Run()
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
