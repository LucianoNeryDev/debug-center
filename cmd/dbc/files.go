package main

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/controller"
	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

func newFilesCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "files <agent> <dir>",
		Short: "List a remote directory (absolute path)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			es, err := c.List(cmd.Context(), args[1])
			if err != nil {
				return err
			}
			slices.SortFunc(es, func(a, b proto.FileEntry) int {
				if a.IsDir != b.IsDir {
					if a.IsDir {
						return -1
					}
					return 1
				}
				return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
			})
			if asJSON {
				return printJSON(es)
			}
			tw := newTable()
			fmt.Fprintln(tw, "MODE\tSIZE\tMODIFIED\tNAME")
			for _, e := range es {
				size, name := controller.Bytes(float64(e.Size)), e.Name
				if e.IsDir {
					size, name = "-", name+"/"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Mode, size, e.ModTime.Local().Format("2006-01-02 15:04"), name)
			}
			return tw.Flush()
		},
	}
	jsonFlag(cmd, &asJSON)
	return cmd
}

func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <agent> <remote-file> [local-path]",
		Short: "Download a file from the agent",
		Long: `Download a remote file (absolute path). The local path defaults to the
file's name in the current directory; an existing directory receives the file
under its remote name, and - writes it to standard output.`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote, dest := args[1], "."
			if len(args) == 3 {
				dest = args[2]
			}
			_, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if dest == "-" {
				_, err := c.Pull(cmd.Context(), remote, os.Stdout)
				return err
			}
			if dest, err = localDest(dest, remoteBase(remote)); err != nil {
				return err
			}
			// Download next to the destination and rename, so a failed
			// transfer never leaves a truncated file behind.
			tmp, err := os.CreateTemp(filepath.Dir(dest), ".dbc-pull-*")
			if err != nil {
				return err
			}
			n, err := c.Pull(cmd.Context(), remote, tmp)
			if cerr := tmp.Close(); err == nil {
				err = cerr
			}
			if err == nil {
				err = os.Rename(tmp.Name(), dest)
			}
			if err != nil {
				os.Remove(tmp.Name())
				return err
			}
			fmt.Fprintf(os.Stderr, "%s -> %s (%s)\n", remote, dest, controller.Bytes(float64(n)))
			return nil
		},
	}
}

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push <agent> <local-file> <remote-path>",
		Short: "Upload a file to the agent",
		Long: `Upload a file to an absolute remote path, replacing any file there
atomically and creating missing directories. A remote path ending in / or \
receives the file under its local name.`,
		Example: `  dbc push pc1 fix.ps1 'C:\Users\Public\'
  dbc push pc1 nginx.conf /etc/nginx/nginx.conf`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, remote := args[1], args[2]
			if strings.HasSuffix(remote, "/") || strings.HasSuffix(remote, `\`) {
				remote += filepath.Base(local)
			}
			p, c, err := connect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			n, err := c.Push(cmd.Context(), local, remote)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%s -> %s:%s (%s)\n", local, p.Hostname, remote, controller.Bytes(float64(n)))
			return nil
		},
	}
}

// remoteBase is the last element of a remote path, which may use / or \
// whichever OS dbc runs on.
func remoteBase(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// localDest is where a pulled file lands: inside dest when it is an existing
// directory, otherwise dest itself.
func localDest(dest, base string) (string, error) {
	fi, err := os.Stat(dest)
	if err != nil || !fi.IsDir() {
		return dest, nil
	}
	if base == "" || base == "." || base == ".." || strings.HasSuffix(base, ":") {
		return "", fmt.Errorf("cannot name a local file after the remote path; give a local file name")
	}
	return filepath.Join(dest, base), nil
}
