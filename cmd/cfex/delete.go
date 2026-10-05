package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/muthuishere/cfex-cli/internal/cf"
	"github.com/muthuishere/cfex-cli/internal/cfd"
	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

// cmdDelete: `cfex delete <host>...`. On a terminal it lists what will go and asks once (y/N); --yes skips the question
// (scripts); without a terminal and without --yes it only prints the plan (dry run).
func cmdDelete(c *cfg.Config, args []string) error {
	yes, force := false, false
	hosts := []string{}
	for _, a := range args {
		switch a {
		case "--yes", "-y":
			yes = true
		case "--force":
			force = true
		default:
			if strings.HasPrefix(a, "-") {
				return fmt.Errorf("unknown option %s", a)
			}
			hosts = append(hosts, a)
		}
	}
	if len(hosts) == 0 {
		return fmt.Errorf("no domains provided for deletion\nUsage: cfex delete <domain1> [domain2] ... [--yes]")
	}
	type target struct{ host, kind string }
	var ts []target
	fmt.Println("The following tunnels will be deleted:")
	for _, h := range hosts {
		host, err := normHost(c, h)
		if err != nil {
			return err
		}
		kind := "missing"
		if c.Route(host) != nil {
			kind = "route"
			if err := protectedGuard(c, host, force); err != nil {
				return err
			}
		} else if t, _ := cfd.Find(instantName(host)); t != nil {
			kind = "instant"
		}
		if kind == "missing" {
			fmt.Printf("- %s (not found)\n", host)
		} else {
			fmt.Printf("- %s\n", host)
		}
		ts = append(ts, target{host, kind})
	}
	apply := yes
	if !yes && isTTY() {
		if !confirmDelete(os.Stdin, os.Stdout, len(ts)) {
			fmt.Println("Operation cancelled.")
			return nil
		}
		apply = true
	}
	for _, t := range ts {
		fmt.Println("----------------------------------------")
		switch t.kind {
		case "missing":
			fmt.Printf("Skipping %s: no tunnel found (not a cfex route and no instant tunnel)\n", t.host)
		case "route":
			if err := deleteRoute(&realRoute{c: c}, c, t.host, apply, os.Stdout); err != nil {
				return err
			}
		case "instant":
			if err := deleteInstant(&realInstant{c: c, api: apiClient(c)}, t.host, apply, os.Stdout); err != nil {
				return err
			}
		}
	}
	if apply {
		fmt.Println("Deletion process completed.")
	} else {
		fmt.Println("Dry run only (no terminal to ask on). Re-run with --yes to apply.")
	}
	return nil
}

// confirmDelete asks the single y/N question for all targets, like v1 did.
func confirmDelete(in io.Reader, out io.Writer, n int) bool {
	fmt.Fprintf(out, "Are you sure you want to delete these %d tunnels? (y/N) ", n)
	ans, _ := bufio.NewReader(in).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(ans))
	return a == "y" || a == "yes"
}

func protectedGuard(c *cfg.Config, host string, force bool) error {
	if r := c.Route(host); r != nil && r.Protected && !force {
		return fmt.Errorf("%s is a protected route: deleting it needs --force", r.Host)
	}
	return nil
}

// routeOps is what deleting a durable route needs (a fake in tests).
type routeOps interface {
	HasAPI() bool
	DNSGet(zone, id string) (cf.DNS, error)
	DNSDelete(zone, id string) error
	LoadRules() ([]ingress.Rule, error)
	SaveRules(id string, rules []ingress.Rule) error
	Reload() error
}

type realRoute struct{ c *cfg.Config }

func (r *realRoute) HasAPI() bool                        { return apiClient(r.c) != nil }
func (r *realRoute) DNSGet(z, id string) (cf.DNS, error) { return apiClient(r.c).DNSGet(z, id) }
func (r *realRoute) DNSDelete(z, id string) error        { return apiClient(r.c).DNSDelete(z, id) }
func (r *realRoute) LoadRules() ([]ingress.Rule, error)  { return loadRules() }
func (r *realRoute) SaveRules(id string, rules []ingress.Rule) error {
	return saveRules(id, rules)
}
func (r *realRoute) Reload() error { return serviceReload(r.c) }

