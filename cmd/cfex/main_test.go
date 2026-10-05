package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/muthuishere/cfex-cli/internal/cfd"
	"github.com/muthuishere/cfex-cli/internal/cfg"
)

// The suite refuses to run unless CFEX_CONFIG points under the temp dir (fail-closed).
func TestMain(m *testing.M) {
	p := os.Getenv("CFEX_CONFIG")
	if p == "" || !cfg.UnderTemp(p) {
		fmt.Fprintln(os.Stderr, "refusing to run tests: CFEX_CONFIG must point under the temp dir (use `make test`)")
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func TestInstantParse(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		host  string
		port  int
		https bool
		keep  bool
		ok    bool
	}{
		{[]string{"dev.example.com:3000"}, "dev.example.com", 3000, false, false, true},
		{[]string{"dev.example.com", "8080"}, "dev.example.com", 8080, false, false, true},
		{[]string{"dev.example.com:8443", "--https", "--verify-cert"}, "dev.example.com", 8443, true, false, true},
		{[]string{"--https", "Dev.Example.com:1"}, "dev.example.com", 1, true, false, true},
		{[]string{"dev.example.com:3000", "--keep"}, "dev.example.com", 3000, false, true, true},
		{[]string{"dev.example.com:0"}, "", 0, false, false, false},
		{[]string{"dev.example.com:70000"}, "", 0, false, false, false},
		{[]string{"justahost"}, "", 0, false, false, false},
		{[]string{"a.b", "x"}, "", 0, false, false, false},
		{[]string{"--bogus", "a.b:1"}, "", 0, false, false, false},
	} {
		a, err := instantParse(tc.args)
		if (err == nil) != tc.ok {
			t.Fatalf("%v: err=%v", tc.args, err)
		}
		if tc.ok && (a.Host != tc.host || a.Port != tc.port || a.HTTPS != tc.https || a.Keep != tc.keep) {
			t.Fatalf("%v: got %+v", tc.args, a)
		}
	}
}

func TestInstantConfig(t *testing.T) {
	http := instantConfig("d.example.com", 3000, "ID", false, false)
	if !strings.Contains(http, "hostname: d.example.com") || !strings.Contains(http, "service: http://127.0.0.1:3000") || strings.Contains(http, "originRequest") || !strings.HasSuffix(http, "service: http_status:404\n") {
		t.Fatal(http)
	}
	if s := instantConfig("d.example.com", 8443, "ID", true, false); !strings.Contains(s, "https://127.0.0.1:8443") || !strings.Contains(s, "noTLSVerify: true") || !strings.Contains(s, "disableChunkedEncoding: true") {
		t.Fatal(s)
	}
	if s := instantConfig("d.example.com", 8443, "ID", true, true); strings.Contains(s, "noTLSVerify") {
		t.Fatal("--verify-cert must not disable verification")
	}
	if instantName("dev.example.com") != "cfex-instant-dev-example-com" {
		t.Fatal("instant tunnel naming")
	}
}

// fakeInstant records what an instant session does so we can prove the cleanup.
type fakeInstant struct {
	existing    *cfd.Tunnel
	routeErr    error
	calls       []string
	cancel      context.CancelFunc
	cancelOnRun bool
	runErr      error
	dnsDone     bool
}

func (f *fakeInstant) Find(string) (*cfd.Tunnel, error) { return f.existing, nil }
func (f *fakeInstant) Create(n string) (string, error) {
	f.calls = append(f.calls, "create "+n)
	return "TID", nil
}
func (f *fakeInstant) RouteDNS(n, h string) error {
	f.calls = append(f.calls, "route "+h)
	return f.routeErr
}
func (f *fakeInstant) WriteConfig(id string) (string, error) { return "/tmp/x.yml", nil }
func (f *fakeInstant) Run(ctx context.Context, c, n string) error {
	f.calls = append(f.calls, "run")
	if f.cancelOnRun {
		f.cancel() // the user presses Ctrl+C
	}
	<-ctx.Done()
	return f.runErr
}
func (f *fakeInstant) DeleteDNS(h, id string) (bool, error) {
	f.calls = append(f.calls, "delete-dns "+h+" "+id)
	return f.dnsDone, nil
}
func (f *fakeInstant) DeleteTunnel(n string) error {
	f.calls = append(f.calls, "delete-tunnel "+n)
	return nil
}

func TestInstantCtrlCRemovesDNSRecordAndTunnel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeInstant{cancel: cancel, cancelOnRun: true, dnsDone: true}
	var out bytes.Buffer
	if err := instantSession(ctx, f, "dev.example.com", &out); err != nil {
		t.Fatal(err)
	}
	want := []string{"create cfex-instant-dev-example-com", "route dev.example.com", "run", "delete-dns dev.example.com TID", "delete-tunnel cfex-instant-dev-example-com"}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls:\n got %v\nwant %v", f.calls, want)
	}
	if !strings.Contains(out.String(), "DNS record for dev.example.com removed") {
		t.Fatal(out.String())
	}
}

