package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/disc"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

func cmdStop(c *cfg.Config, args []string) error  { return stopStart(c, args, false) }
func cmdStart(c *cfg.Config, args []string) error { return stopStart(c, args, true) }

// forced: CFEX_FORCE=1 lets stop/start touch a protected managed route (a deliberate, explicit act).
var forced = func() bool { return os.Getenv("CFEX_FORCE") == "1" }

// stopStart: a managed route is stopped by removing ONLY its ingress rule (DNS + config kept) and started by
// re-adding it; cfex's own tunnel via its service; adopted non-protected tunnels via their launchd job.
// Anything protected / client / unmanaged is refused.
func stopStart(c *cfg.Config, args []string, start bool) error {
	verb := map[bool]string{true: "start", false: "stop"}[start]
	if len(args) != 1 {
		return fmt.Errorf("usage: cfex %s <host|tunnel>", verb)
	}
	target := args[0]
	if target == c.TunnelName {
		if start {
			return serviceStart(c)
		}
		return serviceStop(c)
	}
	host, herr := normHost(c, target)
	if r := c.Route(host); herr == nil && r != nil {
		if r.Protected && !forced() {
			return fmt.Errorf("%s is a protected route: %s needs an explicit decision (set CFEX_FORCE=1)", host, verb)
		}
		cl, err := client(c)
		if err != nil {
			return err
		}
		acct, err := pickAccount(c, cl)
		if err != nil {
			return err
		}
		full, rules, err := cl.TunnelConfig(acct, r.TunnelID)
		if err != nil {
			return err
		}
		var next []ingress.Rule
		if start {
			next, err = ingress.Add(rules, host, fmt.Sprintf("http://127.0.0.1:%d", r.Port))
			if err == ingress.ErrExists {
				r.State = "running"
				return c.Save()
			}
		} else {
			next, err = ingress.Remove(rules, host)
			if err == ingress.ErrNotFound {
				r.State = "stopped"
				return c.Save()
			}
		}
		if err != nil {
			return err
		}
		if err := cl.PutIngress(acct, r.TunnelID, full, next); err != nil {
			return err
		}
		r.State = map[bool]string{true: "running", false: "stopped"}[start]
		fmt.Printf("%s: %s\n", host, r.State)
		return c.Save()
	}
	for _, a := range c.Adopted {
		if a.Tunnel == target || a.Label == target {
			if cls := classify(c, a.Tunnel, a.TunnelID); cls != "adopted" {
				return fmt.Errorf("%s is %s: refusing to %s", target, cls, verb)
			}
			for _, j := range disc.Jobs() {
				if j.Label == a.Label {
					if start {
						return exec.Command("launchctl", "bootstrap", domain(), j.Plist).Run()
					}
					return exec.Command("launchctl", "bootout", domain()+"/"+j.Label).Run()
				}
			}
			return fmt.Errorf("no launchd job found for adopted tunnel %s", a.Tunnel)
		}
	}
	return fmt.Errorf("%q is not a cfex-managed route: refusing to %s. `cfex list` shows what is managed; protected, client and unmanaged tunnels are never modified", target, verb)
}
