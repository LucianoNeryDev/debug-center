package agent

import (
	"testing"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

func TestAuditLog(t *testing.T) {
	a := NewAudit(auditFile(t))
	for _, e := range []proto.AuditEntry{
		{Peer: "alice@example.com", Op: "exec", Detail: "uptime", Allowed: true},
		{Peer: "mallory@example.com", Op: "GET /v1/info", Allowed: false},
	} {
		if err := a.Log(e); err != nil {
			t.Fatal(err)
		}
	}
	es, err := a.Tail(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || !es[0].Allowed || es[1].Allowed || es[0].Time.IsZero() {
		t.Fatalf("entries = %+v", es)
	}
	if last, _ := a.Tail(1); len(last) != 1 || last[0].Peer != "mallory@example.com" {
		t.Errorf("Tail(1) = %+v", last)
	}
}

func TestAuditLogReportsFailure(t *testing.T) {
	// A directory cannot be opened for writing, even by root.
	a := NewAudit(t.TempDir())
	if err := a.Log(proto.AuditEntry{Op: "exec"}); err == nil {
		t.Fatal("Log into a directory succeeded")
	}
}