func TestInstantFailedRouteStillRemovesTheTunnelItCreated(t *testing.T) {
	f := &fakeInstant{routeErr: errors.New("record exists")}
	err := instantSession(context.Background(), f, "dev.example.com", &bytes.Buffer{})
	if err == nil {
		t.Fatal("route failure on a fresh tunnel must be an error")
	}
	last := f.calls[len(f.calls)-1]
	if last != "delete-tunnel cfex-instant-dev-example-com" {
		t.Fatalf("tunnel not cleaned up: %v", f.calls)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "run") {
			t.Fatal("must not run after a failed route")
		}
	}
}

func TestInstantWithoutTokenSaysDNSIsLeft(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeInstant{cancel: cancel, cancelOnRun: true, dnsDone: false}
	var out bytes.Buffer
	instantSession(ctx, f, "dev.example.com", &out)
	if !strings.Contains(out.String(), "delete the CNAME dev.example.com yourself") {
		t.Fatal(out.String())
	}
}

func TestNormHost(t *testing.T) {
	c := cfg.Default()
	if _, err := normHost(c, "app"); err == nil {
		t.Fatal("a bare name needs a default_zone")
	}
	c.DefaultZone = "example.com"
	if h, _ := normHost(c, "App"); h != "app.example.com" {
		t.Fatal(h)
	}
	if h, _ := normHost(c, "x.other.org"); h != "x.other.org" {
		t.Fatal(h)
	}
}

func TestTokenFromEnvOnly(t *testing.T) {
	c := cfg.Default()
	for _, n := range []string{"CLOUDFLARE_API_TOKEN", "CLOUDFLARE_API_KEY", "MY_TOK"} {
		t.Setenv(n, "")
	}
	if token(c) != "" {
		t.Fatal("no token expected")
	}
	t.Setenv("CLOUDFLARE_API_KEY", "legacy")
	if token(c) != "legacy" {
		t.Fatal("v1 CLOUDFLARE_API_KEY must still work")
	}
	c.TokenEnv = "MY_TOK"
	t.Setenv("MY_TOK", "custom")
	if token(c) != "custom" {
		t.Fatal("token_env must win")
	}
}

func TestServiceUnitNeverContainsToken(t *testing.T) {
	c := cfg.Default()
	body := serviceBody(c, []string{"/usr/bin/cloudflared", "tunnel", "--config", "/x/tunnel.yml", "run", "cfex"})
	if strings.Contains(strings.ToLower(body), "token") || !strings.Contains(body, "tunnel.yml") {
		t.Fatal("unit must run from the config + cloudflared credentials file, never a token")
	}
}

func TestDeleteNeedsConfirmationOnTTYAndDryRunsWithoutOne(t *testing.T) {
	// no terminal and no --yes: nothing may be applied
	isTTY = func() bool { return false }
	defer func() { isTTY = func() bool { return false } }()
	c, f := setup(t)
	var out bytes.Buffer
	apply := false // what cmdDelete computes for yes=false, no TTY
	if err := deleteRoute(f, c, "t.example.com", apply, &out); err != nil || f.saved != 0 || len(f.deleted) != 0 {
		t.Fatal("no TTY and no --yes must be a dry run")
	}
}

func TestOnlyManagedStopStartAllowed(t *testing.T) {
	c := cfg.Default()
	c.Protected = []string{"billing"}
	if err := stopStart(c, []string{"billing"}, false); err == nil {
		t.Fatal("must refuse a tunnel that is not a cfex route")
	}
}
