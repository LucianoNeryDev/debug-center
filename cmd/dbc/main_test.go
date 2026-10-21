package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/controller"
	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

func TestRemoteBase(t *testing.T) {
	for in, want := range map[string]string{
		"/var/log/syslog":           "syslog",
		`C:\Windows\Logs\CBS.log`:   "CBS.log",
		`C:\Users\Public\`:          "Public",
		"/home/me/dir/":             "dir",
		`C:\`:                       "C:",
		"/":                         "",
		`\\server\share\report.pdf`: "report.pdf",
	} {
		if got := remoteBase(in); got != want {
			t.Errorf("remoteBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalDest(t *testing.T) {
	dir := t.TempDir()
	if got, _ := localDest(dir, "a.txt"); got != filepath.Join(dir, "a.txt") {
		t.Errorf("into a directory: got %q", got)
	}
	file := filepath.Join(dir, "named.txt")
	if got, _ := localDest(file, "a.txt"); got != file {
		t.Errorf("new file name: got %q", got)
	}
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := localDest(file, "a.txt"); got != file {
		t.Errorf("existing file: got %q", got)
	}
	for _, base := range []string{"", "C:", ".."} {
		if _, err := localDest(dir, base); err == nil {
			t.Errorf("localDest(dir, %q) should need an explicit name", base)
		}
	}
}

func TestProbe(t *testing.T) {
	controller.Color = false
	agentPort := func(h http.HandlerFunc) int {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
		return port
	}
	loopback := netip.MustParseAddr("127.0.0.1")
	row := func(online bool) []agentRow {
		return []agentRow{{Peer: controller.Peer{Hostname: "dbc-pc-abc123", IP: loopback, Online: online}}}
	}

	global.port = agentPort(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(proto.AgentInfo{Name: "Front desk", Version: "1.2.0", APIVersion: proto.APIVersion, Service: true, Elevated: true})
	})
	rows := row(true)
	probe(context.Background(), rows)
	if rows[0].Agent == nil || rows[0].Agent.Name != "Front desk" || rows[0].Denied {
		t.Fatalf("allowed agent: %+v", rows[0])
	}
	if s := rows[0].status(); s != "online, service" {
		t.Errorf("status = %q", s)
	}

	global.port = agentPort(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(proto.Error{Error: "peer x is not allowed to control this agent"})
	})
	rows = row(true)
	probe(context.Background(), rows)
	if !rows[0].Denied || rows[0].Agent != nil {
		t.Fatalf("denied agent: %+v", rows[0])
	}

	rows = row(false)
	probe(context.Background(), rows)
	if rows[0].Agent != nil || rows[0].Error != "" {
		t.Errorf("offline peers must not be probed: %+v", rows[0])
	}
}
