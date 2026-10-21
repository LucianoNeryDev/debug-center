package agent

import (
	"context"
	"errors"
	"os"
	"time"

	"tailscale.com/tsnet"
)

// Logout brings the stored node up just long enough to log it out, so the
// control server drops it. It is a no-op when there is no registered state.
func Logout(ctx context.Context, cfg *Config) error {
	if _, err := os.Stat(cfg.StateDir()); err != nil {
		return nil
	}
	ts := &tsnet.Server{
		Dir:        cfg.StateDir(),
		Hostname:   cfg.Hostname(),
		ControlURL: cfg.ControlURL,
		Logf:       func(string, ...any) {},
		UserLogf:   func(string, ...any) {},
	}
	defer ts.Close()
	if err := ts.Start(); err != nil {
		return err
	}
	lc, err := ts.LocalClient()
	if err != nil {
		return err
	}
	for {
		st, err := lc.StatusWithoutPeers(ctx)
		if err == nil {
			switch st.BackendState {
			case "Running":
				return lc.Logout(ctx)
			case "NeedsLogin", "Stopped":
				return nil // already logged out or never registered
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("timed out waiting for the tailnet; remove the node on the control server by hand")
		case <-time.After(500 * time.Millisecond):
		}
	}
}
