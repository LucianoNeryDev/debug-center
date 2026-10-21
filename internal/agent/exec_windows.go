package agent

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const defaultShell = "powershell"

// psPrelude makes PowerShell emit UTF-8 and drop progress records, which
// would otherwise arrive on stderr as CLIXML noise.
const psPrelude = "$ProgressPreference='SilentlyContinue';[Console]::OutputEncoding=[Text.Encoding]::UTF8;"

func shellCommand(shell, line string) (*exec.Cmd, error) {
	if shell == "" {
		shell = defaultShell
	}
	var cmd *exec.Cmd
	switch shell {
	case "powershell", "pwsh":
		exe := "powershell.exe"
		if shell == "pwsh" {
			exe = "pwsh.exe"
		}
		// -EncodedCommand sidesteps every quoting layer between us and PS.
		cmd = exec.Command(exe, "-NoLogo", "-NoProfile", "-NonInteractive",
			"-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePS(psPrelude+line))
	case "cmd":
		cmd = exec.Command("cmd.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /S /C "chcp 65001 >nul & ` + line + `"`}
	default:
		return nil, fmt.Errorf("unsupported shell %q on windows (use powershell, pwsh or cmd)", shell)
	}
	prepareCmd(cmd)
	return cmd, nil
}

func encodePS(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, r := range u {
		b[2*i], b[2*i+1] = byte(r), byte(r>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func prepareCmd(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW
}

// killTree ends the process and everything it spawned.
func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	kill := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	prepareCmd(kill)
	if kill.Run() != nil {
		_ = cmd.Process.Kill()
	}
}

func quoteArg(shell, s string) (string, error) {
	switch shell {
	case "", "powershell", "pwsh":
		return "'" + replaceAll(s, "'", "''") + "'", nil
	case "cmd":
		for _, r := range s {
			switch r {
			case '"', '&', '|', '<', '>', '^', '%', '!', '\n', '\r':
				return "", fmt.Errorf("argument %q contains a character cmd cannot take safely", s)
			}
		}
		return `"` + s + `"`, nil
	}
	return "", fmt.Errorf("unsupported shell %q", shell)
}

// IsElevated reports whether the process holds an elevated token.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
