package main

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/muthuishere/cfex-cli/internal/cfd"
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

// tinfo is one tunnel from whichever source is available.
type tinfo struct {
	ID, Name, Account string
	Running           bool
	Rules             []ingress.Rule
}

// localRules finds ingress rules for a tunnel id in cfex's own files and in ~/.cloudflared/*.yml.
func localRules(c *cfg.Config, id string) []ingress.Rule {
	files, _ := filepath.Glob(filepath.Join(cloudflaredDir(), "*.yml"))
	more, _ := filepath.Glob(filepath.Join(cfg.StateDir(), "instant", "*.yml"))
	files = append(append(files, more...), cfg.TunnelFile())
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var doc struct {
			Tunnel  string           `yaml:"tunnel"`
			URL     string           `yaml:"url"`
			Ingress []map[string]any `yaml:"ingress"`
		}
		if yaml.Unmarshal(b, &doc) != nil || doc.Tunnel != id {
			continue
		}
		var rules []ingress.Rule
		for _, r := range doc.Ingress {
			rules = append(rules, r)
		}
		if doc.URL != "" {
			rules = append(rules, ingress.Rule{"hostname": "(see " + filepath.Base(f) + ")", "service": doc.URL})
		}
		return rules
	}
	return nil
}

func cloudflaredDir() string { h, _ := os.UserHomeDir(); return filepath.Join(h, ".cloudflared") }

// allTunnels lists every tunnel the account has: through cloudflared when it is logged in, otherwise through the API
// when a token is available. The API (if present) also supplies remote ingress for remotely-managed tunnels.
func allTunnels(c *cfg.Config) ([]tinfo, error) {
	api := apiClient(c)
	var out []tinfo
	if cfd.HasCert() {
		ts, err := cfd.List()
		if err != nil {
			return nil, err
		}
		acct := ""
		if api != nil {
			acct, _ = pickAccount(c, api)
		}
		for _, t := range ts {
			ti := tinfo{ID: t.ID, Name: t.Name, Running: t.Running(), Rules: localRules(c, t.ID)}
			if len(ti.Rules) == 0 && api != nil && acct != "" {
				_, ti.Rules, _ = api.TunnelConfig(acct, t.ID)
			}
			out = append(out, ti)
		}
		return out, nil
	}
	if api == nil {
		return nil, fmt.Errorf("cannot list tunnels: cloudflared is not logged in (run `cloudflared tunnel login`) and there is no API token (CLOUDFLARE_API_TOKEN)")
	}
	accts, err := api.Accounts()
	if err != nil {
		return nil, err
	}
	for _, a := range accts {
		ts, err := api.Tunnels(a.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cfex: tunnels of %q: %v\n", a.Name, err)
			continue
		}
		res := make([][]ingress.Rule, len(ts))
		var wg sync.WaitGroup
		for i := range ts {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, res[i], _ = api.TunnelConfig(a.ID, ts[i].ID)
				if len(res[i]) == 0 {
					res[i] = localRules(c, ts[i].ID)
				}
			}(i)
		}
		wg.Wait()
		for i, t := range ts {
			out = append(out, tinfo{ID: t.ID, Name: t.Name, Account: a.Name, Running: t.Status == "healthy", Rules: res[i]})
		}
	}
	return out, nil
}

func cmdList(c *cfg.Config, args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	noHTTP := fs.Bool("no-http", false, "skip live HTTP checks")
	fs.Parse(args)
	tunnels, err := allTunnels(c)
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
	for _, t := range tunnels {
		class := classify(c, t.Name, t.ID)
		run := "down"
		if t.Running {
			run = "running"
		}
		local := "-"
		j, ok := byID[t.ID]
		if !ok && t.Name == c.TunnelName {
			for _, jj := range jobs {
				if jj.Label == c.ServiceLabel {
					j, ok = jj, true
				}
			}
		}
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
		for _, r := range t.Rules {
			h, _ := r["hostname"].(string)
			svc, _ := r["service"].(string)
			if h == "" && strings.HasPrefix(svc, "http_status") {
				continue
			}
			if h == "" {
				h = "(catch-all)"
			} else if pth, _ := r["path"].(string); pth != "" {
				h += pth
			}
			rows = append(rows, Row{h, portOf(svc), t.Name, class, run, "", local, t.Account})
			n++
		}
		if n == 0 {
			h := "(no routes known)"
			if class == "instant" {
				h = strings.ReplaceAll(strings.TrimPrefix(t.Name, instantPrefix), "-", ".")
			}
			rows = append(rows, Row{h, "-", t.Name, class, run, "", local, t.Account})
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
