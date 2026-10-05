// Package ingress holds the PURE merge logic for a tunnel's ingress rule list.
// Every function returns a new slice and never mutates its input, and each only
// touches the rule(s) for the one hostname it is given: adding or removing
// route B can never alter route A.
package ingress

import (
	"errors"
	"fmt"
)

// Rule is one ingress entry exactly as the Cloudflare API returns it (kept generic so unknown
// fields such as originRequest survive a round trip untouched).
type Rule = map[string]any

const CatchAll = "http_status:404"

var ErrExists = errors.New("hostname already has an ingress rule on this tunnel")
var ErrNotFound = errors.New("no ingress rule for that hostname")

func host(r Rule) string { h, _ := r["hostname"].(string); return h }

// IsCatchAll reports whether r is the mandatory final hostname-less rule.
func IsCatchAll(r Rule) bool { return host(r) == "" }

func clone(rules []Rule) []Rule {
	out := make([]Rule, 0, len(rules)+2)
	for _, r := range rules {
		c := Rule{}
		for k, v := range r {
			c[k] = v
		}
		out = append(out, c)
	}
	return out
}

// Normalize returns a copy that ends with exactly one catch-all rule.
func Normalize(rules []Rule) []Rule {
	out := make([]Rule, 0, len(rules)+1)
	var catch Rule
	for _, r := range clone(rules) {
		if IsCatchAll(r) {
			catch = r
			continue
		}
		out = append(out, r)
	}
	if catch == nil {
		catch = Rule{"service": CatchAll}
	}
	return append(out, catch)
}

// Has reports whether any rule carries hostname h.
func Has(rules []Rule, h string) bool {
	for _, r := range rules {
		if host(r) == h {
			return true
		}
	}
	return false
}

// Add appends one rule for hostname -> service just before the catch-all.
func Add(rules []Rule, hostname, service string) ([]Rule, error) {
	if hostname == "" || service == "" {
		return nil, fmt.Errorf("hostname and service are required")
	}
	if Has(rules, hostname) {
		return nil, ErrExists
	}
	n := Normalize(rules)
	last := n[len(n)-1]
	n = append(n[:len(n)-1], Rule{"hostname": hostname, "service": service}, last)
	return n, nil
}

// Remove drops every rule for hostname and nothing else.
func Remove(rules []Rule, hostname string) ([]Rule, error) {
	if !Has(rules, hostname) {
		return nil, ErrNotFound
	}
	out := make([]Rule, 0, len(rules))
	for _, r := range clone(rules) {
		if host(r) != hostname {
			out = append(out, r)
		}
	}
	return Normalize(out), nil
}

// Service returns the service of the first rule for hostname.
func Service(rules []Rule, hostname string) (string, bool) {
	for _, r := range rules {
		if host(r) == hostname {
			s, _ := r["service"].(string)
			return s, true
		}
	}
	return "", false
}
