// Command dbc-agent runs on the machine being serviced. It installs itself as
// a service (with consent and administrator approval), joins the support
// tailnet and executes what an allowed controller asks for.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/agent"
	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/buildinfo"
	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/install"
)

var pause bool

func main() {
	root := &cobra.Command{
		Use:           "dbc-agent",
		Short:         "Debug Center remote support agent",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&pause, "pause", false, "wait for Enter before exiting (used after UAC relaunch)")
	_ = root.PersistentFlags().MarkHidden("pause")

	installCmd := newInstallCmd()
	root.AddCommand(installCmd, newUninstallCmd(), newRunCmd(), newServiceCmd(), newStatusCmd(), &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run:   func(*cobra.Command, []string) { fmt.Println(buildinfo.Version) },
	})

	// Double-clicking the .exe on a customer machine should just start the install.
	if runtime.GOOS == "windows" && len(os.Args) == 1 {
		pause = true
		root.SetArgs([]string{"install"})
	}

	err := root.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	if pause {
		fmt.Print("\nPress Enter to close.")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	if err != nil {
		os.Exit(1)
	}
}

// elevate returns true when an elevated copy took over and we should stop.
func elevate() (bool, error) {
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"install"}
	}
	return install.EnsureElevated(args)
}

type tailnetFlags struct {
	name, controlURL, authKey string
	allow, tags               []string
	port                      int
}

func (f *tailnetFlags) register(cmd *cobra.Command) {
	host, _ := os.Hostname()
	authKey := os.Getenv("DBC_AUTH_KEY")
	if authKey == "" {
		authKey = buildinfo.AuthKey
	}
	fl := cmd.Flags()
	fl.StringVar(&f.name, "name", host, "name shown to the controller (customer, ticket or machine)")
	fl.StringVar(&f.controlURL, "control-url", buildinfo.ControlURL, "coordination server (Headscale) URL")
	fl.StringVar(&f.authKey, "auth-key", authKey, "pre-auth key for the first login (env DBC_AUTH_KEY)")
	fl.StringSliceVar(&f.allow, "allow", buildinfo.List(buildinfo.AllowPeers), "tailnet logins or tag:… allowed to control this agent")
	fl.StringSliceVar(&f.tags, "tag", buildinfo.List(buildinfo.AgentTags), "tags to advertise (the auth key must permit them)")
	fl.IntVar(&f.port, "port", 0, "tailnet port to listen on (default 7420)")
}

func newInstallCmd() *cobra.Command {
	var f tailnetFlags
	var dir string
	var yes bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the agent as a service and join the support tailnet",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if done, err := elevate(); done || err != nil {
				return err
			}
			return install.Install(install.Options{
				Dir: dir, Name: f.name, ControlURL: f.controlURL, AuthKey: f.authKey,
				Allow: f.allow, Tags: f.tags, Port: f.port, Yes: yes,
				Stdin: os.Stdin, Out: os.Stdout,
			})
		},
	}
	f.register(cmd)
	cmd.Flags().StringVar(&dir, "dir", agent.DefaultDir(), "install directory")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the consent prompt (only when the owner already agreed)")
	return cmd
}

func newUninstallCmd() *cobra.Command {
	var o install.UninstallOptions
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the service, leave the tailnet and delete every agent file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if done, err := elevate(); done || err != nil {
				return err
			}
			o.Stdin, o.Out = os.Stdin, os.Stdout
			return install.Uninstall(o)
		},
	}
	cmd.Flags().StringVar(&o.Dir, "dir", agent.DefaultDir(), "install directory")
	cmd.Flags().BoolVarP(&o.Yes, "yes", "y", false, "do not ask for confirmation")
	cmd.Flags().StringVar(&o.KeepAudit, "keep-audit", "", "copy the audit log to this path before deleting")
	cmd.Flags().BoolVar(&o.SelfDelete, "self-delete", false, "delete this binary when done")
	_ = cmd.Flags().MarkHidden("self-delete")
	return cmd
}

