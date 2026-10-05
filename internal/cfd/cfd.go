// Package cfd drives the cloudflared CLI: create / route dns / list / info / delete tunnels with cloudflared's own
// credentials (cert.pem from `cloudflared tunnel login`, per-tunnel credentials json). cfex only falls back to the
// Cloudflare API for things cloudflared cannot do (DNS record comments, deleting DNS records, remote ingress).
package cfd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Tunnel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Connections []struct {
		ID string `json:"id"`
	} `json:"conns"`
}

func (t Tunnel) Running() bool { return len(t.Connections) > 0 }

var uuidRe = regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)

// Bin returns the cloudflared path.
func Bin() (string, error) {
	p, err := exec.LookPath("cloudflared")
	if err != nil {
		return "", fmt.Errorf("cloudflared is not installed (https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/)")
	}
	return p, nil
}

// CertPath is the origin cert written by `cloudflared tunnel login`.
func CertPath() string {
	if p := os.Getenv("TUNNEL_ORIGIN_CERT"); p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".cloudflared", "cert.pem")
}

func HasCert() bool { _, err := os.Stat(CertPath()); return err == nil }

// ErrNoCert explains how to log in.
var ErrNoCert = fmt.Errorf("cloudflared is not logged in: run `cloudflared tunnel login` once and pick your domain")

// ParseList parses `cloudflared tunnel list --output json` (pure, unit-tested). "null" and "" mean no tunnels.
func ParseList(b []byte) ([]Tunnel, error) {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		return nil, nil
	}
	var ts []Tunnel
	if err := json.Unmarshal(b, &ts); err != nil {
		return nil, fmt.Errorf("cannot parse cloudflared tunnel list: %w", err)
	}
	return ts, nil
}

// ParseID pulls the tunnel id out of `cloudflared tunnel create` output.
func ParseID(out string) string { return uuidRe.FindString(out) }

func run(args ...string) (string, error) {
	bin, err := Bin()
	if err != nil {
		return "", err
	}
	out, err := exec.Command(bin, args...).CombinedOutput()
	return string(out), err
}

func List() ([]Tunnel, error) {
	if !HasCert() {
		return nil, ErrNoCert
	}
	bin, err := Bin()
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(bin, "tunnel", "list", "--output", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("cloudflared tunnel list: %w", err)
	}
	return ParseList(out)
}

func Find(name string) (*Tunnel, error) {
	ts, err := List()
	if err != nil {
		return nil, err
	}
	for i := range ts {
		if ts[i].Name == name {
			return &ts[i], nil
		}
	}
	return nil, nil
}

// Create makes a locally-managed tunnel; its credentials json lands in ~/.cloudflared.
func Create(name string) (string, error) {
	if !HasCert() {
		return "", ErrNoCert
	}
	out, err := run("tunnel", "create", name)
	id := ParseID(out)
	if err != nil || id == "" {
		return "", fmt.Errorf("cloudflared tunnel create %s: %s", name, strings.TrimSpace(out))
	}
	return id, nil
}

// RouteDNS creates the CNAME. It never passes -f, so an existing record is never overwritten.
func RouteDNS(tunnel, host string) error {
	out, err := run("tunnel", "route", "dns", tunnel, host)
	if err != nil {
		return fmt.Errorf("cloudflared tunnel route dns: %s", strings.TrimSpace(out))
	}
	return nil
}

// Delete cleans up stale connections and deletes the tunnel.
func Delete(name string) error {
	run("tunnel", "cleanup", name)
	if out, err := run("tunnel", "delete", "-f", name); err != nil {
		return fmt.Errorf("cloudflared tunnel delete: %s", strings.TrimSpace(out))
	}
	return nil
}

func CredentialsFile(id string) string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".cloudflared", id+".json")
}

func Version() string {
	out, err := run("--version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
