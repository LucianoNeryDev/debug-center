//go:build !windows

package agent

import "runtime"

// DefaultDir is where the installed agent keeps its binary and data. One
// directory for everything keeps uninstall a single removal.
func DefaultDir() string {
	if runtime.GOOS == "darwin" {
		return "/Library/Application Support/DebugCenter"
	}
	return "/opt/debug-center"
}

const ExeName = "dbc-agent"
