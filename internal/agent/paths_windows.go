package agent

import (
	"os"
	"path/filepath"
)

// DefaultDir is where the installed agent keeps its binary and data. One
// directory for everything keeps uninstall a single removal.
func DefaultDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "DebugCenter")
}

const ExeName = "dbc-agent.exe"
