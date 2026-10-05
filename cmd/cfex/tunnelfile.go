package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/muthuishere/cfex-cli/internal/cfd"
	"github.com/muthuishere/cfex-cli/internal/cfg"
	"github.com/muthuishere/cfex-cli/internal/ingress"
)

// cfex's durable tunnel is a locally-managed cloudflared tunnel whose ingress lives in ~/.config/cfex/tunnel.yml.
// Routes are added/removed with the pure merge in internal/ingress, so one route never alters another.

func loadRules() ([]ingress.Rule, error) {
	b, err := os.ReadFile(cfg.TunnelFile())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Ingress []map[string]any `yaml:"ingress"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", cfg.TunnelFile(), err)
	}
	rules := make([]ingress.Rule, 0, len(doc.Ingress))
	for _, r := range doc.Ingress {
		rules = append(rules, r)
	}
	return rules, nil
}

func saveRules(id string, rules []ingress.Rule) error {
	if err := cfg.GuardTestPath(cfg.TunnelFile()); err != nil {
		return err
	}
	doc := map[string]any{"tunnel": id, "credentials-file": cfd.CredentialsFile(id), "ingress": ingress.Normalize(rules)}
	b, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.TunnelFile()), 0o700); err != nil {
		return err
	}
	return os.WriteFile(cfg.TunnelFile(), b, 0o600)
}

// serviceURL is the origin cloudflared dials for a route.
func serviceURL(https bool, port int) string {
	if https {
		return fmt.Sprintf("https://127.0.0.1:%d", port)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// routeRule builds the ingress rule for a route; https origins skip certificate verification unless verify is set.
func routeRule(host, service string, https, verify bool) ingress.Rule {
	r := ingress.Rule{"hostname": host, "service": service}
	if https && !verify {
		r["originRequest"] = map[string]any{"noTLSVerify": true}
	}
	return r
}
