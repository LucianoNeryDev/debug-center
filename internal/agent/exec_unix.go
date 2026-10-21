//go:build !windows

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const defaultShell = "sh"

func shellCommand(shell, line string) (*exec.Cmd, error) {
	if shell == "" {
		shell = defaultShell
	}
	var cmd *exec.Cmd
	switch shell {
	case "sh":
		cmd = exec.Command("/bin/sh", "-c", line)
	case "bash", "zsh":
		cmd = exec.Command(shell, "-c", line)
	default:
		return nil, fmt.Errorf("unsupported shell %q (use sh, bash or zsh)", shell)
	}
	prepareCmd(cmd)
	return cmd, nil
}

func prepareCmd(cmd *exec.Cmd) {
	// Own process group, so killTree reaches everything the command spawned.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) != nil {
		_ = cmd.Process.Kill()
	}
}

func quoteArg(shell, s string) (string, error) {
	switch shell {
	case "", "sh", "bash", "zsh":
		return "'" + replaceAll(s, "'", `'\''`) + "'", nil
	}
	return "", fmt.Errorf("unsupported shell %q", shell)
}

// IsElevated reports whether the process runs as root.
func IsElevated() bool { return os.Geteuid() == 0 }
