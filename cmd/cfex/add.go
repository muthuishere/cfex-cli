package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/muthuishere/cfex-cli/internal/cf"
	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

func cmdAdd(c *cfg.Config, args []string) error {
	prot := false
	pos := []string{}
	for _, a := range args {
		if a == "--protected" {
			prot = true
		} else {
			pos = append(pos, a)
		}
	}
	if len(pos) != 2 {
		return fmt.Errorf("usage: cfex add <port> <host> [--protected]")
	}
	port, err := strconv.Atoi(pos[0])
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("bad port %q", pos[0])
	}
	host, err := normHost(c, pos[1])
	if err != nil {
		return err
	}
	cl, err := client(c)
	if err != nil {
		return err
	}
	acct, err := pickAccount(c, cl)
	if err != nil {
		return err
	}
	c.AccountID = acct
	if c.Route(host) != nil {
		return fmt.Errorf("%s is already a cfex route (use stop/start/delete)", host)
	}
	zones, err := cl.Zones()
	if err != nil {
		return err
	}
	z, err := zoneFor(zones, host)
	if err != nil {
		return err
	}
	if ex, err := cl.DNSByName(z.ID, host); err != nil {
		return err
	} else if len(ex) > 0 {
		return fmt.Errorf("%s already has a %s DNS record that cfex did not create: refusing to touch it", host, ex[0].Type)
	}
	if !listening(port) {
		fmt.Fprintf(os.Stderr, "warning: nothing is listening on 127.0.0.1:%d yet\n", port)
	}
	if err := ensureTunnel(c, cl); err != nil {
		return err
	}
	full, rules, err := cl.TunnelConfig(acct, c.TunnelID)
	if err != nil {
		return err
	}
	next, err := ingress.Add(rules, host, "http://127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return err
	}
	rec, err := cl.DNSCreate(z.ID, cf.DNS{Type: "CNAME", Name: host, Content: c.TunnelID + ".cfargotunnel.com", Proxied: true, Comment: comment})
	if err != nil {
		return err
	}
	if err := cl.PutIngress(acct, c.TunnelID, full, next); err != nil {
		_ = cl.DNSDelete(z.ID, rec.ID) // roll back the record we just made
		return err
	}
	c.Routes = append(c.Routes, cfg.Route{Host: host, Port: port, Zone: z.Name, ZoneID: z.ID, DNSID: rec.ID,
		Protected: prot, State: "running", Created: time.Now().Format(time.RFC3339), TunnelID: c.TunnelID})
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Printf("routed https://%s -> 127.0.0.1:%d on tunnel %s\n", host, port, c.TunnelName)
	st := ""
	for i := 0; i < 24; i++ { // DNS + edge can take a little while
		if st = httpStatus("https://" + host); st != "err" && st != "530" && st != "404" && st != "502" {
			break
		}
		time.Sleep(5 * time.Second)
	}
	fmt.Printf("live check: https://%s -> HTTP %s\n", host, st)
	return nil
}

// ensureTunnel creates cfex's OWN named tunnel (and its service) on first use.
func ensureTunnel(c *cfg.Config, cl *cf.Client) error {
	fresh := false
	if c.TunnelID == "" {
		ts, err := cl.Tunnels(c.AccountID)
		if err != nil {
			return err
		}
		for _, t := range ts {
			if t.Name == c.TunnelName {
				c.TunnelID = t.ID
			}
		}
		if c.TunnelID == "" {
			sec := make([]byte, 32)
			rand.Read(sec)
			t, err := cl.CreateTunnel(c.AccountID, c.TunnelName, base64.StdEncoding.EncodeToString(sec))
			if err != nil {
				return err
			}
			c.TunnelID, fresh = t.ID, true
		}
		if err := c.Save(); err != nil {
			return err
		}
	}
	if err := storeToken(c, cl, fresh); err != nil {
		return err
	}
	return installService(c)
}
