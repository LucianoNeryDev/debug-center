//go:build !windows

package install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/term"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/agent"
)

// EnsureElevated re-executes the binary through sudo when not root. On
// success the process image is replaced and this never returns.
func EnsureElevated(args []string) (relaunched bool, err error) {
	if agent.IsElevated() {
		return false, nil
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil || !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, errors.New("root is required: run this command with sudo")
	}
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	return false, syscall.Exec(sudo, append([]string{"sudo", exe}, args...), os.Environ())
}

func restrictDir(dir string) error { return os.Chmod(dir, 0o700) }

func removeDir(dir string) error { return os.RemoveAll(dir) }

func scheduleSelfDelete() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe)
	}
}

// SpawnUninstaller starts the uninstall outside the service's process group
// (systemd would otherwise kill it together with the service).
func SpawnUninstaller(dir string) error {
	exe := filepath.Join(dir, agent.ExeName)
	args := []string{"uninstall", "--dir", dir, "--yes"}
	if run, err := exec.LookPath("systemd-run"); err == nil {
		unit := "dbc-uninstall-" + strconv.FormatInt(time.Now().Unix(), 10)
		out, err := exec.Command(run, append([]string{"--collect", "--quiet", "--unit", unit, exe}, args...)...).CombinedOutput()
		if err == nil {
			return nil
		}
		fmt.Fprintf(os.Stderr, "systemd-run: %v: %s\n", err, out)
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
