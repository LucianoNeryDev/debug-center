package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/controller"
	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// agentRow is one line of `dbc ls`: the tailnet view of a peer plus, for
// online peers, what the agent says about itself.
type agentRow struct {
	controller.Peer
	Agent  *proto.AgentInfo `json:"agent,omitempty"`
	Denied bool             `json:"denied,omitempty"`
	Error  string           `json:"error,omitempty"`
}

func (r agentRow) platform() string {
	if r.Agent != nil {
		return r.Agent.OS + "/" + r.Agent.Arch
	}
	return r.OS
}

func (r agentRow) status() string {
	switch {
	case !r.Online:
		return controller.Dim("offline, seen " + controller.Ago(r.LastSeen))
	case r.Denied:
		return controller.Red("online, access denied")
	case r.Agent == nil:
		return controller.Red("online, not answering")
	}
	parts := []string{controller.Green("online"), "session"}
	if r.Agent.Service {
		parts[1] = "service"
	}
	if !r.Agent.Elevated {
		parts = append(parts, controller.Red("not elevated"))
	}
	if r.Agent.APIVersion != proto.APIVersion {
		parts = append(parts, controller.Red(fmt.Sprintf("api v%d, dbc speaks v%d", r.Agent.APIVersion, proto.APIVersion)))
	}
	return strings.Join(parts, ", ")
}

// probe asks each online agent who it is, which also shows whether this
// controller is allowed in.
func probe(ctx context.Context, rows []agentRow) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := range rows {
		if !rows[i].Online {
			continue
		}
		wg.Go(func() {
			r := &rows[i]
			a, err := controller.NewClient(r.IP, global.port).Agent(ctx)
			if err != nil {
				var ae *controller.APIError
				r.Denied = errors.As(err, &ae) && ae.Status == http.StatusForbidden
				r.Error = err.Error()
				return
			}
			r.Agent = &a
		})
	}
	wg.Wait()
}

func newLsCmd() *cobra.Command {
	var all, asJSON bool
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List agents on the tailnet",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			peers, err := discoverer().Peers(cmd.Context())
			if err != nil {
				return err
			}
			rows := []agentRow{}
			for _, p := range peers {
				if p.Online || all {
					rows = append(rows, agentRow{Peer: p})
				}
			}
			probe(cmd.Context(), rows)
			if asJSON {
				return printJSON(rows)
			}
			if len(rows) == 0 {
				if all || len(peers) == 0 {
					fmt.Fprintln(os.Stderr, "no agents on the tailnet")
				} else {
					fmt.Fprintf(os.Stderr, "no agents online; %d offline (--all lists them)\n", len(peers))
				}
				return nil
			}
			tw := newTable()
			fmt.Fprintln(tw, "HOSTNAME\tNAME\tIP\tOS\tVERSION\tSTATUS")
			for _, r := range rows {
				name, version := "-", "-"
				if r.Agent != nil {
					name, version = r.Agent.Name, r.Agent.Version
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Hostname, name, r.IP, r.platform(), version, r.status())
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "include offline agents")
	jsonFlag(cmd, &asJSON)
	return cmd
}

func newInfoCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "info <agent>",
		Short: "Show the machine's hardware and software inventory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			si, err := c.Info(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(si)
			}
			controller.PrintInfo(os.Stdout, si)
			return nil
		},
	}
	jsonFlag(cmd, &asJSON)
	return cmd
}

func newStatsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "stats <agent>",
		Short: "Take one sample of CPU, memory, disk, network and temperatures",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			st, err := c.Stats(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(st)
			}
			controller.PrintStats(os.Stdout, p.Hostname, st)
			return nil
		},
	}
	jsonFlag(cmd, &asJSON)
	return cmd
}

