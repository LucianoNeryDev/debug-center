package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"tailscale.com/client/local"
	"tailscale.com/client/tailscale/apitype"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

const testLogin = "alice@example.com"

// newTestServer returns an agent that trusts every caller as testLogin and
// audits to auditPath. It never touches a tailnet: its LocalAPI client points
// at a socket that does not exist, so a logout fails fast.
func newTestServer(t *testing.T, auditPath string) *Server {
	t.Helper()
	return &Server{
		cfg:     &Config{AllowPeers: []string{testLogin}},
		opts:    Options{Logf: func(string, ...any) {}},
		lc:      &local.Client{Socket: filepath.Join(t.TempDir(), "no-tailscaled.sock"), UseSocketOnly: true},
		audit:   NewAudit(auditPath),
		whois:   func(context.Context, string) (*apitype.WhoIsResponse, error) { return whoIsUser(testLogin), nil },
		stopped: make(chan struct{}),
	}
}

// serve exposes the full handler chain, authorization included, on loopback.
func serve(t *testing.T, s *Server) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(s.authorize(s.routes()))
	t.Cleanup(srv.Close)
	return srv
}

func auditFile(t *testing.T) string { return filepath.Join(t.TempDir(), "audit.log") }

// brokenAudit is an audit path that cannot be written: a directory.
func brokenAudit(t *testing.T) string { return t.TempDir() }

func auditLines(t *testing.T, s *Server) []proto.AuditEntry {
	t.Helper()
	es, err := s.audit.Tail(0)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

// helperCommand runs this test binary as the remote command; see
// TestHelperProcess.
func helperCommand() []string {
	return []string{os.Args[0], "-test.run=^TestHelperProcess$"}
}

// TestHelperProcess is not a test of its own: the tests below run the test
// binary as a remote command, and this is what it does then.
func TestHelperProcess(t *testing.T) {
	if p := os.Getenv("DBC_TEST_MARKER"); p != "" {
		if err := os.WriteFile(p, []byte("ran"), 0o600); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if os.Getenv("DBC_TEST_SLEEP") != "" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

// execOnce sends one exec request and returns the final exit message.
func execOnce(t *testing.T, srv *httptest.Server, req proto.ExecRequest) proto.ExecMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/exec", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if err := wsjson.Write(ctx, c, req); err != nil {
		t.Fatal(err)
	}
	for {
		var m proto.ExecMessage
		if err := wsjson.Read(ctx, c, &m); err != nil {
			t.Fatalf("no exit message: %v", err)
		}
		if m.Type == proto.MsgExit {
			return m
		}
	}
}

func TestExecAuditsAllowedFlag(t *testing.T) {
	tests := []struct {
		name        string
		req         proto.ExecRequest
		wantCode    int
		wantOp      string
		wantAllowed bool
		wantRan     bool
	}{
		{"accepted command", proto.ExecRequest{Argv: helperCommand()}, 0, "exec", true, true},
		{"unknown action", proto.ExecRequest{Action: "no-such-action"}, -1, "action", false, false},
		{"unsupported shell", proto.ExecRequest{Command: "true", Shell: "fish"}, -1, "exec", false, false},
		{"empty request", proto.ExecRequest{}, -1, "exec", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			t.Setenv("DBC_TEST_MARKER", marker)
			s := newTestServer(t, auditFile(t))

			m := execOnce(t, serve(t, s), tt.req)
			if m.Code != tt.wantCode {
				t.Errorf("exit = %+v, want code %d", m, tt.wantCode)
			}
			if _, err := os.Stat(marker); (err == nil) != tt.wantRan {
				t.Errorf("command ran = %v, want %v", err == nil, tt.wantRan)
			}
			es := auditLines(t, s)
			if len(es) != 1 {
				t.Fatalf("audit = %+v, want one line", es)
			}
			e := es[0]
			if e.Allowed != tt.wantAllowed || e.Op != tt.wantOp || e.Peer != testLogin {
				t.Errorf("audit = %+v, want op %q, peer %q, Allowed=%v", e, tt.wantOp, testLogin, tt.wantAllowed)
			}
			if !tt.wantAllowed && !strings.Contains(e.Detail, "(rejected: ") {
				t.Errorf("rejected request detail %q does not say why", e.Detail)
			}
		})
	}
}

// Without an audit line nothing may change on the machine.
func TestStateChangesFailClosed(t *testing.T) {
	t.Run("exec", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "ran")
		t.Setenv("DBC_TEST_MARKER", marker)
		m := execOnce(t, serve(t, newTestServer(t, brokenAudit(t))), proto.ExecRequest{Argv: helperCommand()})
		if m.Code != -1 || !strings.Contains(m.Error, "audited") {
			t.Errorf("exit = %+v, want a refusal", m)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("the command ran without an audit line")
		}
	})

	t.Run("file put", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "new")
		target := filepath.Join(dir, "upload.txt")
		srv := serve(t, newTestServer(t, brokenAudit(t)))
		req, _ := http.NewRequest("PUT", srv.URL+"/v1/files?path="+url.QueryEscape(target), strings.NewReader("payload"))
		expectRefusal(t, req)
		if _, err := os.Stat(dir); err == nil {
			t.Error("the upload changed the disk without an audit line")
		}
	})

	t.Run("process kill", func(t *testing.T) {
		child := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
		child.Env = append(os.Environ(), "DBC_TEST_SLEEP=1")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan struct{})
		go func() { _ = child.Wait(); close(exited) }()
		t.Cleanup(func() { _ = child.Process.Kill(); <-exited })

		srv := serve(t, newTestServer(t, brokenAudit(t)))
		req, _ := http.NewRequest("POST", srv.URL+"/v1/processes/"+strconv.Itoa(child.Process.Pid)+"/kill", nil)
		expectRefusal(t, req)
		select {
		case <-exited:
			t.Error("the process was killed without an audit line")
		case <-time.After(200 * time.Millisecond):
		}
	})

	t.Run("uninstall", func(t *testing.T) {
		s := newTestServer(t, brokenAudit(t))
		uninstalled := make(chan struct{})
		s.opts.OnUninstall = func() { close(uninstalled) }
		req, _ := http.NewRequest("POST", serve(t, s).URL+"/v1/uninstall", nil)
		expectRefusal(t, req)
		select {
		case <-uninstalled:
			t.Error("uninstalled without an audit line")
		case <-s.stopped:
			t.Error("stopped without an audit line")
		case <-time.After(1500 * time.Millisecond): // the handler waits 1s before acting
		}
	})
}

func expectRefusal(t *testing.T, req *http.Request) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var e proto.Error
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(e.Error, "audited") {
		t.Errorf("got %d %q, want 500 and a refusal", resp.StatusCode, e.Error)
	}
}

// Reading is still allowed when the audit log is broken; the failure goes to
// stderr instead.
func TestReadsContinueWithoutAudit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := serve(t, newTestServer(t, brokenAudit(t)))
	resp, err := http.Get(srv.URL + "/v1/files?path=" + url.QueryEscape(file))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "hello" {
		t.Errorf("got %d %q, want 200 \"hello\"", resp.StatusCode, body)
	}
}
