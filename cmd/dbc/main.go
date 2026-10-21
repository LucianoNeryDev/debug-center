// Command dbc is the technician's controller. It finds agents on the support
// tailnet through the local tailscaled and drives their control API:
// inventory, live load, processes, commands, files and the audit log.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"tailscale.com/client/local"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/buildinfo"
	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/controller"
	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

var global struct {
	port   int
	socket string
}

func main() {
	root := &cobra.Command{
		Use:   "dbc",
		Short: "Debug Center controller: find and service agents on the support tailnet",
		Long: `dbc finds Debug Center agents on the support tailnet through the local
tailscaled and drives them: inventory, live load, processes, commands,
actions, files and the audit log.

<agent> is a tailnet IP, a hostname with or without the dbc- prefix, the
agent ID (first 6 characters are enough) or a unique part of the hostname.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().IntVar(&global.port, "port", proto.Port, "agent API port on the tailnet")
	root.PersistentFlags().StringVar(&global.socket, "socket", "", "tailscaled LocalAPI socket (default: the platform's)")

	root.AddGroup(
		&cobra.Group{ID: "inspect", Title: "Inspect:"},
		&cobra.Group{ID: "control", Title: "Control:"},
		&cobra.Group{ID: "files", Title: "Files:"},
	)
	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("inspect", newLsCmd(), newInfoCmd(), newStatsCmd(), newTopCmd(), newPsCmd(), newAuditCmd())
	add("control", newExecCmd(), newActionsCmd(), newRunCmd(), newKillCmd(), newUninstallCmd())
	add("files", newFilesCmd(), newPullCmd(), newPushCmd())
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run:   func(*cobra.Command, []string) { fmt.Println(buildinfo.Version) },
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := root.ExecuteContext(ctx)
	interrupted := ctx.Err() != nil // stop cancels ctx too
	stop()
	var code exitCode
	switch {
	case errors.As(err, &code):
		os.Exit(int(code))
	case err != nil && interrupted:
		os.Exit(130) // interrupted; the error is just the cancellation
	case err != nil:
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// exitCode ends dbc with a status and no further message, e.g. to pass on
// the exit status of a remote command.
type exitCode int

func (c exitCode) Error() string { return "exit status " + strconv.Itoa(int(c)) }

func discoverer() controller.Discoverer {
	return controller.Discoverer{LC: &local.Client{Socket: global.socket, UseSocketOnly: global.socket != ""}}
}

// connect resolves the agent named on the command line.
func connect(ctx context.Context, query string) (controller.Peer, *controller.Client, error) {
	p, err := discoverer().Resolve(ctx, query)
	if err != nil {
		return p, nil, err
	}
	if !p.Online {
		warnf("%s looks offline (last seen %s); trying anyway", p.Hostname, controller.Ago(p.LastSeen))
	}
	return p, controller.NewClient(p.IP, global.port), nil
}

func warnf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", a...)
}

// confirm asks a yes/no question; anything but y or yes declines.
func confirm(question string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

func jsonFlag(cmd *cobra.Command, v *bool) {
	cmd.Flags().BoolVar(v, "json", false, "print JSON instead of a table")
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// newTable aligns tab-separated columns. Only the last column may carry
// colour: escape codes would count towards the width of the others.
func newTable() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

// trunc shortens s to n runes.
func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