func newTopCmd() *cobra.Command {
	var interval time.Duration
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "top <agent>",
		Short: "Watch CPU, memory, disk, network and temperatures live",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval < 250*time.Millisecond {
				return fmt.Errorf("--interval must be at least 250ms")
			}
			ctx := cmd.Context()
			p, c, err := connect(ctx, args[0])
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				return c.StreamStats(ctx, interval, func(st proto.Stats) error { return enc.Encode(st) })
			}
			if term.IsTerminal(int(os.Stdout.Fd())) {
				// Alternate screen with the cursor hidden, like top(1).
				fmt.Print("\x1b[?1049h\x1b[?25l")
				defer fmt.Print("\x1b[?25h\x1b[?1049l")
			}
			fmt.Printf("waiting for the first sample from %s…\n", p.Hostname)
			return c.StreamStats(ctx, interval, func(st proto.Stats) error {
				controller.RenderTop(os.Stdout, p.Hostname, st)
				return nil
			})
		},
	}
	cmd.Flags().DurationVarP(&interval, "interval", "i", 2*time.Second, "time between samples")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON sample per line instead of a live view")
	return cmd
}

func newPsCmd() *cobra.Command {
	var sortBy string
	var limit int
	var wide, asJSON bool
	cmd := &cobra.Command{
		Use:   "ps <agent> [filter]",
		Short: "List processes, busiest first",
		Long: `List processes with their CPU use (share of the whole machine over one
second) and resident memory. A filter keeps processes whose name or command
line contains it, ignoring case.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			byKey, ok := processOrder[sortBy]
			if !ok {
				return fmt.Errorf("--sort must be cpu, mem, pid or name")
			}
			_, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			ps, err := c.Processes(cmd.Context())
			if err != nil {
				return err
			}
			if len(args) == 2 {
				f := strings.ToLower(args[1])
				ps = slices.DeleteFunc(ps, func(p proto.Process) bool {
					return !strings.Contains(strings.ToLower(p.Name), f) && !strings.Contains(strings.ToLower(p.Cmdline), f)
				})
			}
			slices.SortStableFunc(ps, byKey)
			if limit > 0 && len(ps) > limit {
				ps = ps[:limit]
			}
			if asJSON {
				return printJSON(ps)
			}
			tw := newTable()
			fmt.Fprintln(tw, "PID\tPPID\tUSER\tCPU%\tMEM\tNAME\tCOMMAND")
			for _, p := range ps {
				line := p.Cmdline
				if !wide {
					line = trunc(line, 80)
				}
				fmt.Fprintf(tw, "%d\t%d\t%s\t%.1f\t%s\t%s\t%s\n",
					p.PID, p.PPID, trunc(p.User, 24), p.CPUPercent, controller.Bytes(float64(p.RSS)), p.Name, line)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVarP(&sortBy, "sort", "s", "cpu", "sort by cpu, mem, pid or name")
	cmd.Flags().IntVarP(&limit, "limit", "n", 25, "show at most this many (0 = all)")
	cmd.Flags().BoolVarP(&wide, "wide", "w", false, "do not shorten command lines")
	jsonFlag(cmd, &asJSON)
	return cmd
}

var processOrder = map[string]func(a, b proto.Process) int{
	"cpu":  func(a, b proto.Process) int { return cmp.Compare(b.CPUPercent, a.CPUPercent) },
	"mem":  func(a, b proto.Process) int { return cmp.Compare(b.RSS, a.RSS) },
	"pid":  func(a, b proto.Process) int { return cmp.Compare(a.PID, b.PID) },
	"name": func(a, b proto.Process) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) },
}

func newAuditCmd() *cobra.Command {
	var n int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "audit <agent>",
		Short: "Show the agent's audit log: who asked for what, oldest first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			es, err := c.Audit(cmd.Context(), n)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(es)
			}
			tw := newTable()
			fmt.Fprintln(tw, "TIME\tPEER\tNODE\tOP\tRESULT\tDETAIL")
			for _, e := range es {
				node, _, _ := strings.Cut(e.Node, ".")
				result := "ok"
				if !e.Allowed {
					result = "denied"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					e.Time.Local().Format("2006-01-02 15:04:05"), e.Peer, node, e.Op, result, e.Detail)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().IntVarP(&n, "limit", "n", 50, "number of most recent entries")
	jsonFlag(cmd, &asJSON)
	return cmd
}
