package agent

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/sysinfo"
)

var osGetpid = os.Getpid

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.info)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	s.record(r, "info", "")
	si := sysinfo.Collect(r.Context())
	si.Agent = s.info
	writeJSON(w, si)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var sm sysinfo.Sampler
	sm.Prime(r.Context())
	time.Sleep(time.Second)
	st, err := sm.Sample(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, st)
}

func (s *Server) handleStatsStream(w http.ResponseWriter, r *http.Request) {
	interval, err := time.ParseDuration(r.URL.Query().Get("interval"))
	if err != nil || interval < 250*time.Millisecond {
		interval = 2 * time.Second
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	s.record(r, "stats.stream", interval.String())
	ctx := c.CloseRead(r.Context()) // cancelled when the controller goes away

	var sm sysinfo.Sampler
	sm.Prime(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		st, err := sm.Sample(ctx)
		if err != nil {
			c.Close(websocket.StatusInternalError, err.Error())
			return
		}
		wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = wsjson.Write(wctx, c, st)
		cancel()
		if err != nil {
			return
		}
	}
}

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	ps, err := sysinfo.Processes(r.Context(), time.Second)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, ps)
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.ParseInt(r.PathValue("pid"), 10, 32)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad pid")
		return
	}
	s.record(r, "process.kill", strconv.FormatInt(pid, 10))
	if err := sysinfo.Kill(r.Context(), int32(pid)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "killed"})
}

func (s *Server) handleActions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, Actions())
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 100
	}
	es, err := s.audit.Tail(n)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, es)
}
