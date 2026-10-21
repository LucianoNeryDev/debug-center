package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// handleExec runs a command, shell line or action and streams its output.
// The first client message is an ExecRequest; after that the client may send
// stdin, stdin_close and kill messages. The server ends with one exit message.
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(4 << 20)
	ctx := r.Context()

	var req proto.ExecRequest
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err = wsjson.Read(rctx, c, &req)
	cancel()
	if err != nil {
		return
	}

	var mu sync.Mutex
	send := func(m proto.ExecMessage) error {
		mu.Lock()
		defer mu.Unlock()
		wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return wsjson.Write(wctx, c, m)
	}

	cmd, op, desc, err := buildCommand(req)
	if err != nil {
		s.record(r, op, desc+" (rejected: "+err.Error()+")")
		_ = send(proto.ExecMessage{Type: proto.MsgExit, Code: -1, Error: err.Error()})
		c.Close(websocket.StatusNormalClosure, "")
		return
	}
	s.record(r, op, desc)

	code, runErr := runStreaming(ctx, c, cmd, req, send)
	m := proto.ExecMessage{Type: proto.MsgExit, Code: code}
	if runErr != nil {
		m.Error = runErr.Error()
	}
	_ = send(m)
	c.Close(websocket.StatusNormalClosure, "")
}

type streamWriter struct {
	typ  string
	send func(proto.ExecMessage) error
}

func (sw streamWriter) Write(p []byte) (int, error) {
	if err := sw.send(proto.ExecMessage{Type: sw.typ, Data: append([]byte(nil), p...)}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func runStreaming(ctx context.Context, c *websocket.Conn, cmd *exec.Cmd, req proto.ExecRequest, send func(proto.ExecMessage) error) (int, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if req.TimeoutSec > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, time.Duration(req.TimeoutSec)*time.Second)
		defer cancel()
	}

	cmd.Dir = req.Dir
	cmd.Stdout = streamWriter{proto.MsgStdout, send}
	cmd.Stderr = streamWriter{proto.MsgStderr, send}
	// Grandchildren that inherit the pipes must not keep the stream open.
	cmd.WaitDelay = 2 * time.Second
	var stdin io.WriteCloser
	if req.Stdin {
		var err error
		if stdin, err = cmd.StdinPipe(); err != nil {
			return -1, err
		}
	}
	if err := cmd.Start(); err != nil {
		return -1, err
	}

	// Client messages: stdin data, stdin close, kill. A dropped connection
	// kills the command, so nothing keeps running unattended. Read on ctx, not
	// runCtx: a cancelled read context closes the socket, and the exit
	// message after a timeout or kill must still get through.
	go func() {
		for {
			var m proto.ExecMessage
			if err := wsjson.Read(ctx, c, &m); err != nil {
				cancel()
				return
			}
			switch m.Type {
			case proto.MsgStdin:
				if stdin != nil {
					_, _ = stdin.Write(m.Data)
				}
			case proto.MsgStdinClose:
				if stdin != nil {
					_ = stdin.Close()
				}
			case proto.MsgKill:
				cancel()
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		select {
		case <-runCtx.Done():
			killTree(cmd)
		case <-done:
		}
	}()
	err := cmd.Wait()
	close(done)

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return -1, fmt.Errorf("timed out after %ds", req.TimeoutSec)
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &ee):
		if runCtx.Err() != nil {
			return ee.ExitCode(), errors.New("killed")
		}
		return ee.ExitCode(), nil
	default:
		return -1, err
	}
}

// buildCommand turns a request into a process. op and desc feed the audit log.
func buildCommand(req proto.ExecRequest) (cmd *exec.Cmd, op, desc string, err error) {
	switch {
	case req.Action != "":
		op = "action"
		desc = strings.TrimSpace(req.Action + " " + strings.Join(req.Args, " "))
		a, ok := findAction(req.Action)
		if !ok {
			return nil, op, desc, fmt.Errorf("unknown action %q", req.Action)
		}
		line, err := a.render(req.Args)
		if err != nil {
			return nil, op, desc, err
		}
		cmd, err = shellCommand(a.Shell, line)
		return cmd, op, desc, err
	case len(req.Argv) > 0:
		op, desc = "exec", strings.Join(req.Argv, " ")
		cmd = exec.Command(req.Argv[0], req.Argv[1:]...)
		prepareCmd(cmd)
		return cmd, op, desc, nil
	case req.Command != "":
		op, desc = "exec", req.Command
		if req.Shell != "" {
			desc = req.Shell + ": " + req.Command
		}
		cmd, err = shellCommand(req.Shell, req.Command)
		return cmd, op, desc, err
	default:
		return nil, "exec", "", errors.New("empty request")
	}
}
