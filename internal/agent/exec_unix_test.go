//go:build !windows

package agent

import (
	"testing"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

func TestBuildCommandShellLine(t *testing.T) {
	a := plainAction(t)
	tests := []struct {
		name string
		req  proto.ExecRequest
		want []string
	}{
		{"default shell", proto.ExecRequest{Command: "uptime; df -h"}, []string{"/bin/sh", "-c", "uptime; df -h"}},
		{"bash", proto.ExecRequest{Command: "echo $BASH", Shell: "bash"}, []string{"bash", "-c", "echo $BASH"}},
		{"named action", proto.ExecRequest{Action: a.Name}, []string{"/bin/sh", "-c", a.Command}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, _, err := buildCommand(tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if len(cmd.Args) != len(tt.want) || cmd.Args[0] != tt.want[0] || cmd.Args[1] != tt.want[1] || cmd.Args[2] != tt.want[2] {
				t.Errorf("args = %q, want %q", cmd.Args, tt.want)
			}
		})
	}
}

// Action arguments come from the technician, but must still reach the
// command as one literal word each.
func TestActionArgumentsAreQuoted(t *testing.T) {
	a := action{Action: proto.Action{Name: "restart", Params: []string{"unit"}}, Command: "systemctl restart {{1}}"}
	tests := []struct{ arg, want string }{
		{"nginx", "systemctl restart 'nginx'"},
		{"x; rm -rf /", "systemctl restart 'x; rm -rf /'"},
		{"it's", `systemctl restart 'it'\''s'`},
		{"$(reboot)", "systemctl restart '$(reboot)'"},
	}
	for _, tt := range tests {
		got, err := a.render([]string{tt.arg})
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("render(%q) = %q, want %q", tt.arg, got, tt.want)
		}
	}
	if _, err := a.render(nil); err == nil {
		t.Error("render without the required argument succeeded")
	}
}
