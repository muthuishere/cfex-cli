package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/muthuishere/cfex-cli/internal/cf"
	"github.com/muthuishere/cfex-cli/internal/cfg"
)

// comment is stamped on every DNS record cfex creates; delete refuses records without it.
const comment = "managed-by: cfex"

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// classify says whether a tunnel may be touched. Only "managed" routes and "instant" tunnels (made by `cfex host:port`) can be mutated.
func classify(c *cfg.Config, tunnel, id string) string {
	switch {
	case tunnel == c.TunnelName || (id != "" && id == c.TunnelID):
		return "managed"
	case matchAny(c.Protected, tunnel):
		return "PROTECTED"
	case matchAny(c.Client, tunnel):
		return "client"
	case strings.HasPrefix(tunnel, instantPrefix):
		return "instant"
	}
	for _, a := range c.Adopted {
		if a.Tunnel == tunnel {
			return "adopted"
		}
	}
	return "unmanaged"
}

func normHost(c *cfg.Config, h string) (string, error) {
	h = strings.ToLower(strings.TrimSpace(h))
	if !strings.Contains(h, ".") {
		if c.DefaultZone == "" {
			return "", fmt.Errorf("%q is not a full hostname and no default_zone is set in %s", h, cfg.Path())
		}
		h += "." + c.DefaultZone
	}
	return h, nil
}

func zoneFor(zs []cf.Zone, host string) (cf.Zone, error) {
	best := cf.Zone{}
	for _, z := range zs {
		if (host == z.Name || strings.HasSuffix(host, "."+z.Name)) && len(z.Name) > len(best.Name) {
			best = z
		}
	}
	if best.ID == "" {
		return best, fmt.Errorf("%s is not in any Cloudflare zone this token can see", host)
	}
	return best, nil
}

func httpStatus(url string) string {
	// Resolve through a public resolver: the local one caches NXDOMAIN for a name queried just before it was created.
	res := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, n, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, n, "1.1.1.1:53")
	}}
	tr := &http.Transport{DialContext: (&net.Dialer{Timeout: 5 * time.Second, Resolver: res}).DialContext}
	cl := &http.Client{Transport: tr, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := cl.Get(url)
	if err != nil {
		return "err"
	}
	resp.Body.Close()
	return fmt.Sprint(resp.StatusCode)
}

func portOf(service string) string {
	if i := strings.LastIndex(service, ":"); i >= 0 && strings.HasPrefix(service, "http") {
		return service[i+1:]
	}
	return service
}

func listening(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// apiClient returns the optional Cloudflare API client, or nil when no token is available.
func apiClient(c *cfg.Config) *cf.Client {
	if t := token(c); t != "" {
		return cf.New(t)
	}
	return nil
}

const noTokenHint = "no API token: set CLOUDFLARE_API_TOKEN (Zone:Read, DNS:Edit) to enable DNS ownership checks and DNS deletion"

// pickAccount uses account_id from config; otherwise the token's only account. Several accounts: the user must choose.
func pickAccount(c *cfg.Config, cl *cf.Client) (string, error) {
	if c.AccountID != "" {
		return c.AccountID, nil
	}
	as, err := cl.Accounts()
	if err != nil {
		return "", err
	}
	switch len(as) {
	case 0:
		return "", fmt.Errorf("token sees no Cloudflare account")
	case 1:
		return as[0].ID, nil
	}
	names := []string{}
	for _, a := range as {
		names = append(names, a.Name+" ("+a.ID+")")
	}
	return "", fmt.Errorf("token sees several accounts, set account_id in %s: %s", cfg.Path(), strings.Join(names, ", "))
}

// validateZone, when an API token is available, checks that the host's domain is visible to it and explains the usual
// causes if not. Without a token the check is skipped (cloudflared reports problems itself).
func validateZone(c *cfg.Config, host string) error {
	cl := apiClient(c)
	if cl == nil {
		return nil
	}
	zones, err := cl.Zones()
	if err == nil {
		_, err = zoneFor(zones, host)
	}
	if err != nil {
		return fmt.Errorf("could not fetch zone data for %s: %v\nPossible issues:\n1. Invalid API token\n2. The domain is not in your Cloudflare account\n3. The API token has no permission for this zone", host, err)
	}
	return nil
}

// isTTY reports whether stdin is an interactive terminal (a variable so tests can force either answer).
var isTTY = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return true
}
