package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/muthuishere/cfex-cli/internal/cf"
	"github.com/muthuishere/cfex-cli/internal/cfd"
	"github.com/muthuishere/cfex-cli/internal/cfg"
)

// Instant mode: `cfex dev.example.com:3000` puts a local port on your domain right now, in the foreground. It makes a
// quick named tunnel with cloudflared, routes the hostname, runs until Ctrl+C, then removes the tunnel and the DNS record
// it created. --keep turns it into a durable `cfex add`.

const instantPrefix = "cfex-instant-"

func instantName(host string) string { return instantPrefix + strings.ReplaceAll(host, ".", "-") }

type instantArgs struct {
	Host      string
	Port      int
	HTTPS     bool
	VerifyCrt bool
	Keep      bool
}

func instantParse(args []string) (instantArgs, error) {
	var a instantArgs
	pos := []string{}
	for _, x := range args {
		switch x {
		case "--https":
			a.HTTPS = true
		case "--verify-cert":
			a.VerifyCrt = true
		case "--keep":
			a.Keep = true
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

// instantConfig is the cloudflared config for an instant tunnel (pure, unit-tested).
func instantConfig(host string, port int, tunnelID string, https, verify bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tunnel: %s\ncredentials-file: %s\ningress:\n  - hostname: %s\n    service: %s\n", tunnelID, cfd.CredentialsFile(tunnelID), host, serviceURL(https, port))
	if https {
		b.WriteString("    originRequest:\n")
		if !verify {
			b.WriteString("      noTLSVerify: true\n")
		}
		b.WriteString("      disableChunkedEncoding: true\n")
	}
	b.WriteString("  - service: http_status:404\n")
	return b.String()
}

// detectHTTPS reports whether a local service speaks TLS (certificate errors ignored).
func detectHTTPS(port int) bool {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp", fmt.Sprintf("localhost:%d", port), &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"})
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// instantOps is what an instant session needs (a fake in tests).
type instantOps interface {
	Find(name string) (*cfd.Tunnel, error)
	Create(name string) (string, error)
	RouteDNS(name, host string) error
	WriteConfig(id string) (string, error)
	Run(ctx context.Context, cfgFile, name string) error // blocks until ctx is done or cloudflared exits
	DeleteDNS(host, tunnelID string) (done bool, err error)
	DeleteTunnel(name string) error
}

// instantSession runs the whole life of an instant tunnel and ALWAYS cleans up what it created.
func instantSession(ctx context.Context, ops instantOps, host string, out io.Writer) error {
	name := instantName(host)
	t, err := ops.Find(name)
	if err != nil {
		return err
	}
	var id string
	freshTunnel := t == nil
	if freshTunnel {
		fmt.Fprintln(out, "Creating new tunnel named", name)
		if id, err = ops.Create(name); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(out, "Found existing tunnel named", name, "(left over from an earlier run); reusing it")
		id = t.ID
	}
	fmt.Fprintln(out, "Tunnel ID:", id)
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		fmt.Fprintln(out, "\nCleaning up...")
		switch done, err := ops.DeleteDNS(host, id); {
		case err != nil:
			fmt.Fprintf(out, "could not delete the DNS record for %s: %v\n", host, err)
		case done:
			fmt.Fprintln(out, "DNS record for", host, "removed")
		default:
			fmt.Fprintf(out, "DNS record for %s left in place (%s); delete the CNAME %s yourself\n", host, noTokenHint, host)
		}
		if err := ops.DeleteTunnel(name); err != nil {
			fmt.Fprintf(out, "could not delete tunnel %s: %v\n", name, err)
		} else {
			fmt.Fprintln(out, "Tunnel", name, "removed")
		}
	}
	defer cleanup()
	if err := ops.RouteDNS(name, host); err != nil {
		if freshTunnel {
			return fmt.Errorf("DNS route for %s failed (the hostname may already have a record, or DNS is still propagating): %w", host, err)
		}
		fmt.Fprintf(out, "Warning: DNS route creation failed (%v). Continuing; if the tunnel does not work, try again in a few minutes.\n", err)
	} else {
		fmt.Fprintln(out, "DNS route created.")
	}
	cfgFile, err := ops.WriteConfig(id)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Your service will be accessible at https://%s\nPress Ctrl+C to stop the tunnel (it is removed on exit; use --keep to make it permanent)\n", host)
	err = ops.Run(ctx, cfgFile, name)
	if ctx.Err() != nil {
		return nil // Ctrl+C is the normal way out
	}
	return err
}

type realInstant struct {
	c     *cfg.Config
	a     instantArgs
	https bool
	api   *cf.Client
}

func (r *realInstant) Find(n string) (*cfd.Tunnel, error) { return cfd.Find(n) }
func (r *realInstant) Create(n string) (string, error)    { return cfd.Create(n) }
func (r *realInstant) RouteDNS(n, h string) error         { return cfd.RouteDNS(n, h) }
func (r *realInstant) DeleteTunnel(n string) error        { return cfd.Delete(n) }
func (r *realInstant) WriteConfig(id string) (string, error) {
	dir := filepath.Join(cfg.StateDir(), "instant")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, strings.ReplaceAll(r.a.Host, ".", "_")+".yml")
	return p, os.WriteFile(p, []byte(instantConfig(r.a.Host, r.a.Port, id, r.https, r.a.VerifyCrt)), 0o600)
}
func (r *realInstant) Run(ctx context.Context, cfgFile, name string) error {
	bin, err := cfd.Bin()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, "tunnel", "--no-autoupdate", "--config", cfgFile, "run", name)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGINT) }
	cmd.WaitDelay = 10 * time.Second
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// DeleteDNS removes ONLY the record that points at this tunnel (needs the optional API token).
func (r *realInstant) DeleteDNS(host, tunnelID string) (bool, error) {
	if r.api == nil {
		return false, nil
	}
	zones, err := r.api.Zones()
	if err != nil {
		return false, err
	}
	z, err := zoneFor(zones, host)
	if err != nil {
		return false, err
	}
	recs, err := r.api.DNSByName(z.ID, host)
	if err != nil {
		return false, err
	}
	for _, d := range recs {
		if d.Type == "CNAME" && d.Content == tunnelID+".cfargotunnel.com" {
			if err := r.api.DNSDelete(z.ID, d.ID); err != nil {
				return false, err
			}
		}
	}
	return waitUntil(60*time.Second, 2*time.Second, "DNS record deletion", func() bool {
		left, _ := r.api.DNSByName(z.ID, host)
		for _, d := range left {
			if d.Content == tunnelID+".cfargotunnel.com" {
				return false
			}
		}
		return true
	}), nil
}

