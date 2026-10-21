package controller

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// Color is on for terminals unless NO_COLOR is set.
var Color = term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("NO_COLOR") == ""

func paint(code, s string) string {
	if !Color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func Bold(s string) string  { return paint("1", s) }
func Dim(s string) string   { return paint("2", s) }
func Green(s string) string { return paint("32", s) }
func Red(s string) string   { return paint("31", s) }

func Bytes(n float64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%.0f B", n)
	}
	exp, div := 0, float64(unit)
	for v := n / unit; v >= unit && exp < 5; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", n/div, "KMGTPE"[exp])
}

func Duration(sec uint64) string {
	d := time.Duration(sec) * time.Second
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	m := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, h)
	}
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// Ago renders a past time relative to now.
func Ago(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return Duration(uint64(time.Since(t).Seconds())) + " ago"
}

func bar(pct float64, width int) string {
	n := int(pct / 100 * float64(width))
	n = max(0, min(n, width))
	b := strings.Repeat("|", n) + strings.Repeat(" ", width-n)
	switch {
	case pct >= 90:
		b = paint("31", b)
	case pct >= 70:
		b = paint("33", b)
	default:
		b = paint("32", b)
	}
	return "[" + b + "]"
}

// PrintInfo renders the machine inventory.
func PrintInfo(w io.Writer, si proto.SystemInfo) {
	row := func(k, format string, a ...any) {
		fmt.Fprintf(w, "%-10s %s\n", Bold(k), fmt.Sprintf(format, a...))
	}
	a := si.Agent
	flags := []string{a.OS + "/" + a.Arch, "v" + a.Version}
	if a.Elevated {
		flags = append(flags, "elevated")
	} else {
		flags = append(flags, Red("NOT elevated"))
	}
	if a.Service {
		flags = append(flags, "service")
	} else {
		flags = append(flags, "session")
	}
	row("Agent", "%s  %s", a.Hostname, Dim("("+a.Name+", id "+a.ID+", "+strings.Join(flags, ", ")+")"))
	h := si.Host
	row("Host", "%s  %s %s  kernel %s %s  up %s", h.Hostname, h.Platform, h.PlatformVersion, h.KernelVersion, h.KernelArch, Duration(h.UptimeSec))
	if h.Virtualization != "" {
		row("Virtual", "%s guest", h.Virtualization)
	}
	hw := si.Hardware
	if hw.Manufacturer != "" || hw.Model != "" {
		s := strings.TrimSpace(hw.Manufacturer + " " + hw.Model)
		if hw.Serial != "" {
			s += "  serial " + hw.Serial
		}
		row("Machine", "%s", s)
	}
	if hw.BIOSVendor != "" || hw.BoardModel != "" {
		row("BIOS", "%s %s (%s)  board %s %s", hw.BIOSVendor, hw.BIOSVersion, hw.BIOSDate, hw.BoardVendor, hw.BoardModel)
	}
	c := si.CPU
	row("CPU", "%s  %d cores / %d threads  %.0f MHz", c.Model, c.PhysicalCores, c.LogicalCores, c.MHz)
	row("Memory", "%s  (swap %s)", Bytes(float64(si.Memory.Total)), Bytes(float64(si.Memory.SwapTotal)))
	for _, m := range si.Memory.Modules {
		fmt.Fprintf(w, "  %-8s %s %d MHz %s %s\n", m.Slot, Bytes(float64(m.Capacity)), m.SpeedMHz, m.Manufacturer, m.PartNumber)
	}
	for i, g := range hw.GPUs {
		k := ""
		if i == 0 {
			k = "GPU"
		}
		row(k, "%s", g)
	}
	for i, d := range hw.Disks {
		k := ""
		if i == 0 {
			k = "Disks"
		}
		row(k, "%-40s %10s  %s %s %s", d.Model, Bytes(float64(d.Size)), d.Interface, d.Media, d.Status)
	}
	for i, v := range si.Volumes {
		k := ""
		if i == 0 {
			k = "Volumes"
		}
		row(k, "%-20s %-6s %10s  %s %4.0f%%  %s free", v.Mountpoint, v.FSType, Bytes(float64(v.Total)), bar(v.UsedPercent, 20), v.UsedPercent, Bytes(float64(v.Free)))
	}
	first := true
	for _, n := range si.NICs {
		if !n.Up || len(n.Addrs) == 0 {
			continue
		}
		k := ""
		if first {
			k, first = "Network", false
		}
		row(k, "%-20s %-17s %s", n.Name, n.MAC, strings.Join(n.Addrs, " "))
	}
	if len(si.Users) > 0 {
		row("Users", "%s", strings.Join(si.Users, ", "))
	}
	for i, wn := range si.Warnings {
		k := ""
		if i == 0 {
			k = "Warnings"
		}
		row(k, "%s", Dim(wn))
	}
}

