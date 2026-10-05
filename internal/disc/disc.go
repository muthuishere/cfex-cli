// Package disc is read-only local discovery: launchd jobs, ~/.cloudflared scripts and token-file
// NAMES. It never prints a token value; the tunnel id inside a token is read in memory only.
package disc

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Job struct {
	Label       string // launchd label
	Plist       string
	Loaded      bool
	PID         int
	LastExit    string
	Script      string // script path run by the plist, if any
	TokenFile   string // token file NAME (no value)
	TunnelID    string // decoded in memory from the token file; "" if unknown
	Disabled    bool
	ForwardPort string // local port of an ssh -L forward ("" for cloudflared jobs)
}

var fwdRe = regexp.MustCompile(`<string>(?:127\.0\.0\.1:)?(\d+):[^<]+</string>`)
var tokRe = regexp.MustCompile(`([A-Za-z0-9_.-]+-token)\b`)
var scriptRe = regexp.MustCompile(`<string>([^<>\s]+\.sh)</string>`)

// TunnelIDFromToken decodes a connector token (base64 JSON {"a","t","s"}) and returns only "t".
func TunnelIDFromToken(tok string) string {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(tok))
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(tok))
		if err != nil {
			return ""
		}
	}
	var m struct {
		T string `json:"t"`
	}
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return m.T
}

func launchctlState() map[string][2]string { // label -> pid, status
	out, _ := exec.Command("launchctl", "list").Output()
	m := map[string][2]string{}
	for _, ln := range strings.Split(string(out), "\n")[1:] {
		f := strings.Fields(ln)
		if len(f) == 3 {
			m[f[2]] = [2]string{f[0], f[1]}
		}
	}
	return m
}

func Jobs() []Job {
	home, _ := os.UserHomeDir()
	st := launchctlState()
	var jobs []Job
	plists, _ := filepath.Glob(filepath.Join(home, "Library", "LaunchAgents", "*"))
	for _, p := range plists {
		base := filepath.Base(p)
		if !strings.HasSuffix(base, ".plist") && !strings.HasSuffix(base, ".plist.disabled") {
			continue // skips editor/backup copies such as *.plist.bak-1005
		}
		if !strings.Contains(base, "tunnel") && !strings.Contains(base, "cloudflared") {
			continue
		}
		b, _ := os.ReadFile(p)
		// A plist can embed a token in its arguments; only the script path and token-file NAME are extracted.
		txt := string(b)
		j := Job{Plist: p, Disabled: strings.HasSuffix(base, ".disabled")}
		j.Label = strings.TrimSuffix(strings.TrimSuffix(base, ".disabled"), ".plist")
		if m := scriptRe.FindStringSubmatch(txt); m != nil {
			j.Script = strings.ReplaceAll(strings.Replace(m[1], "~", home, 1), "$HOME", home)
		}
		if strings.Contains(txt, "/usr/bin/ssh") {
			if m := fwdRe.FindStringSubmatch(txt); m != nil {
				j.ForwardPort = m[1]
			}
		}
		if s, ok := st[j.Label]; ok {
			j.Loaded = true
			j.PID, _ = strconv.Atoi(s[0])
			j.LastExit = s[1]
		}
		if j.Script != "" {
			if sb, err := os.ReadFile(j.Script); err == nil {
				if m := tokRe.FindString(string(sb)); m != "" {
					j.TokenFile = m
					if tb, err := os.ReadFile(filepath.Join(home, ".cloudflared", m)); err == nil {
						j.TunnelID = TunnelIDFromToken(string(tb))
					}
				}
			}
		}
		jobs = append(jobs, j)
	}
	return jobs
}

// Scripts lists ~/.cloudflared/*-tunnel.sh and token-file NAMES (never contents).
func Scripts() (scripts, tokenFiles []string) {
	home, _ := os.UserHomeDir()
	s, _ := filepath.Glob(filepath.Join(home, ".cloudflared", "*-tunnel.sh"))
	t, _ := filepath.Glob(filepath.Join(home, ".cloudflared", "*-token"))
	for _, x := range s {
		scripts = append(scripts, filepath.Base(x))
	}
	for _, x := range t {
		tokenFiles = append(tokenFiles, filepath.Base(x))
	}
	return
}

// RunningCloudflaredCount counts processes without ever printing their arguments.
func RunningCloudflaredCount() int {
	out, _ := exec.Command("pgrep", "-f", "cloudflared").Output()
	n := 0
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			n++
		}
	}
	return n
}

// ShortName strips the launchd prefix/suffix so a label can be compared with a tunnel name.
func ShortName(label string) string {
	l := label
	if i := strings.LastIndex(l, "."); i >= 0 && i < len(l)-1 {
		l = l[i+1:]
	}
	return strings.TrimSuffix(l, "-tunnel")
}
