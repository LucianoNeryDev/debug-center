// Package sysinfo gathers machine inventory and live load through gopsutil,
// plus per-OS hardware probes (DMI on Linux, WMI on Windows).
package sysinfo

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// Collect builds the inventory. Probes that fail are reported in Warnings so
// a partial picture still reaches the controller.
func Collect(ctx context.Context) proto.SystemInfo {
	si := proto.SystemInfo{Collected: time.Now().UTC()}
	warn := func(what string, err error) {
		if err != nil {
			si.Warnings = append(si.Warnings, fmt.Sprintf("%s: %v", what, err))
		}
	}

	if h, err := host.InfoWithContext(ctx); err == nil {
		si.Host = proto.HostInfo{
			Hostname:        h.Hostname,
			OS:              h.OS,
			Platform:        h.Platform,
			PlatformVersion: h.PlatformVersion,
			KernelVersion:   h.KernelVersion,
			KernelArch:      h.KernelArch,
			BootTime:        time.Unix(int64(h.BootTime), 0).UTC(),
			UptimeSec:       h.Uptime,
		}
		if h.VirtualizationRole == "guest" {
			si.Host.Virtualization = h.VirtualizationSystem
		}
	} else {
		warn("host", err)
	}

	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		si.CPU.Model = infos[0].ModelName
		si.CPU.Vendor = infos[0].VendorID
		si.CPU.MHz = infos[0].Mhz
	} else {
		warn("cpu", err)
	}
	si.CPU.PhysicalCores, _ = cpu.CountsWithContext(ctx, false)
	si.CPU.LogicalCores, _ = cpu.CountsWithContext(ctx, true)

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		si.Memory.Total = vm.Total
	} else {
		warn("memory", err)
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		si.Memory.SwapTotal = sw.Total
	}

	vols, err := volumes(ctx)
	warn("volumes", err)
	si.Volumes = vols

	if ifs, err := net.InterfacesWithContext(ctx); err == nil {
		for _, i := range ifs {
			nic := proto.NIC{Name: i.Name, MAC: i.HardwareAddr, MTU: i.MTU}
			for _, f := range i.Flags {
				if f == "up" {
					nic.Up = true
				}
			}
			for _, a := range i.Addrs {
				nic.Addrs = append(nic.Addrs, a.Addr)
			}
			si.NICs = append(si.NICs, nic)
		}
	} else {
		warn("nics", err)
	}

	if us, err := host.UsersWithContext(ctx); err == nil {
		seen := map[string]bool{}
		for _, u := range us {
			if !seen[u.User] {
				seen[u.User] = true
				si.Users = append(si.Users, u.User)
			}
		}
	}

	hw, mods, errs := hardware(ctx)
	si.Hardware = hw
	si.Memory.Modules = mods
	for _, e := range errs {
		warn("hardware", e)
	}
	return si
}

func volumes(ctx context.Context) ([]proto.Volume, error) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, err
	}
	var out []proto.Volume
	for _, p := range parts {
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		out = append(out, proto.Volume{
			Device: p.Device, Mountpoint: p.Mountpoint, FSType: p.Fstype,
			Total: u.Total, Used: u.Used, Free: u.Free, UsedPercent: u.UsedPercent,
		})
	}
	return out, nil
}
