package sysinfo

import (
	"context"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// Processes lists processes with CPU usage measured over the given window,
// expressed as a share of the whole machine (Task Manager style).
func Processes(ctx context.Context, window time.Duration) ([]proto.Process, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	before := make(map[int32]float64, len(procs))
	for _, p := range procs {
		if t, err := p.TimesWithContext(ctx); err == nil {
			before[p.Pid] = t.User + t.System
		}
	}
	start := time.Now()
	select {
	case <-time.After(window):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	elapsed := time.Since(start).Seconds() * float64(runtime.NumCPU())

	out := make([]proto.Process, 0, len(procs))
	for _, p := range procs {
		pr := proto.Process{PID: p.Pid}
		pr.Name, _ = p.NameWithContext(ctx)
		pr.PPID, _ = p.PpidWithContext(ctx)
		pr.User, _ = p.UsernameWithContext(ctx)
		if st, err := p.StatusWithContext(ctx); err == nil && len(st) > 0 {
			pr.Status = st[0]
		}
		if m, err := p.MemoryInfoWithContext(ctx); err == nil {
			pr.RSS = m.RSS
		}
		pr.Cmdline, _ = p.CmdlineWithContext(ctx)
		if t, err := p.TimesWithContext(ctx); err == nil {
			if b, ok := before[p.Pid]; ok && elapsed > 0 {
				pr.CPUPercent = pct(t.User+t.System-b, elapsed)
			}
		}
		out = append(out, pr)
	}
	return out, nil
}

// Kill terminates a process.
func Kill(ctx context.Context, pid int32) error {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return err
	}
	return p.KillWithContext(ctx)
}
