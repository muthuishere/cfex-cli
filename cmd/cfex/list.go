package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/disc"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

type Row struct {
	Host    string `json:"host"`
	Port    string `json:"port"`
	Tunnel  string `json:"tunnel"`
	Class   string `json:"class"`
	Run     string `json:"run"`
	HTTP    string `json:"http"`
	Local   string `json:"local"` // service (launchd label) on this machine, if any
	Account string `json:"account"`
}

func cmdList(c *cfg.Config, args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	noHTTP := fs.Bool("no-http", false, "skip live HTTP checks")
	fs.Parse(args)
	cl, err := client(c)
	if err != nil { // v1 behaviour: no API token, just what cloudflared knows
		fmt.Fprintln(os.Stderr, "note:", err, "\nshowing cloudflared's own tunnel list only (cfex v1 view)")
		return legacyList()
	}
	accts, err := cl.Accounts()
	if err != nil {
		return err
	}
	jobs := disc.Jobs()
	byID := map[string]disc.Job{}
	for _, j := range jobs {
		if j.TunnelID != "" {
			byID[j.TunnelID] = j
		}
	}
	var rows []Row
	for _, a := range accts {
		ts, err := cl.Tunnels(a.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cfex: tunnels of %q: %v\n", a.Name, err)
			continue
		}
		type res struct{ rules []ingress.Rule }
		out := make([]res, len(ts))
		var wg sync.WaitGroup
		for i := range ts {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, r, _ := cl.TunnelConfig(a.ID, ts[i].ID)
				out[i] = res{r}
			}(i)
		}
		wg.Wait()
		for i, t := range ts {
			class := classify(c, t.Name, t.ID)
			run := "down"
			if t.Status == "healthy" {
				run = "running"
			} else if t.Status == "degraded" {
				run = "degraded"
			}
			local := "-"
			j, ok := byID[t.ID]
			if !ok { // no token-derived match: fall back to launchd label naming
				for _, jj := range jobs {
					if sn := disc.ShortName(jj.Label); jj.TunnelID == "" && jj.ForwardPort == "" && (sn == t.Name || sn+"-demo" == t.Name || sn == strings.TrimSuffix(t.Name, "-demo")) {
						j, ok = jj, true
					}
				}
			}
			if ok {
				local = j.Label
				if !j.Loaded {
					local += " (not loaded)"
				}
				if j.Disabled {
					local += " (disabled)"
				}
			}
			n := 0
			for _, r := range out[i].rules {
				h, _ := r["hostname"].(string)
				svc, _ := r["service"].(string)
				if h == "" && strings.HasPrefix(svc, "http_status") {
					continue
				}
				if h == "" {
					h = "(catch-all)"
					if class == "legacy" { // a v1 tunnel: its hostname is encoded in the name
						h = strings.ReplaceAll(strings.TrimPrefix(t.Name, "tunnel_"), "_", ".")
					}
				} else if pth, _ := r["path"].(string); pth != "" {
					h += pth
				}
				rows = append(rows, Row{h, portOf(svc), t.Name, class, run, "", local, a.Name})
				n++
			}
			if n == 0 {
				h := "(no routes)"
				if class == "legacy" {
					h = strings.ReplaceAll(strings.TrimPrefix(t.Name, "tunnel_"), "_", ".")
				}
				rows = append(rows, Row{h, "-", t.Name, class, run, "", local, a.Name})
			}
		}
	}
	// managed routes that are stopped have no ingress rule: show them from config
	for _, r := range c.Routes {
		if r.State == "stopped" {
			rows = append(rows, Row{r.Host, fmt.Sprint(r.Port), c.TunnelName, "managed", "stopped", "", "-", ""})
		}
	}
	for _, j := range jobs { // local jobs that matched no account tunnel
		used := false
		for _, r := range rows {
			if strings.HasPrefix(r.Local, j.Label) {
				used = true
			}
		}
		if used {
			continue
		}
		if j.ForwardPort != "" { // an ssh -L forward: judged by a live check, not by its log
			p, _ := strconv.Atoi(j.ForwardPort)
			run := "no answer"
			if listening(p) {
				run = "answers"
			}
			rows = append(rows, Row{"(ssh-forward)", j.ForwardPort, "-", classify(c, disc.ShortName(j.Label), ""), run, "-", j.Label, ""})
			continue
		}
		state, run := "not loaded", "stopped"
		if j.Loaded {
			state = "loaded, last exit " + j.LastExit
		}
		if j.Disabled {
			run, state = "stale", "disabled plist"
		}
		rows = append(rows, Row{"(no matching tunnel)", "-", "-", classify(c, disc.ShortName(j.Label), ""), run, "-", j.Label + " (" + state + ")", ""})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Host < rows[j].Host })
	if !*noHTTP {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 12)
		for i := range rows {
			if strings.HasPrefix(rows[i].Host, "(") || rows[i].Host == "*" || rows[i].HTTP != "" {
				if rows[i].HTTP == "" {
					rows[i].HTTP = "-"
				}
				continue
			}
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sem <- struct{}{}
				h := rows[i].Host
				if k := strings.Index(h, "/"); k >= 0 { // path rule: check the hostname
					h = h[:k]
				}
				rows[i].HTTP = httpStatus("https://" + h)
				<-sem
			}(i)
		}
		wg.Wait()
	} else {
		for i := range rows {
			if rows[i].HTTP == "" {
				rows[i].HTTP = "-"
			}
		}
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"routes": rows, "cloudflared_processes": disc.RunningCloudflaredCount()})
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "HOST\tPORT\tTUNNEL\tCLASS\tRUN\tHTTP\tLOCAL SERVICE")
	hostCount := map[string]int{}
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Host, r.Port, r.Tunnel, r.Class, r.Run, r.HTTP, r.Local)
		hostCount[r.Host]++
	}
	tw.Flush()
	scripts, toks := disc.Scripts()
	fmt.Printf("\n%d rows · %d cloudflared processes · %d launchd tunnel jobs · %d ~/.cloudflared scripts · %d plaintext token files (names only; `cfex doctor`)\n",
		len(rows), disc.RunningCloudflaredCount(), len(jobs), len(scripts), len(toks))
	var dups []string
	for h, n := range hostCount {
		if n > 1 && !strings.HasPrefix(h, "(") && h != "*" {
			dups = append(dups, h)
		}
	}
	sort.Strings(dups)
	if len(dups) > 0 {
		fmt.Println("hostnames claimed by more than one rule/tunnel:", strings.Join(dups, ", "))
	}
	return nil
}

// legacyList prints cloudflared's own tunnels the way cfex v1 did.
func legacyList() error {
	ts, err := localTunnels()
	if err != nil {
		return err
	}
	fmt.Println("Active Tunnels:\n----------------------------------------")
	if len(ts) == 0 {
		fmt.Println("No active tunnels found.")
	}
	for _, t := range ts {
		n := t.Name
		if strings.HasPrefix(n, "tunnel_") {
			n = strings.ReplaceAll(strings.TrimPrefix(n, "tunnel_"), "_", ".")
		}
		fmt.Printf("%-40s %s\n", n, t.ID)
	}
	return nil
}
