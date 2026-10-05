package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cfexcli "github.com/muthuishere/cfex-cli"
	"github.com/muthuishere/cfex-cli/internal/cfg"
)

// selfPath is the running binary (a variable so tests never delete the test binary).
var selfPath = os.Executable

func skillDirs() []string {
	h, _ := os.UserHomeDir()
	return []string{filepath.Join(h, ".claude", "skills", "cfex"), filepath.Join(h, ".agents", "skills", "cfex")}
}

// cmdSkill: `cfex skill install|uninstall|path`.
func cmdSkill(c *cfg.Config, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: cfex skill install | uninstall | path")
	}
	switch args[0] {
	case "install":
		for _, d := range skillDirs() {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(d, "SKILL.md"), cfexcli.Skill, 0o644); err != nil {
				return err
			}
			fmt.Println("installed", filepath.Join(d, "SKILL.md"))
		}
	case "uninstall":
		for _, d := range skillDirs() {
			if err := os.RemoveAll(d); err != nil {
				return err
			}
			fmt.Println("removed", d)
		}
	case "path":
		for _, d := range skillDirs() {
			fmt.Println(d)
		}
	default:
		return fmt.Errorf("usage: cfex skill install | uninstall | path")
	}
	return nil
}

func ask(q string) bool {
	if !isTTY() {
		return false
	}
	fmt.Print(q + " (y/N) ")
	a, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	a = strings.ToLower(strings.TrimSpace(a))
	return a == "y" || a == "yes"
}

// cmdUninstall removes the cfex binary and skill. Everything else is opt-in (a prompt on a terminal, or a flag), and
// tunnels and DNS records in your Cloudflare account are never deleted.
func cmdUninstall(c *cfg.Config, args []string) error {
	var stopSvc, purgeCfg, purgeCFD bool
	for _, a := range args {
		switch a {
		case "--stop-service":
			stopSvc = true
		case "--purge-config":
			purgeCfg = true
		case "--purge-cloudflared":
			purgeCFD = true
		case "--yes", "-y":
		default:
			return fmt.Errorf("usage: cfex uninstall [--stop-service] [--purge-config] [--purge-cloudflared]")
		}
	}
	for _, d := range skillDirs() {
		if _, err := os.Stat(d); err == nil {
			os.RemoveAll(d)
			fmt.Println("removed skill", d)
		}
	}
	if c.TunnelID != "" && (stopSvc || ask("Stop and remove the cfex service? Your durable routes will stop (the tunnel and DNS records are not deleted)")) {
		serviceStop(c)
		os.Remove(plistPath(c))
		os.Remove(unitPath(c))
		fmt.Println("removed service", c.ServiceLabel)
	}
	if purgeCfg || ask("Remove cfex config and state ("+cfg.Dir()+", "+cfg.StateDir()+")?") {
		os.RemoveAll(cfg.Dir())
		os.RemoveAll(cfg.StateDir())
		fmt.Println("removed cfex config and state")
	}
	if purgeCFD || ask("Remove cloudflared configuration files (~/.cloudflared)? This deletes your cloudflared login and every tunnel credentials file") {
		os.RemoveAll(cloudflaredDir())
		fmt.Println("removed", cloudflaredDir())
	}
	self, err := selfPath()
	if err == nil {
		if rerr := os.Remove(self); rerr != nil {
			fmt.Printf("could not remove %s (%v); run: sudo rm %s\n", self, rerr, self)
			if p, e := exec.LookPath("cfex"); e == nil && p != self {
				fmt.Println("another cfex is also on PATH:", p)
			}
		} else {
			fmt.Println("removed", self)
		}
	}
	fmt.Println("Uninstallation completed. Tunnels and DNS records in your Cloudflare account were left untouched (use `cfex delete` first if you want them gone).")
	return nil
}
