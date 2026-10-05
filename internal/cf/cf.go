// Package cf is a thin Cloudflare API client. The token is passed in by the caller (read from the
// environment only, never from a file or a flag).
package cf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/muthuishere/cfex-cli/internal/ingress"
)

const Base = "https://api.cloudflare.com/client/v4"

type Client struct {
	HTTP  *http.Client
	token string
}

func New(token string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, token: token}
}

func (c *Client) do(method, path string, body, out any) error {
	tok := c.token
	if tok == "" {
		return fmt.Errorf("no Cloudflare API token (set CLOUDFLARE_API_TOKEN)")
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, Base+path, rd)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int
			Message string
		} `json:"errors"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("%s %s: http %d, unparseable body", method, path, resp.StatusCode)
	}
	if !env.Success {
		msgs := []string{}
		for _, e := range env.Errors {
			msgs = append(msgs, fmt.Sprintf("%d %s", e.Code, e.Message))
		}
		return fmt.Errorf("%s %s: http %d: %s", method, path, resp.StatusCode, strings.Join(msgs, "; "))
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

type Account struct{ ID, Name string }
type Tunnel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	ConfigSrc   string `json:"config_src"`
	Connections []struct {
		ID string `json:"id"`
	} `json:"connections"`
}
type Zone struct{ ID, Name string }
type DNS struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Comment string `json:"comment"`
	Proxied bool   `json:"proxied"`
}

func (c *Client) Accounts() (a []Account, err error) { return a, c.do("GET", "/accounts", nil, &a) }

func (c *Client) Tunnels(acct string) (t []Tunnel, err error) {
	return t, c.do("GET", "/accounts/"+acct+"/cfd_tunnel?is_deleted=false&per_page=100", nil, &t)
}

func (c *Client) Zones() (z []Zone, err error) { return z, c.do("GET", "/zones?per_page=50", nil, &z) }

// TunnelConfig returns the full remote config (so a PUT can round-trip fields we don't own) and its ingress.
func (c *Client) TunnelConfig(acct, tid string) (full map[string]any, rules []ingress.Rule, err error) {
	var r struct {
		Config map[string]any `json:"config"`
	}
	if err = c.do("GET", "/accounts/"+acct+"/cfd_tunnel/"+tid+"/configurations", nil, &r); err != nil {
		return
	}
	full = r.Config
	if full == nil {
		full = map[string]any{}
	}
	if raw, ok := full["ingress"].([]any); ok {
		for _, x := range raw {
			if m, ok := x.(map[string]any); ok {
				rules = append(rules, m)
			}
		}
	}
	return
}

func (c *Client) PutIngress(acct, tid string, full map[string]any, rules []ingress.Rule) error {
	cp := map[string]any{}
	for k, v := range full {
		cp[k] = v
	}
	cp["ingress"] = rules
	return c.do("PUT", "/accounts/"+acct+"/cfd_tunnel/"+tid+"/configurations", map[string]any{"config": cp}, nil)
}

func (c *Client) CreateTunnel(acct, name, secretB64 string) (t Tunnel, err error) {
	return t, c.do("POST", "/accounts/"+acct+"/cfd_tunnel", map[string]any{
		"name": name, "config_src": "cloudflare", "tunnel_secret": secretB64}, &t)
}

// TunnelToken returns the connector token. Callers must hand it straight to sec, never print it.
func (c *Client) TunnelToken(acct, tid string) (tok string, err error) {
	return tok, c.do("GET", "/accounts/"+acct+"/cfd_tunnel/"+tid+"/token", nil, &tok)
}

func (c *Client) DNSByName(zone, name string) (d []DNS, err error) {
	return d, c.do("GET", "/zones/"+zone+"/dns_records?name="+url.QueryEscape(name), nil, &d)
}

func (c *Client) DNSGet(zone, id string) (d DNS, err error) {
	return d, c.do("GET", "/zones/"+zone+"/dns_records/"+id, nil, &d)
}

func (c *Client) DNSCreate(zone string, d DNS) (out DNS, err error) {
	return out, c.do("POST", "/zones/"+zone+"/dns_records", d, &out)
}

func (c *Client) DNSDelete(zone, id string) error {
	return c.do("DELETE", "/zones/"+zone+"/dns_records/"+id, nil, nil)
}
