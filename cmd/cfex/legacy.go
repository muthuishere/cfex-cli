package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/muthuishere/cfex-cli/internal/cf"
	"github.com/muthuishere/cfex-cli/internal/cfg"
)

// cfex v1 behaviour: one cloudflared-managed tunnel per hostname, run in the foreground until Ctrl+C.
// `cfex host:3000`, `cfex host 3000`, --https, --verify-cert, `cfex list`, `cfex delete host` keep working.

type legacyArgs struct {
	Host      string
	Port      int
	HTTPS     bool
	VerifyCrt bool
}

func legacyParse(args []string) (legacyArgs, error) {
	var a legacyArgs
	pos := []string{}
	for _, x := range args {
		switch x {
		case "--https":
			a.HTTPS = true
		case "--verify-cert":
			a.VerifyCrt = true
		default:
			if strings.HasPrefix(x, "-") {
				return a, fmt.Errorf("unknown option %s\n\n%s", x, usage)
			}
			pos = append(pos, x)
		}
	}
	var port string
	if len(pos) == 1 {
		if m := regexp.MustCompile(`^([^:]+):([0-9]+)$`).FindStringSubmatch(pos[0]); m != nil {
			a.Host, port = m[1], m[2]
		}
	} else if len(pos) == 2 {
		a.Host, port = pos[0], pos[1]
	}
	if a.Host == "" {
		return a, fmt.Errorf("invalid arguments\n\n%s", usage)
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return a, fmt.Errorf("local_port must be a number")
	}
	if p < 1 || p > 65535 {
		return a, fmt.Errorf("local_port must be between 1 and 65535")
	}
	a.Port, a.Host = p, strings.ToLower(a.Host)
	return a, nil
}

func legacyName(host string) string { return "tunnel_" + strings.ReplaceAll(host, ".", "_") }

func cloudflaredDir() string { h, _ := os.UserHomeDir(); return filepath.Join(h, ".cloudflared") }

// legacyConfig is the per-hostname cloudflared config v1 wrote (pure, unit-tested).
func legacyConfig(https bool, port int, tunnelID string, ignoreCert bool) string {
	creds := filepath.Join(cloudflaredDir(), tunnelID+".json")
	if !https {
		return fmt.Sprintf("url: http://localhost:%d\ntunnel: %s\ncredentials-file: %s\n", port, tunnelID, creds)
	}
	s := fmt.Sprintf("url: https://localhost:%d\ntunnel: %s\ncredentials-file: %s\noriginRequest:\n", port, tunnelID, creds)
	if ignoreCert {
		s += "  noTLSVerify: true\n"
	}
	return s + "  disableChunkedEncoding: true\n"
}

func detectHTTPS(port int) bool {
	d := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", fmt.Sprintf("localhost:%d", port), &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"})
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

type ltunnel struct{ ID, Name string }

func localTunnels() ([]ltunnel, error) {
	out, err := exec.Command("cloudflared", "tunnel", "list", "--output", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("cloudflared tunnel list: %v (run `cloudflared tunnel login` once)", err)
	}
	var ts []ltunnel
	if s := strings.TrimSpace(string(out)); s != "" && s != "null" {
		var raw []struct{ ID, Name string }
		if err := json.Unmarshal(out, &raw); err != nil {
			return nil, err
		}
		for _, r := range raw {
			ts = append(ts, ltunnel(r))
		}
	}
	return ts, nil
}

