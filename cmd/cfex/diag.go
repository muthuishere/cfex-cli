package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/disc"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

func cmdStatus(c *cfg.Config, args []string) error {
	fmt.Printf("tunnel      %s (%s)\n", c.TunnelName, orDash(c.TunnelID))
	fmt.Printf("service     %s loaded=%v\n", c.ServiceLabel, serviceLoaded(c))
	n := 0
	for _, r := range c.Routes {
		if r.State == "stopped" {
			n++
		}
	}
	fmt.Printf("routes      %d (%d stopped)\n", len(c.Routes), n)
	fmt.Printf("cloudflared %d processes on this machine\n", disc.RunningCloudflaredCount())
	if c.TunnelID != "" {
		if cl, err := client(c); err == nil {
			if acct, err := pickAccount(c, cl); err == nil {
				if ts, err := cl.Tunnels(acct); err == nil {
					for _, t := range ts {
						if t.ID == c.TunnelID {
							fmt.Printf("api         %s, %d connections\n", t.Status, len(t.Connections))
						}
					}
				}
			}
		}
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "not created yet"
	}
	return s
}

func cmdDoctor(c *cfg.Config, args []string) error {
	bad := 0
	check := func(ok bool, what, detail string) {
		m := "ok  "
		if !ok {
			m = "FAIL"
			bad++
		}
		fmt.Printf("[%s] %s %s\n", m, what, detail)
	}
	if p, err := exec.LookPath("cloudflared"); err == nil {
		v, _ := exec.Command(p, "--version").Output()
		check(true, "cloudflared installed", strings.TrimSpace(string(v)))
	} else {
		check(false, "cloudflared installed", "see https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/")
	}
	cl, cerr := client(c)
	if cerr != nil {
		check(false, "Cloudflare API token available", cerr.Error())
		return fmt.Errorf("%d check(s) failed", bad)
	}
	check(true, "Cloudflare API token available", "")
	acct, aerr := pickAccount(c, cl)
	check(aerr == nil, "Cloudflare account reachable", fmt.Sprint(aerr))
	if c.TunnelID == "" {
		fmt.Println("[info] cfex's own tunnel is created on the first `cfex add`")
	} else if aerr == nil {
		check(serviceLoaded(c), "service "+c.ServiceLabel+" running", "")
		_, rules, err := cl.TunnelConfig(acct, c.TunnelID)
		check(err == nil, "ingress readable", fmt.Sprint(err))
		zs, _ := cl.Zones()
		for _, r := range c.Routes {
			if r.State == "stopped" {
				continue
			}
			svc, has := ingress.Service(rules, r.Host)
			check(has, "ingress rule for "+r.Host, svc)
			if z, err := zoneFor(zs, r.Host); err == nil {
				recs, _ := cl.DNSByName(z.ID, r.Host)
				check(len(recs) == 1 && strings.HasPrefix(recs[0].Content, c.TunnelID), "DNS -> tunnel for "+r.Host, "")
			}
		}
	}
	_, toks := disc.Scripts()
	if len(toks) > 0 {
		fmt.Printf("[warn] %d plaintext token files in ~/.cloudflared (names only): %s\n       consider moving them into a secret manager.\n", len(toks), strings.Join(toks, " "))
	}
	if bad > 0 {
		return fmt.Errorf("%d check(s) failed", bad)
	}
	return nil
}

// cmdImport adopts an existing tunnel into config WITHOUT changing what it serves: read-only discovery, a report,
// then a config record (config only; no API write, no service change).
func cmdImport(c *cfg.Config, args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	dry := fs.Bool("dry-run", false, "report only; do not record")
	target := ""
	rest := []string{}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") && target == "" {
			target = a
		} else {
			rest = append(rest, a)
		}
	}
	fs.Parse(rest)
	if target == "" {
		return fmt.Errorf("usage: cfex import <script|tunnel> [--dry-run]")
	}
	cl, err := client(c)
	if err != nil {
		return err
	}
	acct, err := pickAccount(c, cl)
	if err != nil {
		return err
	}
	ts, err := cl.Tunnels(acct)
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(strings.TrimSuffix(target[strings.LastIndex(target, "/")+1:], ".sh"), "-tunnel")
	var job *disc.Job
	jobs := disc.Jobs()
	for i, j := range jobs {
		if (j.Script != "" && strings.Contains(j.Script, base+"-tunnel.sh")) || j.Label == target {
			job = &jobs[i]
		}
	}
	idx := -1
	for i, t := range ts {
		if t.Name == target || (job != nil && job.TunnelID == t.ID) || t.Name == base || t.Name == base+"-demo" {
			idx = i
		}
	}
	if idx < 0 {
		return fmt.Errorf("no account tunnel matches %q (use the tunnel name from `cfex list`)", target)
	}
	found := ts[idx]
	_, rules, _ := cl.TunnelConfig(acct, found.ID)
	fmt.Printf("tunnel   %s (%s) status=%s class=%s\n", found.Name, found.ID, found.Status, classify(c, found.Name, found.ID))
	for _, r := range rules {
		if h, _ := r["hostname"].(string); h != "" {
			fmt.Printf("route    %s -> %v\n", h, r["service"])
		}
	}
	a := cfg.Adopted{Tunnel: found.Name, TunnelID: found.ID}
	if job != nil {
		a.Label, a.Script, a.TokenFile = job.Label, job.Script, job.TokenFile
		fmt.Printf("service  %s loaded=%v (token file NAME: %s)\n", job.Label, job.Loaded, job.TokenFile)
	} else {
		fmt.Println("service  no launchd job found for this tunnel")
	}
	if *dry {
		fmt.Println("dry run: nothing recorded")
		return nil
	}
	for _, x := range c.Adopted {
		if x.TunnelID == a.TunnelID {
			fmt.Println("already adopted")
			return nil
		}
	}
	c.Adopted = append(c.Adopted, a)
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "adopted into config; serving is unchanged. Protected/client classes still apply.")
	return nil
}