// waitUntil polls like v1 did (max 60 s): "Waiting for X..." then "X confirmed", or a timeout notice.
func waitUntil(max, every time.Duration, what string, done func() bool) bool {
	fmt.Printf("Waiting for %s to complete...\n", what)
	start := time.Now()
	for !done() {
		if time.Since(start) >= max {
			fmt.Printf("Timeout waiting for %s\n", what)
			return false
		}
		time.Sleep(every)
	}
	fmt.Printf("%s confirmed\n", what)
	return true
}

func cmdInstant(c *cfg.Config, args []string) error {
	a, err := instantParse(args)
	if err != nil {
		return err
	}
	if a.Keep { // --keep: this is a durable route
		fa := []string{strconv.Itoa(a.Port), a.Host}
		if a.HTTPS {
			fa = append(fa, "--https")
		}
		if a.VerifyCrt {
			fa = append(fa, "--verify-cert")
		}
		return cmdAdd(c, fa)
	}
	if err := ensureLogin(); err != nil {
		return err
	}
	if err := validateZone(c, a.Host); err != nil {
		return err
	}
	https := a.HTTPS
	if !https && detectHTTPS(a.Port) {
		https = true
		fmt.Printf("Detected HTTPS service on port %d: switching to HTTPS mode\n", a.Port)
	}
	if https {
		fmt.Printf("Tunneling HTTPS traffic from localhost:%d (certificate verification %s)\n", a.Port, map[bool]string{true: "enabled", false: "disabled"}[a.VerifyCrt])
	} else {
		fmt.Printf("Tunneling HTTP traffic from localhost:%d\n", a.Port)
	}
	if !listening(a.Port) && !https {
		fmt.Fprintf(os.Stderr, "warning: nothing is listening on localhost:%d yet\n", a.Port)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return instantSession(ctx, &realInstant{c: c, a: a, https: https, api: apiClient(c)}, a.Host, os.Stdout)
}

const loginHelp = `cloudflared is not logged in. Authentication:
1. You will be redirected to Cloudflare's login page
2. Log in to your Cloudflare account
3. Select the PRIMARY domain you want to use for tunneling
   (the root domain you own, e.g. yourdomain.com)
4. Authorize Tunnel for that domain

Once authenticated you can create tunnels for any subdomain under that domain.`

// ensureLogin checks cloudflared and runs the guided `cloudflared tunnel login` when there is no cert.pem.
func ensureLogin() error {
	bin, err := cfd.Bin()
	if err != nil {
		return err
	}
	if cfd.HasCert() {
		return nil
	}
	fmt.Println(loginHelp)
	fmt.Println("\nStarting authentication...")
	cmd := exec.Command(bin, "tunnel", "login")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil || !cfd.HasCert() {
		return fmt.Errorf("authentication failed. Please try again (cloudflared tunnel login)")
	}
	fmt.Println("\nAuthentication successful! You can now create tunnels for your domain and its subdomains.")
	return nil
}
