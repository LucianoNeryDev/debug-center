package agent

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"tailscale.com/client/tailscale/apitype"
)

// Peer is the tailnet identity behind a request, as reported by WhoIs.
type Peer struct {
	Login string
	Node  string
	Tags  []string
	Addr  string
}

func (p Peer) String() string {
	if len(p.Tags) > 0 {
		return strings.Join(p.Tags, ",")
	}
	return p.Login
}

type peerKey struct{}

func peerFrom(ctx context.Context) Peer {
	p, _ := ctx.Value(peerKey{}).(Peer)
	return p
}

func peerOf(w *apitype.WhoIsResponse, addr string) Peer {
	p := Peer{Addr: addr}
	if w == nil {
		return p
	}
	if w.Node != nil {
		p.Node = strings.TrimSuffix(w.Node.Name, ".")
		p.Tags = w.Node.Tags
	}
	if w.UserProfile != nil {
		p.Login = w.UserProfile.LoginName
	}
	return p
}

// allowed reports whether a peer matches the allowlist. Entries starting with
// "tag:" match node tags; anything else matches the login name exactly.
// Tagged nodes never match on login, since their login is a placeholder.
func allowed(allow []string, p Peer) bool {
	for _, a := range allow {
		if strings.HasPrefix(a, "tag:") {
			if slices.Contains(p.Tags, a) {
				return true
			}
		} else if len(p.Tags) == 0 && p.Login != "" && strings.EqualFold(a, p.Login) {
			return true
		}
	}
	return false
}

// authorize gates every endpoint behind the tailnet identity of the caller.
// The tailnet ACL is the first gate; this is the second.
func (s *Server) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, err := s.lc.WhoIs(r.Context(), r.RemoteAddr)
		p := peerOf(who, r.RemoteAddr)
		if err != nil || !allowed(s.cfg.AllowPeers, p) {
			s.audit.Log(auditEntry(p, r.Method+" "+r.URL.Path, "", false))
			writeErr(w, http.StatusForbidden, "peer "+p.String()+" ("+p.Node+") is not allowed to control this agent")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), peerKey{}, p)))
	})
}