// deleteRoute removes the ingress rule + the DNS record of a durable route. It refuses anything cfex did not create
// (not in config, or, when the API is available, a DNS record without the managed-by comment). Without an API token the
// ingress rule is removed and the CNAME is left for the user to delete.
func deleteRoute(o routeOps, c *cfg.Config, host string, apply bool, out io.Writer) error {
	r := c.Route(host)
	if r == nil {
		return fmt.Errorf("%s is not a cfex-managed route: refusing (cfex only deletes what it created)", host)
	}
	dnsKnown := o.HasAPI() && r.DNSID != ""
	if dnsKnown {
		rec, err := o.DNSGet(r.ZoneID, r.DNSID)
		if err != nil {
			return fmt.Errorf("cannot verify DNS record: %w", err)
		}
		if !strings.Contains(rec.Comment, comment) {
			return fmt.Errorf("DNS record %s lacks the %q comment: refusing to delete", host, comment)
		}
	}
	rules, err := o.LoadRules()
	if err != nil {
		return err
	}
	has := ingress.Has(rules, host)
	mode := "DRY RUN"
	if apply {
		mode = "APPLYING"
	}
	fmt.Fprintf(out, "%s: delete %s\n", mode, host)
	if has {
		fmt.Fprintf(out, "  - remove ingress rule %s -> 127.0.0.1:%d on tunnel %s\n", host, r.Port, c.TunnelName)
	} else {
		fmt.Fprintf(out, "  - ingress rule already absent (route is stopped)\n")
	}
	if dnsKnown {
		fmt.Fprintf(out, "  - delete DNS record %s (id %s, managed-by cfex)\n", host, r.DNSID)
	} else {
		fmt.Fprintf(out, "  - DNS record %s cannot be deleted without an API token (%s); delete that CNAME yourself\n", host, noTokenHint)
	}
	if !apply {
		return nil
	}
	if has {
		next, err := ingress.Remove(rules, host)
		if err != nil {
			return err
		}
		if err := o.SaveRules(c.TunnelID, next); err != nil {
			return err
		}
	}
	if dnsKnown {
		if err := o.DNSDelete(r.ZoneID, r.DNSID); err != nil {
			return err
		}
	}
	c.DropRoute(host)
	if err := c.Save(); err != nil {
		return err
	}
	if has {
		if err := o.Reload(); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "deleted.")
	return nil
}

type instantDeleteOps interface {
	Find(name string) (*cfd.Tunnel, error)
	DeleteDNS(host, tunnelID string) (bool, error)
	DeleteTunnel(name string) error
}

func deleteInstant(o instantDeleteOps, host string, apply bool, out io.Writer) error {
	name := instantName(host)
	t, err := o.Find(name)
	if err != nil || t == nil {
		return fmt.Errorf("no instant tunnel named %s", name)
	}
	if !apply {
		fmt.Fprintf(out, "DRY RUN: delete instant tunnel %s (id %s) and its DNS record for %s\n", name, t.ID, host)
		return nil
	}
	fmt.Fprintf(out, "Processing deletion for %s...\n", host)
	switch done, err := o.DeleteDNS(host, t.ID); {
	case err != nil:
		fmt.Fprintf(out, "could not delete the DNS record: %v\n", err)
	case !done:
		fmt.Fprintf(out, "DNS record left in place (%s); delete the CNAME %s yourself\n", noTokenHint, host)
	}
	fmt.Fprintln(out, "Cleaning up existing connections and deleting the tunnel...")
	if err := o.DeleteTunnel(name); err != nil {
		return fmt.Errorf("failed to delete tunnel for %s: %w", host, err)
	}
	waitUntil(60*time.Second, 2*time.Second, "tunnel deletion", func() bool { t, _ := o.Find(name); return t == nil })
	fmt.Fprintf(out, "Tunnel for %s has been deleted.\n", host)
	return nil
}
