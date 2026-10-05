package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/muthuishere/cfex-cli/internal/cfd"
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
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "not created yet"
	}
	return s
}

const tokenHelp = `Create an API token at https://dash.cloudflare.com/profile/api-tokens
  1. Click 'Create Token' and use the 'Edit zone DNS' template
  2. Under 'Zone Resources' choose 'Include Specific Zone' and pick your domain
  3. Export it: export CLOUDFLARE_API_TOKEN='your-api-token'  (CLOUDFLARE_API_KEY also works)
  For persistence, add that line to ~/.zshrc or ~/.bashrc.`

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
	_, cerr := cfd.Bin()
	check(cerr == nil, "cloudflared installed", cfd.Version())
	if cerr != nil {
		fmt.Println("       install: https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/")
	}
	login := cfd.HasCert()
	check(login, "cloudflared logged in (needed for: instant tunnels, add, list, delete)", cfd.CertPath())
	if !login {
		fmt.Println("       fix: cloudflared tunnel login   (pick the domain you want to tunnel)")
	}
	api := apiClient(c)
	if api == nil {
		fmt.Println("[info] no API token (optional). Without it cfex cannot: verify/stamp DNS ownership, delete DNS records, read remote ingress, validate zones.")
		fmt.Println(indent(tokenHelp))
	} else {
		_, err := pickAccount(c, api)
		check(err == nil, "Cloudflare API token works (enables DNS ownership checks, DNS deletion, remote ingress)", errString(err))
	}
	if c.TunnelID == "" {
		fmt.Println("[info] cfex's durable tunnel is created on the first `cfex add`")
	} else {
		check(serviceLoaded(c), "service "+c.ServiceLabel+" running", "")
		rules, err := loadRules()
		check(err == nil, "tunnel.yml readable", errString(err))
		for _, r := range c.Routes {
			if r.State == "stopped" {
				continue
			}
			svc, has := ingress.Service(rules, r.Host)
			check(has, "ingress rule for "+r.Host, svc)
			if api != nil && r.DNSID != "" {
				d, err := api.DNSGet(r.ZoneID, r.DNSID)
				check(err == nil && strings.HasPrefix(d.Content, c.TunnelID), "DNS -> tunnel for "+r.Host, errString(err))
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

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func indent(s string) string { return "       " + strings.ReplaceAll(s, "\n", "\n       ") }

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
	ts, err := allTunnels(c)
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
	run := map[bool]string{true: "running", false: "down"}[found.Running]
	fmt.Printf("tunnel   %s (%s) %s class=%s\n", found.Name, found.ID, run, classify(c, found.Name, found.ID))
	for _, r := range found.Rules {
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
