package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// Audit appends one JSON line per request so the machine owner can see
// exactly what was done and by whom.
type Audit struct {
	mu   sync.Mutex
	path string
}

func NewAudit(path string) *Audit { return &Audit{path: path} }

// Log appends e, stamped with the current time. An error means the line was
// not written: handlers that change the machine must then refuse the request.
func (a *Audit) Log(e proto.AuditEntry) error {
	e.Time = time.Now().UTC()
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return nil
}

// Tail returns the last n entries.
func (a *Audit) Tail(n int) ([]proto.AuditEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.Open(a.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []proto.AuditEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var e proto.AuditEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			all = append(all, e)
		}
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, sc.Err()
}
