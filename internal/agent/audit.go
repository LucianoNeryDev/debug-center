package agent

import (
	"bufio"
	"encoding/json"
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

func (a *Audit) Log(e proto.AuditEntry) {
	e.Time = time.Now().UTC()
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
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
