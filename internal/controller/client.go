package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// Client talks to one agent.
type Client struct {
	base string
	http *http.Client
}

func NewClient(ip netip.Addr, port int) *Client {
	return &Client{
		base: "http://" + netip.AddrPortFrom(ip, uint16(port)).String(),
		http: &http.Client{},
	}
}

func (c *Client) url(path string, q url.Values) string {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.url(path, q), body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkResp(resp); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// APIError is a non-2xx answer from an agent.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return "agent: " + e.Message }

func checkResp(resp *http.Response) error {
	if resp.StatusCode < 300 {
		return nil
	}
	var e proto.Error
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&e) == nil && e.Error != "" {
		return &APIError{Status: resp.StatusCode, Message: e.Error}
	}
	return &APIError{Status: resp.StatusCode, Message: resp.Status}
}

func (c *Client) Agent(ctx context.Context) (proto.AgentInfo, error) {
	var v proto.AgentInfo
	return v, c.do(ctx, "GET", "/v1/agent", nil, nil, &v)
}

func (c *Client) Info(ctx context.Context) (proto.SystemInfo, error) {
	var v proto.SystemInfo
	return v, c.do(ctx, "GET", "/v1/info", nil, nil, &v)
}

func (c *Client) Stats(ctx context.Context) (proto.Stats, error) {
	var v proto.Stats
	return v, c.do(ctx, "GET", "/v1/stats", nil, nil, &v)
}

func (c *Client) Processes(ctx context.Context) ([]proto.Process, error) {
	var v []proto.Process
	return v, c.do(ctx, "GET", "/v1/processes", nil, nil, &v)
}

func (c *Client) Kill(ctx context.Context, pid int) error {
	return c.do(ctx, "POST", "/v1/processes/"+strconv.Itoa(pid)+"/kill", nil, nil, nil)
}

func (c *Client) Actions(ctx context.Context) ([]proto.Action, error) {
	var v []proto.Action
	return v, c.do(ctx, "GET", "/v1/actions", nil, nil, &v)
}

func (c *Client) Audit(ctx context.Context, n int) ([]proto.AuditEntry, error) {
	var v []proto.AuditEntry
	return v, c.do(ctx, "GET", "/v1/audit", url.Values{"n": {strconv.Itoa(n)}}, nil, &v)
}

func (c *Client) Uninstall(ctx context.Context) error {
	return c.do(ctx, "POST", "/v1/uninstall", nil, nil, nil)
}

// List returns a remote directory listing.
func (c *Client) List(ctx context.Context, path string) ([]proto.FileEntry, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url("/v1/files", url.Values{"path": {path}}), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkResp(resp); err != nil {
		return nil, err
	}
	if resp.Header.Get("X-Dbc-Kind") != "dir" {
		return nil, fmt.Errorf("%s is a file, not a directory", path)
	}
	var v []proto.FileEntry
	return v, json.NewDecoder(resp.Body).Decode(&v)
}

// Pull downloads a remote file into w and returns the byte count.
func (c *Client) Pull(ctx context.Context, path string, w io.Writer) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url("/v1/files", url.Values{"path": {path}}), nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if err := checkResp(resp); err != nil {
		return 0, err
	}
	if resp.Header.Get("X-Dbc-Kind") == "dir" {
		return 0, fmt.Errorf("%s is a directory; use `dbc files`", path)
	}
	return io.Copy(w, resp.Body)
}

// Push uploads a local file to a remote path.
func (c *Client) Push(ctx context.Context, local, remote string) (int64, error) {
	f, err := os.Open(local)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if fi.IsDir() {
		return 0, fmt.Errorf("%s is a directory; push one file at a time", local)
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", c.url("/v1/files", url.Values{"path": {remote}}), f)
	if err != nil {
		return 0, err
	}
	req.ContentLength = fi.Size()
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if err := checkResp(resp); err != nil {
		return 0, err
	}
	var v struct{ Bytes int64 }
	return v.Bytes, json.NewDecoder(resp.Body).Decode(&v)
}

func (c *Client) ws(ctx context.Context, path string, q url.Values) (*websocket.Conn, error) {
	u := "ws" + c.url(path, q)[len("http"):]
	conn, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPClient: c.http})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusForbidden {
			return nil, errors.New("agent: this tailnet identity is not allowed to control the agent")
		}
		return nil, err
	}
	conn.SetReadLimit(16 << 20)
	return conn, nil
}

// StreamStats calls fn for every sample until ctx ends or fn returns an error.
func (c *Client) StreamStats(ctx context.Context, interval time.Duration, fn func(proto.Stats) error) error {
	conn, err := c.ws(ctx, "/v1/stats/stream", url.Values{"interval": {interval.String()}})
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	for {
		var st proto.Stats
		if err := wsjson.Read(ctx, conn, &st); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := fn(st); err != nil {
			return err
		}
	}
}

// Session is a running remote command.
type Session struct {
	conn *websocket.Conn
	ctx  context.Context
}

// Exec starts a command; drive it with Wait, and optionally Stdin and Kill.
func (c *Client) Exec(ctx context.Context, req proto.ExecRequest) (*Session, error) {
	conn, err := c.ws(ctx, "/v1/exec", nil)
	if err != nil {
		return nil, err
	}
	if err := wsjson.Write(ctx, conn, req); err != nil {
		conn.CloseNow()
		return nil, err
	}
	return &Session{conn: conn, ctx: ctx}, nil
}

func (s *Session) send(m proto.ExecMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return wsjson.Write(ctx, s.conn, m)
}

// Kill asks the agent to terminate the command tree.
func (s *Session) Kill() error { return s.send(proto.ExecMessage{Type: proto.MsgKill}) }

// ForwardStdin copies r to the remote stdin, then closes it.
func (s *Session) ForwardStdin(r io.Reader) {
	buf := make([]byte, 32<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if s.send(proto.ExecMessage{Type: proto.MsgStdin, Data: append([]byte(nil), buf[:n]...)}) != nil {
				return
			}
		}
		if err != nil {
			_ = s.send(proto.ExecMessage{Type: proto.MsgStdinClose})
			return
		}
	}
}

// Wait copies output to stdout/stderr and returns the remote exit code.
func (s *Session) Wait(stdout, stderr io.Writer) (int, error) {
	defer s.conn.CloseNow()
	for {
		var m proto.ExecMessage
		if err := wsjson.Read(context.Background(), s.conn, &m); err != nil {
			return -1, fmt.Errorf("connection lost: %w", err)
		}
		switch m.Type {
		case proto.MsgStdout:
			_, _ = stdout.Write(m.Data)
		case proto.MsgStderr:
			_, _ = stderr.Write(m.Data)
		case proto.MsgExit:
			if m.Error != "" {
				return m.Code, errors.New(m.Error)
			}
			return m.Code, nil
		}
	}
}
