package sysinfo

import (
	"context"
	"runtime"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/shirou/gopsutil/v4/sensors"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// Sampler produces Stats with rates computed against its own previous sample,
// so concurrent streams do not disturb each other.
type Sampler struct {
	prevT    time.Time
	prevCPU  []cpu.TimesStat
	prevDisk map[string]disk.IOCountersStat
	prevNet  map[string]net.IOCountersStat
}

// Prime takes a baseline sample; the first real Sample after it has rates.
func (s *Sampler) Prime(ctx context.Context) {
	_, _ = s.Sample(ctx)
}

// Sample reads current load. CPU and IO rates are zero on the first call.
func (s *Sampler) Sample(ctx context.Context) (proto.Stats, error) {
	now := time.Now()
	st := proto.Stats{Time: now.UTC()}
	elapsed := now.Sub(s.prevT).Seconds()

	if times, err := cpu.TimesWithContext(ctx, true); err == nil {
		if len(s.prevCPU) == len(times) {
			var busyAll, totalAll float64
			for i, t := range times {
				b, tot := busyTotal(t)
				pb, ptot := busyTotal(s.prevCPU[i])
				st.PerCPU = append(st.PerCPU, pct(b-pb, tot-ptot))
				busyAll += b - pb
				totalAll += tot - ptot
			}
			st.CPUPercent = pct(busyAll, totalAll)
		}
		s.prevCPU = times
	} else {
		return st, err
	}

	if runtime.GOOS != "windows" {
		if l, err := load.AvgWithContext(ctx); err == nil {
			st.Load = []float64{l.Load1, l.Load5, l.Load15}
		}
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		st.MemTotal, st.MemUsed = vm.Total, vm.Used
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		st.SwapTotal, st.SwapUsed = sw.Total, sw.Used
	}
	if vols, err := volumes(ctx); err == nil {
		for _, v := range vols {
			st.Volumes = append(st.Volumes, proto.VolumeUse{
				Mountpoint: v.Mountpoint, Total: v.Total, Used: v.Used, UsedPercent: v.UsedPercent,
			})
		}
	}

	if io, err := disk.IOCountersWithContext(ctx); err == nil {
		if s.prevDisk != nil && elapsed > 0 {
			for name, c := range io {
				if p, ok := s.prevDisk[name]; ok && !skipDisk(name) {
					st.DiskIO = append(st.DiskIO, proto.IORate{
						Name:  name,
						Read:  rate(c.ReadBytes, p.ReadBytes, elapsed),
						Write: rate(c.WriteBytes, p.WriteBytes, elapsed),
					})
				}
			}
			sort.Slice(st.DiskIO, func(i, j int) bool { return st.DiskIO[i].Name < st.DiskIO[j].Name })
		}
		s.prevDisk = io
	}

	if nics, err := net.IOCountersWithContext(ctx, true); err == nil {
		cur := make(map[string]net.IOCountersStat, len(nics))
		for _, n := range nics {
			cur[n.Name] = n
			if p, ok := s.prevNet[n.Name]; ok && elapsed > 0 {
				st.NetIO = append(st.NetIO, proto.IORate{
					Name:  n.Name,
					Read:  rate(n.BytesRecv, p.BytesRecv, elapsed),
					Write: rate(n.BytesSent, p.BytesSent, elapsed),
				})
			}
		}
		s.prevNet = cur
	}

	// Sensor support varies wildly; partial results come back with an error.
	temps, _ := sensors.TemperaturesWithContext(ctx)
	for _, t := range temps {
		if t.Temperature > 0 && t.Temperature < 150 {
			st.Temps = append(st.Temps, proto.Temp{Sensor: t.SensorKey, Celsius: t.Temperature})
		}
	}

	if pids, err := process.PidsWithContext(ctx); err == nil {
		st.Processes = len(pids)
	}
	st.UptimeSec, _ = host.UptimeWithContext(ctx)
	s.prevT = now
	return st, nil
}

func busyTotal(t cpu.TimesStat) (busy, total float64) {
	total = t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
	return total - t.Idle - t.Iowait, total
}

func pct(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	p := part / whole * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func rate(cur, prev uint64, secs float64) float64 {
	if cur < prev {
		return 0
	}
	return float64(cur-prev) / secs
}

// skipDisk drops pseudo devices that only add noise to the IO view.
func skipDisk(name string) bool {
	for _, p := range []string{"loop", "ram", "zram", "dm-", "sr"} {
		if len(name) >= len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}
