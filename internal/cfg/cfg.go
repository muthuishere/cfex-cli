// Package cfg is cfex's config file (~/.config/cfex/config.yaml) and state dir.
package cfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Route struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	Zone      string `yaml:"zone"`
	ZoneID    string `yaml:"zone_id"`
	DNSID     string `yaml:"dns_id"`
	State     string `yaml:"state"` // running | stopped
	Created   string `yaml:"created"`
	TunnelID  string `yaml:"tunnel_id"`
	Protected bool   `yaml:"protected,omitempty"` // managed, but never stopped/deleted without --force
}

type Adopted struct {
	Tunnel    string `yaml:"tunnel"`
	TunnelID  string `yaml:"tunnel_id"`
	Script    string `yaml:"script,omitempty"`
	Label     string `yaml:"service_label,omitempty"`
	TokenFile string `yaml:"token_file,omitempty"` // NAME only, never the value
}

type Config struct {
	// DefaultZone lets `cfex add 3000 app` mean app.<default_zone>. Empty: hosts must be fully qualified.
	DefaultZone string `yaml:"default_zone,omitempty"`
	AccountID   string `yaml:"account_id,omitempty"`
	// TunnelName is the one named tunnel cfex owns for durable routes (`cfex add`).
	TunnelName string `yaml:"tunnel_name"`
	TunnelID   string `yaml:"tunnel_id,omitempty"`

	// TokenEnv is the environment variable holding the Cloudflare API token (default CLOUDFLARE_API_TOKEN;
	// CLOUDFLARE_API_KEY is accepted for compatibility with cfex v1).
	TokenEnv string `yaml:"token_env,omitempty"`
	// TokenCmd is an optional command prefix that cfex re-executes itself under so the token only ever lives in
	// the child's environment, e.g. "op run --" or "sec run CLOUDFLARE_TOKEN --". It must export TokenEnv.
	TokenCmd string `yaml:"token_cmd,omitempty"`

	// ServiceLabel is the launchd label / systemd unit name of the connector for the durable tunnel.
	ServiceLabel string `yaml:"service_label,omitempty"`
	// Connector token handling for the durable tunnel. Default: a 0600 token file under the state dir.
	// Or keep it in a secret manager: ConnectorRunCmd wraps the connector ("sec run MYTOKEN --"), ConnectorEnv
	// names the variable it exports, ConnectorSetCmd stores a value read from stdin ("sec set MYTOKEN").
	ConnectorRunCmd string `yaml:"connector_run_cmd,omitempty"`
	ConnectorEnv    string `yaml:"connector_env,omitempty"`
	ConnectorSetCmd string `yaml:"connector_set_cmd,omitempty"`

	Protected []string  `yaml:"protected,omitempty"` // tunnel-name globs that cfex never modifies
	Client    []string  `yaml:"client,omitempty"`    // tunnel-name globs listed as "client": never modified
	Routes    []Route   `yaml:"routes"`
	Adopted   []Adopted `yaml:"adopted"`
}

func Default() *Config {
	return &Config{TunnelName: "cfex", TokenEnv: "CLOUDFLARE_API_TOKEN", ServiceLabel: "dev.cfex.tunnel"}
}

func Dir() string      { h, _ := os.UserHomeDir(); return filepath.Join(h, ".config", "cfex") }
func StateDir() string { h, _ := os.UserHomeDir(); return filepath.Join(h, ".local", "share", "cfex") }
func Path() string {
	if p := os.Getenv("CFEX_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(Dir(), "config.yaml")
}

func Load() (*Config, error) {
	c := Default()
	b, err := os.ReadFile(Path())
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, err
	}
	if c.TokenEnv == "" {
		c.TokenEnv = "CLOUDFLARE_API_TOKEN"
	}
	if c.TunnelName == "" {
		c.TunnelName = "cfex"
	}
	if c.ServiceLabel == "" {
		c.ServiceLabel = "dev.cfex.tunnel"
	}
	return c, nil
}

// UnderTemp reports whether path lives inside the OS temp dir (symlinks resolved).
func UnderTemp(path string) bool {
	real := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}
	rel, err := filepath.Rel(real(os.TempDir()), real(filepath.Dir(path)))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// InTestBinary is true when the running executable is a `go test` binary.
func InTestBinary() bool { return strings.HasSuffix(os.Args[0], ".test") }

// GuardTestPath is the fail-closed rule: a test binary may only touch a config under the temp dir.
func GuardTestPath(path string) error {
	if InTestBinary() && !UnderTemp(path) {
		return fmt.Errorf("refusing: test binary would touch real config %s (set CFEX_CONFIG under the temp dir; use `make test`)", path)
	}
	return nil
}

func (c *Config) Save() error {
	if err := GuardTestPath(Path()); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), b, 0o600)
}

func (c *Config) Route(host string) *Route {
	for i := range c.Routes {
		if c.Routes[i].Host == host {
			return &c.Routes[i]
		}
	}
	return nil
}

func (c *Config) DropRoute(host string) {
	out := c.Routes[:0]
	for _, r := range c.Routes {
		if r.Host != host {
			out = append(out, r)
		}
	}
	c.Routes = out
}
