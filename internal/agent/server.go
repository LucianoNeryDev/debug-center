package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/tsnet"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/buildinfo"
	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// Options tune how the server runs.
type Options struct {
	// Service is true when running under the OS service manager.
	Service bool
	// OnUninstall runs after a controller asked for removal and the node has
	// logged out of the tailnet. It must remove the agent from the machine.
	OnUninstall func()
	// LogoutOnExit logs the node out when Run returns (foreground sessions).
	LogoutOnExit bool
	// Logf receives operator-facing log lines.
	Logf func(format string, args ...any)
}

// Server is a running agent.
type Server struct {
	cfg     *Config
	opts    Options
	ts      *tsnet.Server
	lc      *local.Client
	audit   *Audit
	info    proto.AgentInfo
	stopped chan struct{}
}

// Run joins the tailnet and serves the control API until ctx ends or a remote
// uninstall completes.
func Run(ctx context.Context, cfg *Config, opts Options) error {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	s := &Server{cfg: cfg, opts: opts, audit: NewAudit(cfg.AuditPath()), stopped: make(chan struct{})}
	s.ts = &tsnet.Server{
		Dir:           cfg.StateDir(),
		Hostname:      cfg.Hostname(),
		AuthKey:       cfg.AuthKey,
		ControlURL:    cfg.ControlURL,
		Ephemeral:     cfg.Ephemeral,
		AdvertiseTags: cfg.Tags,
		Logf:          func(string, ...any) {}, // tsnet backend logs are very chatty
		UserLogf:      opts.Logf,
	}
	defer s.ts.Close()

	opts.Logf("joining tailnet as %s", cfg.Hostname())
	st, err := s.ts.Up(ctx)
	if err != nil {
		cfg.writeStatus(Status{Hostname: cfg.Hostname(), PID: pid(), Error: err.Error()})
		return fmt.Errorf("join tailnet: %w", err)
	}
	if opts.Service && cfg.AuthKey != "" {
		// Registered: the node key lives in the state dir now, so the
		// pre-auth key has no reason to stay on this machine.
		cfg.AuthKey = ""
		if err := cfg.Save(); err != nil {
			opts.Logf("warning: could not clear auth key from config: %v", err)
		}
	}
	if s.lc, err = s.ts.LocalClient(); err != nil {
		return err
	}
	if opts.LogoutOnExit {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = s.lc.Logout(ctx)
		}()
	}
	ln, err := s.ts.Listen("tcp", fmt.Sprintf(":%d", cfg.ListenPort()))
	if err != nil {
		return err
	}

	var ips []string
	for _, ip := range st.TailscaleIPs {
		ips = append(ips, ip.String())
	}
	s.info = proto.AgentInfo{
		ID: cfg.ID, Name: cfg.Name, Hostname: cfg.Hostname(), Version: buildinfo.Version,
		APIVersion: proto.APIVersion, OS: runtime.GOOS, Arch: runtime.GOARCH,
		Elevated: IsElevated(), Service: opts.Service, StartedAt: time.Now().UTC(),
	}
	cfg.writeStatus(Status{Hostname: cfg.Hostname(), TailnetIPs: ips, Online: true, PID: pid()})
	opts.Logf("online at %v, port %d; allowed peers: %v", ips, cfg.ListenPort(), cfg.AllowPeers)

	srv := &http.Server{Handler: s.authorize(s.routes()), ReadHeaderTimeout: 15 * time.Second}
	go func() {
		select {
		case <-ctx.Done():
		case <-s.stopped:
		}
		_ = srv.Close()
	}()
	err = srv.Serve(ln)
	cfg.writeStatus(Status{Hostname: cfg.Hostname(), TailnetIPs: ips, Online: false, PID: pid()})
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/agent", s.handleAgent)
	mux.HandleFunc("GET /v1/info", s.handleInfo)
	mux.HandleFunc("GET /v1/stats", s.handleStats)
	mux.HandleFunc("GET /v1/stats/stream", s.handleStatsStream)
	mux.HandleFunc("GET /v1/processes", s.handleProcesses)
	mux.HandleFunc("POST /v1/processes/{pid}/kill", s.handleKill)
	mux.HandleFunc("GET /v1/actions", s.handleActions)
	mux.HandleFunc("GET /v1/exec", s.handleExec)
	mux.HandleFunc("GET /v1/files", s.handleFileGet)
	mux.HandleFunc("PUT /v1/files", s.handleFilePut)
	mux.HandleFunc("GET /v1/audit", s.handleAudit)
	mux.HandleFunc("POST /v1/uninstall", s.handleUninstall)
	return mux
}

func (s *Server) record(r *http.Request, op, detail string) {
	s.audit.Log(auditEntry(peerFrom(r.Context()), op, detail, true))
}

func auditEntry(p Peer, op, detail string, ok bool) proto.AuditEntry {
	return proto.AuditEntry{Peer: p.String(), Node: p.Node, Addr: p.Addr, Op: op, Detail: detail, Allowed: ok}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(proto.Error{Error: msg})
}

func pid() int { return osGetpid() }

// handleUninstall acknowledges, logs the node out of the tailnet and hands
// over to OnUninstall, which removes the agent from the machine.
func (s *Server) handleUninstall(w http.ResponseWriter, r *http.Request) {
	s.record(r, "uninstall", "")
	writeJSON(w, map[string]string{"status": "uninstalling"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(time.Second) // let the response reach the controller
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := s.lc.Logout(ctx); err != nil {
			s.opts.Logf("tailnet logout failed: %v", err)
		}
		if s.opts.OnUninstall != nil {
			s.opts.OnUninstall()
		}
		close(s.stopped)
	}()
}
