package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

func TestAllowed(t *testing.T) {
	alice := Peer{Login: "alice@example.com", Node: "laptop"}
	tagged := Peer{Login: "tagged-devices", Node: "support-box", Tags: []string{"tag:support"}}
	tests := []struct {
		name  string
		allow []string
		peer  Peer
		want  bool
	}{
		{"login matches untagged node", []string{"alice@example.com"}, alice, true},
		{"login match ignores case", []string{"Alice@Example.COM"}, alice, true},
		{"other login", []string{"bob@example.com"}, alice, false},
		{"login entry never matches tagged node", []string{"tagged-devices"}, tagged, false},
		{"tag matches tagged node", []string{"tag:support"}, tagged, true},
		{"tag matches one of several", []string{"tag:support"}, Peer{Tags: []string{"tag:server", "tag:support"}}, true},
		{"other tag", []string{"tag:admin"}, tagged, false},
		{"tag entry never matches untagged node", []string{"tag:support"}, Peer{Login: "tag:support"}, false},
		{"empty login never matches", []string{""}, Peer{Node: "anonymous"}, false},
		{"empty allowlist", nil, alice, false},
		{"second entry matches", []string{"bob@example.com", "tag:support"}, tagged, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowed(tt.allow, tt.peer); got != tt.want {
				t.Errorf("allowed(%q, %+v) = %v, want %v", tt.allow, tt.peer, got, tt.want)
			}
		})
	}
}

func whoIsUser(login string) *apitype.WhoIsResponse {
	return &apitype.WhoIsResponse{
		Node:        &tailcfg.Node{Name: "laptop.tailnet."},
		UserProfile: &tailcfg.UserProfile{LoginName: login},
	}
}

func TestAuthorize(t *testing.T) {
	tests := []struct {
		name      string
		who       *apitype.WhoIsResponse
		err       error
		wantCode  int
		wantPeer  string
		wantAudit bool // a denial leaves an Allowed=false line
	}{
		{"allowed login", whoIsUser("alice@example.com"), nil, http.StatusOK, "alice@example.com", false},
		{"unknown login", whoIsUser("mallory@example.com"), nil, http.StatusForbidden, "", true},
		{"WhoIs error denies", nil, errors.New("no such peer"), http.StatusForbidden, "", true},
		{"WhoIs error denies even with a matching answer", whoIsUser("alice@example.com"), errors.New("stale"), http.StatusForbidden, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, auditFile(t))
			s.whois = func(context.Context, string) (*apitype.WhoIsResponse, error) { return tt.who, tt.err }
			var reached Peer
			h := s.authorize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = peerFrom(r.Context())
			}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/info", nil))

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if reached.Login != tt.wantPeer {
				t.Errorf("handler saw peer %q, want %q", reached.Login, tt.wantPeer)
			}
			es := auditLines(t, s)
			if !tt.wantAudit {
				if len(es) != 0 {
					t.Errorf("authorize should leave auditing to the handler, got %+v", es)
				}
				return
			}
			if len(es) != 1 || es[0].Allowed || es[0].Op != "GET /v1/info" {
				t.Errorf("denial audit = %+v, want one GET /v1/info line with Allowed=false", es)
			}
		})
	}
}
