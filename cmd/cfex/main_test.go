package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

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

func TestLegacyParse(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		host  string
		port  int
		https bool
		ok    bool
	}{
		{[]string{"dev.example.com:3000"}, "dev.example.com", 3000, false, true},
		{[]string{"dev.example.com", "8080"}, "dev.example.com", 8080, false, true},
		{[]string{"dev.example.com:8443", "--https", "--verify-cert"}, "dev.example.com", 8443, true, true},
		{[]string{"--https", "Dev.Example.com:1"}, "dev.example.com", 1, true, true},
		{[]string{"dev.example.com:0"}, "", 0, false, false},
		{[]string{"dev.example.com:70000"}, "", 0, false, false},
		{[]string{"justahost"}, "", 0, false, false},
		{[]string{"a.b", "x"}, "", 0, false, false},
		{[]string{"--bogus", "a.b:1"}, "", 0, false, false},
	} {
		a, err := legacyParse(tc.args)
		if (err == nil) != tc.ok {
			t.Fatalf("%v: err=%v", tc.args, err)
		}
		if tc.ok && (a.Host != tc.host || a.Port != tc.port || a.HTTPS != tc.https) {
			t.Fatalf("%v: got %+v", tc.args, a)
		}
	}
}

func TestLegacyConfigMatchesV1(t *testing.T) {
	http := legacyConfig(false, 3000, "ID", true)
	if !strings.Contains(http, "url: http://localhost:3000\ntunnel: ID\n") || strings.Contains(http, "originRequest") {
		t.Fatal(http)
	}
	if s := legacyConfig(true, 8443, "ID", true); !strings.Contains(s, "url: https://localhost:8443") || !strings.Contains(s, "noTLSVerify: true") || !strings.Contains(s, "disableChunkedEncoding: true") {
		t.Fatal(s)
	}
	if s := legacyConfig(true, 8443, "ID", false); strings.Contains(s, "noTLSVerify") {
		t.Fatal("--verify-cert must not disable verification")
	}
	if legacyName("dev.example.com") != "tunnel_dev_example_com" {
		t.Fatal("v1 tunnel naming must be unchanged")
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
	body := serviceBody(c, []string{"/usr/bin/cloudflared", "tunnel", "run", "--token-file", "/x/connector.token"})
	if strings.Contains(strings.ToLower(body), "eyj") || !strings.Contains(body, "connector.token") {
		t.Fatal("unit must reference the token FILE, never embed a token")
	}
}

func TestOnlyManagedStopStartAllowed(t *testing.T) {
	c := cfg.Default()
	c.Protected = []string{"billing"}
	if err := stopStart(c, []string{"billing"}, false); err == nil {
		t.Fatal("must refuse a tunnel that is not a cfex route")
	}
}