// newRunCmd is the no-install mode: a foreground session that leaves the
// tailnet and deletes its state when it ends.
func newRunCmd() *cobra.Command {
	var f tailnetFlags
	var yes bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a one-off foreground session without installing (Ctrl+C ends it and cleans up)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(f.allow) == 0 {
				return fmt.Errorf("no allowed controllers: pass --allow")
			}
			if f.authKey == "" {
				return fmt.Errorf("no auth key: pass --auth-key or set DBC_AUTH_KEY")
			}
			if !agent.IsElevated() {
				fmt.Fprintln(os.Stderr, "warning: not running as administrator/root; many actions will fail")
			}
			dir, err := os.MkdirTemp("", "dbc-session-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			cfg := agent.NewConfig(dir)
			cfg.ID, cfg.Name, cfg.ControlURL, cfg.AuthKey = agent.NewID(), f.name, f.controlURL, f.authKey
			cfg.AllowPeers, cfg.Tags, cfg.Port, cfg.Ephemeral = f.allow, f.tags, f.port, true
			if !yes {
				fmt.Printf("Allow %s to control this computer until you press Ctrl+C? Type \"yes\": ", strings.Join(f.allow, ", "))
				line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				if !strings.EqualFold(strings.TrimSpace(line), "yes") {
					return fmt.Errorf("session declined")
				}
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			err = agent.Run(ctx, cfg, agent.Options{
				LogoutOnExit: true,
				Logf:         func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
			})
			fmt.Println("session ended; left the tailnet and removed local state")
			return err
		},
	}
	f.register(cmd)
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the consent prompt")
	return cmd
}

type program struct {
	cfg    *agent.Config
	logger service.Logger
	cancel context.CancelFunc
	done   chan struct{}
}

func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan struct{})
	go func() {
		defer close(p.done)
		err := agent.Run(ctx, p.cfg, agent.Options{
			Service: true,
			Logf:    func(format string, a ...any) { _ = p.logger.Infof(format, a...) },
			OnUninstall: func() {
				if err := install.SpawnUninstaller(p.cfg.Dir()); err != nil {
					_ = p.logger.Errorf("remote uninstall: %v", err)
				}
			},
		})
		if err != nil && ctx.Err() == nil {
			_ = p.logger.Errorf("agent stopped: %v", err)
			os.Exit(1) // let the service manager restart us
		}
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
	}
	return nil
}

func newServiceCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:    "service",
		Short:  "Entry point used by the service manager",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := agent.LoadConfig(dir)
			if err != nil {
				return err
			}
			p := &program{cfg: cfg}
			svc, err := service.New(p, install.ServiceConfig(dir))
			if err != nil {
				return err
			}
			if p.logger, err = svc.Logger(nil); err != nil {
				return err
			}
			return svc.Run()
		},
	}
	cmd.Flags().StringVar(&dir, "dir", agent.DefaultDir(), "install directory")
	return cmd
}

func newStatusCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether the agent is installed, running and online",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := agent.LoadConfig(dir)
			if err != nil {
				fmt.Printf("not installed in %s\n", dir)
				return nil
			}
			fmt.Printf("name:      %s\nid:        %s\nhostname:  %s\ncontrol:   %s\nallowed:   %s\ndir:       %s\n",
				cfg.Name, cfg.ID, cfg.Hostname(), cfg.ControlURL, strings.Join(cfg.AllowPeers, ", "), dir)
			if svc, err := install.Service(dir); err == nil {
				st, err := svc.Status()
				state := map[service.Status]string{service.StatusRunning: "running", service.StatusStopped: "stopped"}[st]
				if err != nil {
					state = err.Error()
				}
				fmt.Printf("service:   %s\n", state)
			}
			if st, err := cfg.ReadStatus(); err == nil {
				fmt.Printf("online:    %v %s\n", st.Online, strings.Join(st.TailnetIPs, ", "))
				if st.Error != "" {
					fmt.Printf("error:     %s\n", st.Error)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", agent.DefaultDir(), "install directory")
	return cmd
}
