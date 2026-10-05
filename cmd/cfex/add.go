package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/muthuishere/cfex-cli/internal/cfd"
	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

type addFlags struct {
	Protected, HTTPS, VerifyTLS bool
	Port                        int
	Host                        string
}

func parseAdd(c *cfg.Config, args []string) (addFlags, error) {
	var a addFlags
	pos := []string{}
	for _, x := range args {
		switch x {
		case "--protected":
			a.Protected = true
		case "--https":
			a.HTTPS = true
		case "--verify-cert":
			a.VerifyTLS = true
		default:
			pos = append(pos, x)
		}
	}
	if len(pos) != 2 {
		return a, fmt.Errorf("usage: cfex add <port> <host> [--https] [--verify-cert] [--protected]")
	}
	port, err := strconv.Atoi(pos[0])
	if err != nil || port < 1 || port > 65535 {
		return a, fmt.Errorf("bad port %q", pos[0])
	}
	host, err := normHost(c, pos[1])
	if err != nil {
		return a, err
	}
	a.Port, a.Host = port, host
	return a, nil
}

// cmdAdd: durable route. One locally-managed tunnel owned by cfex (cloudflared tunnel create), the CNAME through
// `cloudflared tunnel route dns` (never overwrites), the ingress rule in tunnel.yml, kept running by launchd/systemd.
func cmdAdd(c *cfg.Config, args []string) error {
	a, err := parseAdd(c, args)
	if err != nil {
		return err
	}
	if c.Route(a.Host) != nil {
		return fmt.Errorf("%s is already a cfex route (use stop/start/delete)", a.Host)
	}
	if err := ensureLogin(); err != nil {
		return err
	}
	if err := validateZone(c, a.Host); err != nil {
		return err
	}
	if !listening(a.Port) {
		fmt.Fprintf(os.Stderr, "warning: nothing is listening on 127.0.0.1:%d yet\n", a.Port)
	}
	if err := ensureTunnel(c); err != nil {
		return err
	}
	rules, err := loadRules()
	if err != nil {
		return err
	}
	next, err := ingress.Add(rules, a.Host, serviceURL(a.HTTPS, a.Port))
	if err != nil {
		return err
	}
	for _, r := range next {
		if h, _ := r["hostname"].(string); h == a.Host {
			for k, v := range routeRule(a.Host, serviceURL(a.HTTPS, a.Port), a.HTTPS, a.VerifyTLS) {
				r[k] = v
			}
		}
	}
	if err := saveRules(c.TunnelID, next); err != nil {
		return err
	}
	if err := cfd.RouteDNS(c.TunnelName, a.Host); err != nil { // refuses to overwrite an existing record
		saveRules(c.TunnelID, rules)
		return err
	}
	route := cfg.Route{Host: a.Host, Port: a.Port, HTTPS: a.HTTPS, VerifyTLS: a.VerifyTLS, Protected: a.Protected,
		State: "running", Created: time.Now().Format(time.RFC3339), TunnelID: c.TunnelID}
	stampOwnership(c, &route)
	c.Routes = append(c.Routes, route)
	if err := c.Save(); err != nil {
		return err
	}
	if err := serviceReload(c); err != nil {
		return err
	}
	fmt.Printf("routed https://%s -> 127.0.0.1:%d on tunnel %s\n", a.Host, a.Port, c.TunnelName)
	st := ""
	for i := 0; i < 24; i++ { // DNS + edge can take a little while
		if st = httpStatus("https://" + a.Host); st != "err" && st != "530" && st != "404" && st != "502" && st != "1033" {
			break
		}
		time.Sleep(5 * time.Second)
	}
	fmt.Printf("live check: https://%s -> HTTP %s\n", a.Host, st)
	return nil
}

// stampOwnership uses the optional API to mark the CNAME cloudflared just made as ours and remember its id.
func stampOwnership(c *cfg.Config, r *cfg.Route) {
	cl := apiClient(c)
	if cl == nil {
		fmt.Fprintln(os.Stderr, "note:", noTokenHint)
		return
	}
	zones, err := cl.Zones()
	if err != nil {
		return
	}
	z, err := zoneFor(zones, r.Host)
	if err != nil {
		return
	}
	recs, err := cl.DNSByName(z.ID, r.Host)
	if err != nil {
		return
	}
	for _, d := range recs {
		if d.Type == "CNAME" && d.Content == c.TunnelID+".cfargotunnel.com" {
			if cl.DNSSetComment(z.ID, d.ID, comment) == nil {
				r.Zone, r.ZoneID, r.DNSID = z.Name, z.ID, d.ID
			}
		}
	}
}

// ensureTunnel creates cfex's OWN tunnel with cloudflared on first use and installs its service.
func ensureTunnel(c *cfg.Config) error {
	if c.TunnelID == "" {
		t, err := cfd.Find(c.TunnelName)
		if err != nil {
			return err
		}
		if t != nil {
			c.TunnelID = t.ID
		} else {
			id, err := cfd.Create(c.TunnelName)
			if err != nil {
				return err
			}
			c.TunnelID = id
		}
		if err := c.Save(); err != nil {
			return err
		}
	}
	return installService(c)
}
