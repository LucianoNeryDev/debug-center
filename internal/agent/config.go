// Package agent implements the program that runs on a customer machine: it
// joins the tailnet as its own node (tsnet) and serves the control API there.
package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// Config is persisted as config.json in the agent directory.
type Config struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ControlURL string `json:"control_url,omitempty"`
	// AuthKey is only needed for the first login; the agent clears it once the
	// node is registered, since tsnet keeps the node key in its state dir.
	AuthKey    string   `json:"auth_key,omitempty"`
	AllowPeers []string `json:"allow_peers"`
	Tags       []string `json:"tags,omitempty"`
	Port       int      `json:"port,omitempty"`
	Ephemeral  bool     `json:"ephemeral,omitempty"`

	dir string
}

// Status is written by the running agent so install and status can report it.
type Status struct {
	Hostname   string   `json:"hostname"`
	TailnetIPs []string `json:"tailnet_ips"`
	Online     bool     `json:"online"`
	PID        int      `json:"pid"`
	Error      string   `json:"error,omitempty"`
}

// Dir is the agent directory holding config, tailnet state and the audit log.
func (c *Config) Dir() string { return c.dir }

func (c *Config) StateDir() string   { return filepath.Join(c.dir, "tailnet") }
func (c *Config) AuditPath() string  { return filepath.Join(c.dir, "audit.log") }
func (c *Config) StatusPath() string { return filepath.Join(c.dir, "status.json") }

// Hostname is the agent's tailnet node name: dbc-<name>-<id prefix>.
func (c *Config) Hostname() string {
	return proto.HostnamePrefix + Slug(c.Name) + "-" + c.ID[:6]
}

func (c *Config) ListenPort() int {
	if c.Port == 0 {
		return proto.Port
	}
	return c.Port
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug lowercases a name into a DNS-label-safe fragment.
func Slug(s string) string {
	s = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 32 {
		s = strings.Trim(s[:32], "-")
	}
	if s == "" {
		s = "device"
	}
	return s
}

// NewID returns a random 12-hex-char agent ID.
func NewID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// NewConfig returns a config bound to dir without touching disk.
func NewConfig(dir string) *Config { return &Config{dir: dir} }

func configPath(dir string) string { return filepath.Join(dir, "config.json") }

// LoadConfig reads dir/config.json.
func LoadConfig(dir string) (*Config, error) {
	b, err := os.ReadFile(configPath(dir))
	if err != nil {
		return nil, err
	}
	c := &Config{dir: dir}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", configPath(dir), err)
	}
	return c, c.Validate()
}

func (c *Config) Validate() error {
	switch {
	case len(c.ID) < 6:
		return errors.New("config: id missing")
	case len(c.AllowPeers) == 0:
		return errors.New("config: allow_peers is empty; nobody could control this agent")
	}
	return nil
}

// Save writes config.json with owner-only permissions.
func (c *Config) Save() error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(configPath(c.dir), b)
}

func (c *Config) writeStatus(s Status) {
	b, _ := json.MarshalIndent(s, "", "  ")
	_ = writeFileAtomic(c.StatusPath(), b)
}

// ReadStatus returns the last status written by the running agent.
func (c *Config) ReadStatus() (Status, error) {
	var s Status
	b, err := os.ReadFile(c.StatusPath())
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

func writeFileAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
