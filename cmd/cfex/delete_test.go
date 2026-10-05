package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/muthuishere/cfex-cli/internal/cf"
	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

type fake struct {
	dns     cf.DNS
	rules   []ingress.Rule
	puts    int
	deleted []string
}

func (f *fake) DNSGet(z, id string) (cf.DNS, error) { return f.dns, nil }
func (f *fake) DNSDelete(z, id string) error        { f.deleted = append(f.deleted, id); return nil }
func (f *fake) TunnelConfig(a, t string) (map[string]any, []ingress.Rule, error) {
	return map[string]any{}, f.rules, nil
}
func (f *fake) PutIngress(a, t string, full map[string]any, r []ingress.Rule) error {
	f.puts++
	f.rules = r
	return nil
}

func setup(t *testing.T) (*cfg.Config, *fake) {
	t.Setenv("CFEX_CONFIG", t.TempDir()+"/config.yaml")
	c := cfg.Default()
	c.Routes = []cfg.Route{{Host: "t.example.com", Port: 9, ZoneID: "z", DNSID: "d1", TunnelID: "T", State: "running"}}
	f := &fake{dns: cf.DNS{Comment: comment}, rules: []ingress.Rule{
		{"hostname": "other.example.com", "service": "http://127.0.0.1:1"},
		{"hostname": "t.example.com", "service": "http://127.0.0.1:9"},
		{"service": ingress.CatchAll}}}
	return c, f
}

func TestDeleteDryRunChangesNothing(t *testing.T) {
	c, f := setup(t)
	var out bytes.Buffer
	if err := deleteRoute(f, c, "a", "t.example.com", false, &out); err != nil {
		t.Fatal(err)
	}
	if f.puts != 0 || len(f.deleted) != 0 || c.Route("t.example.com") == nil {
		t.Fatal("dry run mutated something")
	}
	if !strings.Contains(out.String(), "DRY RUN") {
		t.Fatal(out.String())
	}
}

func TestDeleteRefusesUnmanagedHost(t *testing.T) {
	c, f := setup(t)
	if err := deleteRoute(f, c, "a", "os.example.com", true, &bytes.Buffer{}); err == nil || f.puts != 0 || len(f.deleted) != 0 {
		t.Fatalf("must refuse a route cftunnel did not create: %v", err)
	}
}

func TestDeleteRefusesDNSWithoutManagedComment(t *testing.T) {
	c, f := setup(t)
	f.dns.Comment = "someone else's"
	if err := deleteRoute(f, c, "a", "t.example.com", true, &bytes.Buffer{}); err == nil || len(f.deleted) != 0 {
		t.Fatalf("must refuse DNS record lacking the comment: %v", err)
	}
}

func TestDeleteApplyLeavesOtherRoutes(t *testing.T) {
	c, f := setup(t)
	if err := deleteRoute(f, c, "a", "t.example.com", true, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 1 || ingress.Has(f.rules, "t.example.com") || !ingress.Has(f.rules, "other.example.com") {
		t.Fatalf("bad result: %v %v", f.deleted, f.rules)
	}
}

func TestClassifyProtected(t *testing.T) {
	c := cfg.Default()
	c.Protected = []string{"billing", "pay-*"}
	c.Client = []string{"acme-demo"}
	for _, n := range []string{"billing", "pay-gateway"} {
		if classify(c, n, "") != "PROTECTED" {
			t.Fatal(n)
		}
	}
	if classify(c, "acme-demo", "") != "client" || classify(c, "random", "") != "unmanaged" ||
		classify(c, "tunnel_dev_example_com", "") != "legacy" || classify(c, c.TunnelName, "") != "managed" {
		t.Fatal("classes")
	}
	if classify(cfg.Default(), "billing", "") != "unmanaged" {
		t.Fatal("nothing is protected unless the user's config says so")
	}
}

func TestProtectedRouteNeedsCeoGo(t *testing.T) {
	c, _ := setup(t)
	c.Routes[0].Protected = true
	if err := protectedGuard(c, "t.example.com", false); err == nil {
		t.Fatal("protected route must need --ceo-go")
	}
}
