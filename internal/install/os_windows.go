package install

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/agent"
)

// EnsureElevated relaunches the binary through UAC when it lacks an elevated
// token. relaunched is true when the caller should exit and let the elevated
// copy carry on.
func EnsureElevated(args []string) (relaunched bool, err error) {
	if agent.IsElevated() {
		return false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	cwd, _ := os.Getwd()
	params := make([]string, 0, len(args)+1)
	for _, a := range args {
		params = append(params, syscall.EscapeArg(a))
	}
	params = append(params, "--pause")
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	argp, _ := windows.UTF16PtrFromString(strings.Join(params, " "))
	dir, _ := windows.UTF16PtrFromString(cwd)
	if err := windows.ShellExecute(0, verb, file, argp, dir, windows.SW_SHOWNORMAL); err != nil {
		return false, fmt.Errorf("administrator approval was not granted: %w", err)
	}
	return true, nil
}

// restrictDir limits the directory to SYSTEM and Administrators: it holds the
// tailnet node key. SIDs avoid localised group names.
func restrictDir(dir string) error {
	cmd := exec.Command("icacls", dir, "/inheritance:r",
		"/grant:r", "*S-1-5-18:(OI)(CI)F", "/grant:r", "*S-1-5-32-544:(OI)(CI)F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

// removeDir deletes dir. If the running binary lives inside it, Windows will
// not let us delete it, so a detached cmd finishes the job after we exit.
func removeDir(dir string) error {
	err := os.RemoveAll(dir)
	if err == nil {
		return nil
	}
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		return nil
	}
	return detachedCmd(`ping -n 4 127.0.0.1 >nul & rmdir /s /q "` + dir + `"`)
}

func scheduleSelfDelete() {
	if exe, err := os.Executable(); err == nil {
		_ = detachedCmd(`ping -n 4 127.0.0.1 >nul & del /f /q "` + exe + `"`)
	}
}

func detachedCmd(line string) error {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       `cmd.exe /S /C "` + line + `"`,
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// SpawnUninstaller starts a detached copy of the agent that removes the
// service (and with it the current process) and the install directory.
// The copy runs from the temp dir, since the installed binary cannot be
// deleted while it runs.
func SpawnUninstaller(dir string) error {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	tmp := filepath.Join(os.TempDir(), "dbc-uninstall-"+hex.EncodeToString(b)+".exe")
	if err := copyFile(filepath.Join(dir, agent.ExeName), tmp); err != nil {
		return err
	}
	cmd := exec.Command(tmp, "uninstall", "--dir", dir, "--yes", "--self-delete")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
