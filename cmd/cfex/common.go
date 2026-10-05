package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/muthuishere/cfex-cli/internal/cf"
	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

// comment is stamped on every DNS record cfex creates; delete refuses records without it.
const comment = "managed-by: cfex"

// API is the slice of the Cloudflare client the mutating commands need (a fake in tests).
type API interface {
	DNSGet(zone, id string) (cf.DNS, error)
	DNSDelete(zone, id string) error
	TunnelConfig(acct, tid string) (map[string]any, []ingress.Rule, error)
	PutIngress(acct, tid string, full map[string]any, rules []ingress.Rule) error
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// classify says whether a tunnel may be touched. Only "managed" and (via confirmation) "legacy" can be mutated.
func classify(c *cfg.Config, tunnel, id string) string {
	switch {
	case tunnel == c.TunnelName || (id != "" && id == c.TunnelID):
		return "managed"
	case matchAny(c.Protected, tunnel):
		return "PROTECTED"
	case matchAny(c.Client, tunnel):
		return "client"
	}
	for _, a := range c.Adopted {
		if a.Tunnel == tunnel {
			return "adopted"
		}
	}
	if strings.HasPrefix(tunnel, "tunnel_") {
		return "legacy" // created by cfex v1 (`cfex host:port`)
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

func client(c *cfg.Config) (*cf.Client, error) {
	t := token(c)
	if t == "" {
		return nil, fmt.Errorf("no Cloudflare API token: set CLOUDFLARE_API_TOKEN (create one with Zone:DNS:Edit, Zone:Read and Account:Cloudflare Tunnel:Edit) or token_cmd in %s", cfg.Path())
	}
	return cf.New(t), nil
}

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
