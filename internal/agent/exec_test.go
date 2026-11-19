package agent

import (
	"slices"
	"strings"
	"testing"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// plainAction returns a catalogue action without parameters, so the test
// runs against whichever catalogue this OS builds.
func plainAction(t *testing.T) action {
	t.Helper()
	for _, a := range catalogue {
		if len(a.Params) == 0 {
			return a
		}
	}
	t.Fatal("the catalogue has no action without parameters")
	return action{}
}

func TestBuildCommand(t *testing.T) {
	a := plainAction(t)
	tests := []struct {
		name     string
		req      proto.ExecRequest
		wantOp   string
		wantDesc string
		wantErr  string
	}{
		{"named action", proto.ExecRequest{Action: a.Name}, "action", a.Name, ""},
		{"unknown action", proto.ExecRequest{Action: "no-such-action", Args: []string{"x"}}, "action", "no-such-action x", "unknown action"},
		{"action with surplus arguments", proto.ExecRequest{Action: a.Name, Args: []string{"x"}}, "action", a.Name + " x", "takes no arguments"},
		{"raw argv", proto.ExecRequest{Argv: []string{"hostname", "-s"}}, "exec", "hostname -s", ""},
		{"shell line, default shell", proto.ExecRequest{Command: "echo hi"}, "exec", "echo hi", ""},
		{"shell line, named shell", proto.ExecRequest{Command: "echo hi", Shell: defaultShell}, "exec", defaultShell + ": echo hi", ""},
		{"unsupported shell", proto.ExecRequest{Command: "echo hi", Shell: "fish"}, "exec", "fish: echo hi", "unsupported shell"},
		{"empty request", proto.ExecRequest{}, "exec", "", "empty request"},
		{"argv wins over a shell line", proto.ExecRequest{Argv: []string{"hostname"}, Command: "rm -rf /"}, "exec", "hostname", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, op, desc, err := buildCommand(tt.req)
			if op != tt.wantOp || desc != tt.wantDesc {
				t.Errorf("op, desc = %q, %q; want %q, %q", op, desc, tt.wantOp, tt.wantDesc)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				if cmd != nil {
					t.Errorf("a rejected request returned a command: %v", cmd.Args)
				}
				return
			}
			if err != nil || cmd == nil {
				t.Fatalf("cmd, err = %v, %v", cmd, err)
			}
			if len(tt.req.Argv) > 0 && !slices.Equal(cmd.Args, tt.req.Argv) {
				t.Errorf("argv ran as %q, want %q unchanged", cmd.Args, tt.req.Argv)
			}
		})
	}
}