// RenderTop draws a full-screen live view of one stats sample.
func RenderTop(w io.Writer, title string, st proto.Stats) {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	writeStats(&b, title, st)
	b.WriteString("\n" + Dim("Ctrl+C to exit") + "\n")
	_, _ = io.WriteString(w, b.String())
}

// PrintStats renders one stats sample inline.
func PrintStats(w io.Writer, title string, st proto.Stats) {
	var b strings.Builder
	writeStats(&b, title, st)
	_, _ = io.WriteString(w, b.String())
}

func writeStats(b *strings.Builder, title string, st proto.Stats) {
	fmt.Fprintf(b, "%s  %s  up %s  %d procs", Bold(title), st.Time.Local().Format("15:04:05"), Duration(st.UptimeSec), st.Processes)
	if len(st.Load) == 3 {
		fmt.Fprintf(b, "  load %.2f %.2f %.2f", st.Load[0], st.Load[1], st.Load[2])
	}
	b.WriteString("\n\n")
	fmt.Fprintf(b, "%-5s %s %5.1f%%\n", "CPU", bar(st.CPUPercent, 40), st.CPUPercent)
	for i, p := range st.PerCPU {
		fmt.Fprintf(b, "  %2d %s %3.0f%%", i, bar(p, 10), p)
		if i%4 == 3 || i == len(st.PerCPU)-1 {
			b.WriteString("\n")
		}
	}
	memPct := pctOf(st.MemUsed, st.MemTotal)
	fmt.Fprintf(b, "%-5s %s %5.1f%%  %s / %s\n", "Mem", bar(memPct, 40), memPct, Bytes(float64(st.MemUsed)), Bytes(float64(st.MemTotal)))
	if st.SwapTotal > 0 {
		swPct := pctOf(st.SwapUsed, st.SwapTotal)
		fmt.Fprintf(b, "%-5s %s %5.1f%%  %s / %s\n", "Swap", bar(swPct, 40), swPct, Bytes(float64(st.SwapUsed)), Bytes(float64(st.SwapTotal)))
	}
	b.WriteString("\n" + Bold("Volumes") + "\n")
	for _, v := range st.Volumes {
		fmt.Fprintf(b, "  %-22s %s %5.1f%%  %s / %s\n", trunc(v.Mountpoint, 22), bar(v.UsedPercent, 20), v.UsedPercent, Bytes(float64(v.Used)), Bytes(float64(v.Total)))
	}
	if len(st.DiskIO) > 0 {
		b.WriteString("\n" + Bold("Disk IO") + "\n")
		for _, d := range st.DiskIO {
			fmt.Fprintf(b, "  %-22s read %12s/s   write %12s/s\n", trunc(d.Name, 22), Bytes(d.Read), Bytes(d.Write))
		}
	}
	nets := append([]proto.IORate(nil), st.NetIO...)
	sort.Slice(nets, func(i, j int) bool { return nets[i].Read+nets[i].Write > nets[j].Read+nets[j].Write })
	if len(nets) > 6 {
		nets = nets[:6]
	}
	if len(nets) > 0 {
		b.WriteString("\n" + Bold("Network") + "\n")
		for _, n := range nets {
			fmt.Fprintf(b, "  %-22s down %12s/s   up %12s/s\n", trunc(n.Name, 22), Bytes(n.Read), Bytes(n.Write))
		}
	}
	if len(st.Temps) > 0 {
		b.WriteString("\n" + Bold("Temperatures") + "\n")
		for i, t := range st.Temps {
			s := fmt.Sprintf("%.0f°C", t.Celsius)
			if t.Celsius >= 85 {
				s = Red(s)
			}
			fmt.Fprintf(b, "  %-26s %6s", trunc(t.Sensor, 26), s)
			if i%3 == 2 || i == len(st.Temps)-1 {
				b.WriteString("\n")
			}
		}
	}
}

func pctOf(a, b uint64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
