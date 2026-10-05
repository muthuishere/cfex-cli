package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

func cmdDelete(c *cfg.Config, args []string) error {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	yes := fs.Bool("yes", false, "apply without asking (durable routes: default is a dry run)")
	force := fs.Bool("force", false, "required to delete a PROTECTED managed route")
	hosts := []string{}
	rest := []string{}
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
		} else {
			hosts = append(hosts, a)
		}
	}
	fs.Parse(rest)
	if len(hosts) == 0 {
		return fmt.Errorf("usage: cfex delete <host>... [--yes]")
	}
	var legacy []string
	for _, h := range hosts {
		host, err := normHost(c, h)
		if err != nil {
			return err
		}
		if c.Route(host) == nil { // not a durable route: maybe a cfex v1 tunnel
			legacy = append(legacy, host)
			continue
		}
		if err := protectedGuard(c, host, *force); err != nil {
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
		if err := deleteRoute(cl, c, acct, host, *yes, os.Stdout); err != nil {
			return err
		}
	}
	if len(legacy) > 0 {
		return legacyDelete(c, legacy, *yes, bufio.NewReader(os.Stdin))
	}
	return nil
}

func protectedGuard(c *cfg.Config, host string, force bool) error {
	if r := c.Route(host); r != nil && r.Protected && !force {
		return fmt.Errorf("%s is a protected route: deleting it needs --force", r.Host)
	}
	return nil
}

// deleteRoute removes the ingress rule + the DNS record of a durable (managed) route. It refuses anything cfex did
// not create (not in config, or a DNS record without the managed-by comment).
func deleteRoute(api API, c *cfg.Config, acct, host string, apply bool, out io.Writer) error {
	r := c.Route(host)
	if r == nil {
		return fmt.Errorf("%s is not a cfex-managed route: refusing (cfex only deletes what it created)", host)
	}
	if r.DNSID != "" {
		rec, err := api.DNSGet(r.ZoneID, r.DNSID)
		if err != nil {
			return fmt.Errorf("cannot verify DNS record: %w", err)
		}
		if !strings.Contains(rec.Comment, comment) {
			return fmt.Errorf("DNS record %s lacks the %q comment: refusing to delete", host, comment)
		}
	}
	full, rules, err := api.TunnelConfig(acct, r.TunnelID)
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
	fmt.Fprintf(out, "  - delete DNS record %s (id %s, managed-by cfex)\n", host, r.DNSID)
	if !apply {
		fmt.Fprintln(out, "dry run only; re-run with --yes to apply")
		return nil
	}
	if has {
		next, err := ingress.Remove(rules, host)
		if err != nil {
			return err
		}
		if err := api.PutIngress(acct, r.TunnelID, full, next); err != nil {
			return err
		}
	}
	if r.DNSID != "" {
		if err := api.DNSDelete(r.ZoneID, r.DNSID); err != nil {
			return err
		}
	}
	c.DropRoute(host)
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Fprintln(out, "deleted.")
	return nil
}
