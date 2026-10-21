package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/controller"
	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

func newExecCmd() *cobra.Command {
	var req proto.ExecRequest
	var noShell bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "exec <agent> <command>...",
		Short: "Run a command on the agent and stream its output",
		Long: `Run a command on the agent. As with ssh, the arguments are joined with
spaces and run through the agent's shell: PowerShell on Windows, sh elsewhere,
or the one --shell names. --no-shell runs the program directly instead.

Flags go before the agent name. The remote exit status becomes dbc's. Ctrl+C
kills the remote command; a second Ctrl+C stops waiting for it.`,
		Example: `  dbc exec pc1 'Get-Service | Where-Object Status -eq Stopped'
  dbc exec --shell cmd pc1 ipconfig /all
  dbc exec --no-shell pc1 uname -a
  dbc exec -i pc1 sh -s < diagnose.sh`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			if req.TimeoutSec, err = seconds(timeout); err != nil {
				return err
			}
			if noShell {
				req.Argv = args[1:]
			} else {
				req.Command = strings.Join(args[1:], " ")
			}
			_, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return attach(cmd.Context(), c, req)
		},
	}
	// Everything after the agent name belongs to the remote command.
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().StringVar(&req.Shell, "shell", "", "powershell, cmd, sh or bash (default: the agent OS's)")
	cmd.Flags().BoolVar(&noShell, "no-shell", false, "run the program directly, without a shell")
	cmd.Flags().StringVar(&req.Dir, "dir", "", "remote working directory")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "kill the command after this long (0 = never)")
	cmd.Flags().BoolVarP(&req.Stdin, "stdin", "i", false, "forward local standard input")
	cmd.MarkFlagsMutuallyExclusive("shell", "no-shell")
	return cmd
}

func newActionsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "actions <agent>",
		Short: "List the diagnostic and repair actions the agent offers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			as, err := c.Actions(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(as)
			}
			tw := newTable()
			fmt.Fprintln(tw, "NAME\tARGS\tSHELL\tDESCRIPTION")
			for _, a := range as {
				params := make([]string, len(a.Params))
				for i, p := range a.Params {
					params[i] = "<" + p + ">"
				}
				desc := a.Description
				if a.Slow {
					desc += controller.Dim(" (slow)")
				}
				if a.Dangerous {
					desc += controller.Red(" (disruptive)")
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.Name, strings.Join(params, " "), a.Shell, desc)
			}
			return tw.Flush()
		},
	}
	jsonFlag(cmd, &asJSON)
	return cmd
}

func newRunCmd() *cobra.Command {
	var yes bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "run <agent> <action> [args]...",
		Short: "Run one of the agent's actions (see dbc actions)",
		Long: `Run a named action from the agent's catalogue. Disruptive actions, such as
reboot, ask for confirmation unless --yes is given. The action's exit status
becomes dbc's, and Ctrl+C stops it.`,
		Example: `  dbc run pc1 net-diag
  dbc run pc1 restart-service nginx
  dbc run --yes pc1 reboot`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			secs, err := seconds(timeout)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			p, c, err := connect(ctx, args[0])
			if err != nil {
				return err
			}
			catalogue, err := c.Actions(ctx)
			if err != nil {
				return err
			}
			var a *proto.Action
			for i := range catalogue {
				if catalogue[i].Name == args[1] {
					a = &catalogue[i]
				}
			}
			if a == nil {
				return fmt.Errorf("%s has no action %q (see `dbc actions %s`)", p.Hostname, args[1], args[0])
			}
			if len(args)-2 != len(a.Params) {
				usage := []string{"dbc run", args[0], a.Name}
				for _, p := range a.Params {
					usage = append(usage, "<"+p+">")
				}
				return fmt.Errorf("usage: %s", strings.Join(usage, " "))
			}
			if a.Dangerous && !yes && !confirm(fmt.Sprintf("%s: %s. Run it on %s?", a.Name, a.Description, p.Hostname)) {
				return errors.New("cancelled")
			}
			if a.Slow {
				fmt.Fprintf(os.Stderr, "%s can take a while; Ctrl+C stops it\n", a.Name)
			}
			return attach(ctx, c, proto.ExecRequest{Action: a.Name, Args: args[2:], TimeoutSec: secs})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask before disruptive actions")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "stop the action after this long (0 = never)")
	return cmd
}

// seconds converts a --timeout to the whole seconds the agent takes, rounding up.
func seconds(d time.Duration) (int, error) {
	if d < 0 {
		return 0, fmt.Errorf("--timeout cannot be negative")
	}
	return int(math.Ceil(d.Seconds())), nil
}

// attach streams a remote command's output until it exits. The first Ctrl+C
// asks the agent to kill the command, so its real end still arrives; a second
// one stops waiting. A non-zero remote status comes back as an exitCode.
func attach(ctx context.Context, c *controller.Client, req proto.ExecRequest) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	// ctx only bounds the handshake; the session outlives its cancellation.
	s, err := c.Exec(ctx, req)
	if err != nil {
		return err
	}
	if req.Stdin {
		go s.ForwardStdin(os.Stdin)
	}

	var interrupted atomic.Bool
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-sig:
		case <-done:
			return
		}
		interrupted.Store(true)
		fmt.Fprintln(os.Stderr, "\nstopping the remote command (Ctrl+C again to stop waiting)")
		_ = s.Kill()
		select {
		case <-sig:
			os.Exit(130)
		case <-done:
		}
	}()

	code, err := s.Wait(os.Stdout, os.Stderr)
	if err != nil && !interrupted.Load() {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	switch {
	case interrupted.Load():
		return exitCode(130)
	case code < 0 || code == 0 && err != nil:
		return exitCode(255) // like ssh: the command could not run or its end is unknown
	case code > 0:
		return exitCode(code)
	}
	return nil
}

func newKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <agent> <pid>...",
		Short: "Terminate processes on the agent",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pids := make([]int, 0, len(args)-1)
			for _, a := range args[1:] {
				pid, err := strconv.Atoi(a)
				if err != nil || pid <= 0 {
					return fmt.Errorf("bad pid %q", a)
				}
				pids = append(pids, pid)
			}
			_, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			failed := 0
			for _, pid := range pids {
				if err := c.Kill(cmd.Context(), pid); err != nil {
					fmt.Fprintf(os.Stderr, "%d: %v\n", pid, err)
					failed++
					continue
				}
				fmt.Printf("killed %d\n", pid)
			}
			if failed > 0 {
				return exitCode(1)
			}
			return nil
		},
	}
}

func newUninstallCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "uninstall <agent>",
		Short: "Remove the agent from the machine and the tailnet",
		Long: `Ask the agent to leave the tailnet and delete itself from the machine.
Afterwards the machine is out of reach: reinstalling needs someone at it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if !yes && !confirm(fmt.Sprintf("Remove the agent from %s? You lose access to the machine.", p.Hostname)) {
				return errors.New("cancelled")
			}
			if err := c.Uninstall(cmd.Context()); err != nil {
				return err
			}
			fmt.Printf("%s is leaving the tailnet and removing itself\n", p.Hostname)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	return cmd
}
