package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/muthuishere/cfex-cli/internal/cf"
	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

type fakeRoute struct {
	api     bool
	dns     cf.DNS
	rules   []ingress.Rule
	saved   int
	deleted []string
	reloads int
}

func (f *fakeRoute) HasAPI() bool                        { return f.api }
func (f *fakeRoute) DNSGet(z, id string) (cf.DNS, error) { return f.dns, nil }
func (f *fakeRoute) DNSDelete(z, id string) error        { f.deleted = append(f.deleted, id); return nil }
func (f *fakeRoute) LoadRules() ([]ingress.Rule, error)  { return f.rules, nil }
func (f *fakeRoute) Reload() error                       { f.reloads++; return nil }
func (f *fakeRoute) SaveRules(id string, r []ingress.Rule) error {
	f.saved++
	f.rules = r
	return nil
}

func setup(t *testing.T) (*cfg.Config, *fakeRoute) {
	t.Setenv("CFEX_CONFIG", t.TempDir()+"/config.yaml")
	c := cfg.Default()
	c.Routes = []cfg.Route{{Host: "t.example.com", Port: 9, ZoneID: "z", DNSID: "d1", TunnelID: "T", State: "running"}}
	f := &fakeRoute{api: true, dns: cf.DNS{Comment: comment}, rules: []ingress.Rule{
		{"hostname": "other.example.com", "service": "http://127.0.0.1:1"},
		{"hostname": "t.example.com", "service": "http://127.0.0.1:9"},
		{"service": ingress.CatchAll}}}
	return c, f
}

func TestDeleteDryRunChangesNothing(t *testing.T) {
	c, f := setup(t)
	var out bytes.Buffer
	if err := deleteRoute(f, c, "t.example.com", false, &out); err != nil {
		t.Fatal(err)
	}
	if f.saved != 0 || len(f.deleted) != 0 || c.Route("t.example.com") == nil || !strings.Contains(out.String(), "DRY RUN") {
		t.Fatal("dry run mutated something:", out.String())
	}
}

func TestDeleteRefusesUnmanagedHost(t *testing.T) {
	c, f := setup(t)
	if err := deleteRoute(f, c, "os.example.com", true, &bytes.Buffer{}); err == nil || f.saved != 0 || len(f.deleted) != 0 {
		t.Fatalf("must refuse a route cfex did not create: %v", err)
	}
}

func TestDeleteRefusesDNSWithoutManagedComment(t *testing.T) {
	c, f := setup(t)
	f.dns.Comment = "someone else's"
	if err := deleteRoute(f, c, "t.example.com", true, &bytes.Buffer{}); err == nil || len(f.deleted) != 0 || f.saved != 0 {
		t.Fatalf("must refuse DNS record lacking the comment: %v", err)
	}
}

func TestDeleteApplyLeavesOtherRoutes(t *testing.T) {
	c, f := setup(t)
	if err := deleteRoute(f, c, "t.example.com", true, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 1 || ingress.Has(f.rules, "t.example.com") || !ingress.Has(f.rules, "other.example.com") || f.reloads != 1 {
		t.Fatalf("bad result: %v %v", f.deleted, f.rules)
	}
}

func TestDeleteWithoutAPITokenLeavesDNSAndSaysSo(t *testing.T) {
	c, f := setup(t)
	f.api = false
	var out bytes.Buffer
	if err := deleteRoute(f, c, "t.example.com", true, &out); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 0 || ingress.Has(f.rules, "t.example.com") || !strings.Contains(out.String(), "delete that CNAME yourself") {
		t.Fatal(out.String())
	}
}

func TestProtectedRouteNeedsForce(t *testing.T) {
	c, _ := setup(t)
	c.Routes[0].Protected = true
	if protectedGuard(c, "t.example.com", false) == nil || protectedGuard(c, "t.example.com", true) != nil {
		t.Fatal("protected route must need --force")
	}
}

func TestClassify(t *testing.T) {
	c := cfg.Default()
	c.Protected = []string{"billing", "pay-*"}
	c.Client = []string{"acme-demo"}
	for _, n := range []string{"billing", "pay-gateway"} {
		if classify(c, n, "") != "PROTECTED" {
			t.Fatal(n)
		}
	}
	if classify(c, "acme-demo", "") != "client" || classify(c, "random", "") != "unmanaged" ||
		classify(c, "cfex-instant-dev-example-com", "") != "instant" || classify(c, c.TunnelName, "") != "managed" {
		t.Fatal("classes")
	}
	if classify(cfg.Default(), "billing", "") != "unmanaged" {
		t.Fatal("nothing is protected unless the user's config says so")
	}
}