func legacyRun(c *cfg.Config, args []string) error {
	a, err := legacyParse(args)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("cloudflared"); err != nil {
		return fmt.Errorf("cloudflared is not installed")
	}
	if _, err := os.Stat(filepath.Join(cloudflaredDir(), "cert.pem")); err != nil {
		fmt.Println("cloudflared is not authenticated: opening Cloudflare login. Choose the domain you want to tunnel, then run this command again.")
		cmd := exec.Command("cloudflared", "tunnel", "login")
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Run()
		return fmt.Errorf("authenticate, then re-run")
	}
	cl, err := client(c)
	if err != nil {
		return err
	}
	zones, err := cl.Zones()
	if err != nil {
		return err
	}
	z, err := zoneFor(zones, a.Host)
	if err != nil {
		return err
	}
	name := legacyName(a.Host)
	cfgFile := filepath.Join(cloudflaredDir(), strings.ReplaceAll(a.Host, ".", "_")+"_config.yml")
	ts, err := localTunnels()
	if err != nil {
		return err
	}
	var id string
	for _, t := range ts {
		if t.Name == name {
			id = t.ID
		}
	}
	if id != "" {
		fmt.Println("Found existing tunnel named", name)
		exec.Command("cloudflared", "tunnel", "cleanup", name).Run()
	} else {
		recs, err := cl.DNSByName(z.ID, a.Host)
		if err != nil {
			return err
		}
		for _, r := range recs { // an orphan from an earlier tunnel: only tunnel CNAMEs are replaced
			if r.Type != "CNAME" || !strings.HasSuffix(r.Content, ".cfargotunnel.com") {
				return fmt.Errorf("%s already has a %s record that is not a tunnel: refusing to replace it", a.Host, r.Type)
			}
			fmt.Println("Removing orphaned tunnel DNS record for", a.Host)
			if err := cl.DNSDelete(z.ID, r.ID); err != nil {
				return err
			}
		}
		fmt.Println("Creating new tunnel named", name)
		out, err := exec.Command("cloudflared", "tunnel", "create", name).CombinedOutput()
		id = regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).FindString(string(out))
		if err != nil || id == "" {
			return fmt.Errorf("failed to create tunnel or capture its id: %s", strings.TrimSpace(string(out)))
		}
	}
	fmt.Println("Tunnel ID:", id)
	https := a.HTTPS
	if !https && detectHTTPS(a.Port) {
		https = true
		fmt.Printf("Detected HTTPS service on port %d: switching to HTTPS mode\n", a.Port)
	}
	if err := os.WriteFile(cfgFile, []byte(legacyConfig(https, a.Port, id, !a.VerifyCrt)), 0o600); err != nil {
		return err
	}
	fmt.Println("Config written to", cfgFile)
	if out, err := exec.Command("cloudflared", "tunnel", "route", "dns", name, a.Host).CombinedOutput(); err != nil {
		fmt.Printf("Warning: DNS route creation failed (%s). Continuing; if the tunnel does not work, try again in a few minutes.\n", strings.TrimSpace(string(out)))
	}
	fmt.Printf("Your service will be accessible at https://%s\nPress Ctrl+C to stop the tunnel\n", a.Host)
	bin, _ := exec.LookPath("cloudflared")
	return syscall.Exec(bin, []string{"cloudflared", "tunnel", "--config", cfgFile, "run", name}, os.Environ())
}

// legacyDelete removes cfex v1 tunnels (and their tunnel DNS records), with the same confirmation v1 asked for.
func legacyDelete(c *cfg.Config, hosts []string, yes bool, in *bufio.Reader) error {
	ts, err := localTunnels()
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, t := range ts {
		have[t.Name] = true
	}
	fmt.Println("The following tunnels will be deleted:")
	var todo []string
	for _, h := range hosts {
		if have[legacyName(h)] {
			fmt.Println("-", h)
			todo = append(todo, h)
		} else {
			fmt.Println("-", h, "(not found: neither a cfex route nor a cfex v1 tunnel; nothing will be touched)")
		}
	}
	if len(todo) == 0 {
		return fmt.Errorf("nothing to delete")
	}
	if !yes {
		fmt.Printf("Are you sure you want to delete these %d tunnels? (y/N) ", len(todo))
		ans, _ := in.ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(ans)); a != "y" && a != "yes" {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}
	var cl *cf.Client
	if t := token(c); t != "" {
		cl = cf.New(t)
	} else {
		fmt.Println("note: no API token, DNS records will be left for you to remove")
	}
	var zones []cf.Zone
	if cl != nil {
		zones, _ = cl.Zones()
	}
	for _, h := range todo {
		name := legacyName(h)
		fmt.Println("Processing deletion for", h)
		exec.Command("cloudflared", "tunnel", "cleanup", name).Run()
		if out, err := exec.Command("cloudflared", "tunnel", "delete", "-f", name).CombinedOutput(); err != nil {
			fmt.Printf("Failed to delete tunnel for %s, skipping DNS cleanup: %s\n", h, strings.TrimSpace(string(out)))
			continue
		}
		if cl != nil {
			if z, err := zoneFor(zones, h); err == nil {
				recs, _ := cl.DNSByName(z.ID, h)
				for _, r := range recs {
					if r.Type == "CNAME" && strings.HasSuffix(r.Content, ".cfargotunnel.com") {
						cl.DNSDelete(z.ID, r.ID)
					}
				}
			}
		}
		fmt.Println("Tunnel for", h, "has been deleted.")
	}
	return nil
}
