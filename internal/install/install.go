// Package install puts the agent on a machine as a service and takes it off
// again. Everything lives in one directory so removal is complete.
package install

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kardianos/service"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/agent"
)

const (
	ServiceName        = "dbc-agent"
	ServiceDisplayName = "Debug Center Agent"
)

// ServiceConfig describes the OS service for an agent installed in dir.
func ServiceConfig(dir string) *service.Config {
	return &service.Config{
		Name:        ServiceName,
		DisplayName: ServiceDisplayName,
		Description: "Remote support agent for device diagnostics. Remove with: " + filepath.Join(dir, agent.ExeName) + " uninstall",
		Executable:  filepath.Join(dir, agent.ExeName),
		Arguments:   []string{"service", "--dir", dir},
		Option: service.KeyValue{
			"OnFailure":              "restart",
			"OnFailureDelayDuration": "10s",
			"Restart":                "always",
		},
	}
}

type nopProgram struct{}

func (nopProgram) Start(service.Service) error { return nil }
func (nopProgram) Stop(service.Service) error  { return nil }

// Service returns a handle to the installed service for control calls.
func Service(dir string) (service.Service, error) {
	return service.New(nopProgram{}, ServiceConfig(dir))
}

// Options configure an install.
type Options struct {
	Dir        string
	Name       string
	ControlURL string
	AuthKey    string
	Allow      []string
	Tags       []string
	Port       int
	Yes        bool
	Stdin      io.Reader
	Out        io.Writer
}

// Install copies the running binary into Dir, writes the config, registers
// the service and waits for the agent to come online.
func Install(o Options) error {
	if len(o.Allow) == 0 {
		return errors.New("no allowed controllers: pass --allow (a tailnet login or tag:…) or build with a default")
	}
	cfg, err := agent.LoadConfig(o.Dir)
	reinstall := err == nil
	if !reinstall {
		cfg = agent.NewConfig(o.Dir)
		cfg.ID = agent.NewID()
	}
	cfg.Name = o.Name
	cfg.ControlURL = o.ControlURL
	cfg.AllowPeers = o.Allow
	cfg.Tags = o.Tags
	cfg.Port = o.Port
	_, stateErr := os.Stat(cfg.StateDir())
	registered := stateErr == nil
	if o.AuthKey != "" {
		cfg.AuthKey = o.AuthKey
	} else if !registered {
		return errors.New("no auth key: pass --auth-key or set DBC_AUTH_KEY (a pre-auth key from the control server)")
	}

	if !o.Yes && !consent(o, cfg) {
		return errors.New("installation declined")
	}

	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return err
	}
	if err := restrictDir(o.Dir); err != nil {
		return fmt.Errorf("restrict %s: %w", o.Dir, err)
	}

	svc, err := Service(o.Dir)
	if err != nil {
		return err
	}
	if reinstall {
		_ = svc.Stop()
		_ = svc.Uninstall()
	}
	if err := copySelf(filepath.Join(o.Dir, agent.ExeName)); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}
	os.Remove(cfg.StatusPath())
	if err := cfg.Save(); err != nil {
		return err
	}
	if err := svc.Install(); err != nil {
		return fmt.Errorf("register service: %w", err)
	}
	if err := svc.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	fmt.Fprintf(o.Out, "Service %q installed and started. Waiting for the agent to join the tailnet…\n", ServiceDisplayName)

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if st, err := cfg.ReadStatus(); err == nil {
			if st.Error != "" {
				return fmt.Errorf("agent could not join the tailnet: %s", st.Error)
			}
			if st.Online {
				fmt.Fprintf(o.Out, "\nOnline as %s (%s), agent id %s.\n", st.Hostname, strings.Join(st.TailnetIPs, ", "), cfg.ID)
				fmt.Fprintf(o.Out, "Remove it later with:\n  %s uninstall\n", filepath.Join(o.Dir, agent.ExeName))
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("agent did not come online within 90s; check the service log and the auth key")
}

func consent(o Options, cfg *agent.Config) bool {
	control := cfg.ControlURL
	if control == "" {
		control = "the Tailscale control plane"
	}
	fmt.Fprintf(o.Out, `
Debug Center — remote support agent

Installing this will:
  - register the system service %q, stored in %s
  - join the private support network at %s as %q
  - let %s view system information, run commands with
    administrator rights, transfer files and restart this computer
  - record every remote action in %s

The agent is reachable only through that private network, never from the
internet or the local network.
Remove it at any time with:  %s uninstall

Type "yes" to allow this: `, ServiceDisplayName, o.Dir, control, cfg.Hostname(),
		strings.Join(cfg.AllowPeers, ", "), cfg.AuditPath(), filepath.Join(o.Dir, agent.ExeName))
	line, _ := bufio.NewReader(o.Stdin).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "yes")
}

// UninstallOptions configure a removal.
type UninstallOptions struct {
	Dir        string
	Yes        bool
	KeepAudit  string // copy the audit log here before removal
	SelfDelete bool   // delete the running binary afterwards (temp copy)
	Stdin      io.Reader
	Out        io.Writer
}

// Uninstall stops and removes the service, logs the node out of the tailnet
// and deletes the agent directory.
func Uninstall(o UninstallOptions) error {
	if !o.Yes {
		fmt.Fprintf(o.Out, "Remove %s and everything in %s? Type \"yes\": ", ServiceDisplayName, o.Dir)
		line, _ := bufio.NewReader(o.Stdin).ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(line), "yes") {
			return errors.New("uninstall cancelled")
		}
	}
	step := func(format string, a ...any) { fmt.Fprintf(o.Out, "- "+format+"\n", a...) }

	if svc, err := Service(o.Dir); err == nil {
		if st, err := svc.Status(); err == nil && st == service.StatusRunning {
			if err := svc.Stop(); err != nil {
				step("stop service: %v", err)
			}
			for i := 0; i < 30; i++ {
				if st, err := svc.Status(); err != nil || st != service.StatusRunning {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
		if err := svc.Uninstall(); err == nil {
			step("service removed")
		} else if !errors.Is(err, service.ErrNotInstalled) {
			step("service: %v", err)
		}
	}

	if cfg, err := agent.LoadConfig(o.Dir); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		if err := agent.Logout(ctx, cfg); err != nil {
			step("tailnet logout: %v", err)
		} else {
			step("logged out of the tailnet")
		}
		cancel()
		if o.KeepAudit != "" {
			if err := copyFile(cfg.AuditPath(), o.KeepAudit); err != nil && !os.IsNotExist(err) {
				step("keep audit log: %v", err)
			} else if err == nil {
				step("audit log saved to %s", o.KeepAudit)
			}
		}
	}

	if err := removeDir(o.Dir); err != nil {
		return fmt.Errorf("remove %s: %w", o.Dir, err)
	}
	step("removed %s", o.Dir)
	if o.SelfDelete {
		scheduleSelfDelete()
	}
	fmt.Fprintln(o.Out, "Uninstall complete.")
	return nil
}

func copySelf(dst string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	if src, err = filepath.EvalSymlinks(src); err != nil {
		return err
	}
	if same(src, dst) {
		return nil
	}
	return copyFile(src, dst)
}

func same(a, b string) bool {
	fa, err1 := os.Stat(a)
	fb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
