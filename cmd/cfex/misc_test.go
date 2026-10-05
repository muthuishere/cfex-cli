package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"errors"
	cfexcli "github.com/muthuishere/cfex-cli"
	"github.com/muthuishere/cfex-cli/internal/cfd"
	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

func fakeHome(t *testing.T) string {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("CFEX_CONFIG", filepath.Join(h, "cfg", "config.yaml"))
	return h
}

func TestSkillInstallAndUninstall(t *testing.T) {
	h := fakeHome(t)
	if err := cmdSkill(cfg.Default(), []string{"install"}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{".claude/skills/cfex", ".agents/skills/cfex"} {
		b, err := os.ReadFile(filepath.Join(h, d, "SKILL.md"))
		if err != nil || string(b) != string(cfexcli.Skill) {
			t.Fatalf("%s: %v", d, err)
		}
	}
	if !strings.Contains(string(cfexcli.Skill), "cfex dev.yourdomain.com:3000") || !strings.HasPrefix(string(cfexcli.Skill), "---\nname: cfex\n") {
		t.Fatal("skill must lead with the instant examples and a trigger line")
	}
	if err := cmdSkill(cfg.Default(), []string{"uninstall"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h, ".claude/skills/cfex")); err == nil {
		t.Fatal("skill not removed")
	}
}

func TestUninstallRemovesBinaryAndSkillButNotCloudflaredFiles(t *testing.T) {
	h := fakeHome(t)
	isTTY = func() bool { return false } // no prompts: everything optional stays
	bin := filepath.Join(h, "cfex")
	os.WriteFile(bin, []byte("x"), 0o755)
	selfPath = func() (string, error) { return bin, nil }
	cmdSkill(cfg.Default(), []string{"install"})
	os.MkdirAll(filepath.Join(h, ".cloudflared"), 0o700)
	os.WriteFile(filepath.Join(h, ".cloudflared", "cert.pem"), []byte("c"), 0o600)
	c := cfg.Default()
	c.Save()
	if err := cmdUninstall(c, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err == nil {
		t.Fatal("binary not removed")
	}
	if _, err := os.Stat(filepath.Join(h, ".cloudflared", "cert.pem")); err != nil {
		t.Fatal("cloudflared login must survive uninstall unless asked")
	}
	if _, err := os.Stat(cfg.Path()); err != nil {
		t.Fatal("config must survive uninstall unless asked")
	}
	if err := cmdUninstall(c, []string{"--purge-config", "--purge-cloudflared"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h, ".cloudflared")); err == nil {
		t.Fatal("--purge-cloudflared must remove it")
	}
}

func TestTunnelFileRoundTripKeepsOtherRoutes(t *testing.T) {
	fakeHome(t)
	rules, _ := ingress.Add(nil, "a.example.com", "http://127.0.0.1:1")
	if err := saveRules("TID", rules); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadRules()
	if err != nil {
		t.Fatal(err)
	}
	next, _ := ingress.Add(loaded, "b.example.com", "http://127.0.0.1:2")
	saveRules("TID", next)
	again, _ := loadRules()
	if s, _ := ingress.Service(again, "a.example.com"); s != "http://127.0.0.1:1" || !ingress.Has(again, "b.example.com") || !ingress.IsCatchAll(again[len(again)-1]) {
		t.Fatalf("%v", again)
	}
	if r := routeRule("h.example.com", "https://127.0.0.1:1", true, false); r["originRequest"] == nil {
		t.Fatal("https origins skip verification unless --verify-cert")
	}
	if r := routeRule("h.example.com", "https://127.0.0.1:1", true, true); r["originRequest"] != nil {
		t.Fatal("--verify-cert keeps verification on")
	}
}

func TestConfirmDeleteDefaultsToNo(t *testing.T) {
	for in, want := range map[string]bool{"y\n": true, "Y\n": true, "yes\n": true, "\n": false, "n\n": false, "": false, "maybe\n": false} {
		var out bytes.Buffer
		if got := confirmDelete(strings.NewReader(in), &out, 2); got != want {
			t.Fatalf("%q -> %v", in, got)
		}
		if !strings.Contains(out.String(), "delete these 2 tunnels? (y/N)") {
			t.Fatal(out.String())
		}
	}
}

func TestDetectHTTPS(t *testing.T) {
	tls := httptest.NewTLSServer(http.NewServeMux())
	defer tls.Close()
	plain := httptest.NewServer(http.NewServeMux())
	defer plain.Close()
	port := func(s *httptest.Server) int { u, _ := url.Parse(s.URL); p, _ := strconv.Atoi(u.Port()); return p }
	if !detectHTTPS(port(tls)) || detectHTTPS(port(plain)) {
		t.Fatal("must detect a TLS service and not a plain one")
	}
}

func TestWaitUntilTimesOutAndConfirms(t *testing.T) {
	if !waitUntil(time.Second, time.Millisecond, "x", func() bool { return true }) {
		t.Fatal("immediate success")
	}
	if waitUntil(30*time.Millisecond, 5*time.Millisecond, "x", func() bool { return false }) {
		t.Fatal("must time out")
	}
}

func TestInstantReusesALeftoverTunnel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeInstant{existing: &cfd.Tunnel{ID: "OLD", Name: "cfex-instant-dev-example-com"}, cancel: cancel, cancelOnRun: true, routeErr: errors.New("already routed")}
	var out bytes.Buffer
	if err := instantSession(ctx, f, "dev.example.com", &out); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "create") {
			t.Fatal("must reuse the leftover tunnel, not create another")
		}
	}
	if !strings.Contains(out.String(), "Found existing tunnel") || !strings.Contains(out.String(), "Warning: DNS route creation failed") {
		t.Fatal(out.String())
	}
}

func TestUsageCoversEveryCommand(t *testing.T) {
	for name := range subcommands {
		if name == "ls" || name == "rm" {
			continue
		}
		if !strings.Contains(usage, "cfex "+name) && !strings.Contains(usage, name) {
			t.Fatalf("usage does not mention %s", name)
		}
	}
	for _, must := range []string{"--https", "--verify-cert", "--keep", "-v, --version", "Examples:", "cloudflared tunnel login"} {
		if !strings.Contains(usage, must) {
			t.Fatalf("usage lacks %q", must)
		}
	}
	if !strings.Contains(troubleshoot, "WSL") {
		t.Fatal("troubleshooting keeps the WSL note")
	}
}
